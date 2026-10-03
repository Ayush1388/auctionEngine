package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// Loader reads the current state of an auction from the source of truth.
// auction.Service.Get satisfies it.
type Loader func(ctx context.Context, id uuid.UUID) (auction.Auction, error)

// Indexer keeps Elasticsearch in sync with PostgreSQL. It is an outbox
// handler: every event that changes an auction (created, activated,
// cancelled, completed, settled, bid.placed) triggers a re-index of that
// auction.
//
// Why events instead of writing to Elasticsearch in the request:
//   - Dual write again (see docs/decisions/0001): the database commit can
//     succeed and the Elasticsearch call fail, or the reverse. The outbox
//     makes the "please re-index" intent part of the same commit.
//   - Elasticsearch being slow or down must not slow down or fail bids.
//
// Why it re-reads the auction instead of using the event payload:
// events can be delivered late, twice, or out of order. Reading the
// *current* row and indexing it with its version is idempotent (doing it
// twice is harmless) and self-correcting (a late event just re-indexes the
// latest state, and external versioning rejects anything older).
type Indexer struct {
	index  Index
	load   Loader
	logger *slog.Logger
}

func NewIndexer(index Index, load Loader, logger *slog.Logger) *Indexer {
	return &Indexer{index: index, load: load, logger: logger}
}

// Handler is registered with the outbox router for every auction event.
func (ix *Indexer) Handler() outbox.Handler {
	return outbox.HandlerFunc(func(ctx context.Context, event outbox.Event) error {
		// Every auction event payload carries auction_id.
		var payload struct {
			AuctionID uuid.UUID `json:"auction_id"`
		}
		if err := outbox.DecodePayload(event, &payload); err != nil {
			return err
		}
		if payload.AuctionID == uuid.Nil {
			return fmt.Errorf("%s event without auction_id", event.EventType)
		}
		return ix.Sync(ctx, payload.AuctionID)
	})
}

// Sync indexes the current state of one auction, or removes it from the
// index if it no longer exists.
func (ix *Indexer) Sync(ctx context.Context, id uuid.UUID) error {
	a, err := ix.load(ctx, id)
	if errors.Is(err, auction.ErrNotFound) {
		return ix.index.Delete(ctx, id)
	}
	if err != nil {
		return err
	}
	return ix.index.Upsert(ctx, DocumentFrom(a))
}

// DocumentFrom flattens an auction and its item into one search document.
func DocumentFrom(a auction.Auction) Document {
	return Document{
		ID:            a.ID,
		Title:         a.Item.Name,
		Description:   a.Item.Description,
		Type:          a.Item.Type,
		Status:        string(a.Status),
		OwnerID:       a.OwnerID,
		StartingPrice: a.StartingPrice,
		CurrentBid:    a.CurrentBid,
		BidCount:      a.BidCount,
		EndsAt:        a.EndsAt.UTC(),
		CreatedAt:     a.CreatedAt.UTC(),
		Version:       a.Version,
	}
}

// Reindex rebuilds the whole index from PostgreSQL without downtime:
//
//  1. create a new index "<alias>_v<timestamp>" with the current mapping
//  2. bulk-load every auction into it (searches still hit the old index)
//  3. atomically move the alias to the new index
//  4. delete the old index
//
// Use it after changing the mapping or if the index is ever lost.
// next is called repeatedly and returns the next batch of auctions, or an
// empty slice when done.
func (e *Elastic) Reindex(ctx context.Context, next func(ctx context.Context) ([]auction.Auction, error)) (int, error) {
	name := fmt.Sprintf("%s_v%d", e.alias, time.Now().UTC().UnixNano())
	if err := e.createIndex(ctx, name, false); err != nil {
		return 0, err
	}

	total := 0
	for {
		batch, err := next(ctx)
		if err != nil {
			return total, err
		}
		if len(batch) == 0 {
			break
		}
		if err := e.bulk(ctx, name, batch); err != nil {
			return total, err
		}
		total += len(batch)
	}

	old, err := e.indicesBehindAlias(ctx)
	if err != nil {
		return total, err
	}

	// One _aliases call with remove + add is atomic: there is no instant
	// where the alias points at nothing or at both indices.
	actions := []any{map[string]any{"add": map[string]any{"index": name, "alias": e.alias}}}
	for _, idx := range old {
		actions = append(actions, map[string]any{"remove": map[string]any{"index": idx, "alias": e.alias}})
	}
	status, resp, err := e.do(ctx, http.MethodPost, "/_aliases", map[string]any{"actions": actions})
	if err != nil {
		return total, err
	}
	if status >= 300 {
		return total, fmt.Errorf("swap alias: %d %s", status, resp)
	}

	for _, idx := range old {
		_, _, _ = e.do(ctx, http.MethodDelete, "/"+idx, nil)
	}
	return total, nil
}

// bulk writes a batch with the _bulk API: one HTTP request, newline-
// delimited JSON (an action line, then a document line, per document).
func (e *Elastic) bulk(ctx context.Context, index string, batch []auction.Auction) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, a := range batch {
		doc := DocumentFrom(a)
		_ = enc.Encode(map[string]any{"index": map[string]any{
			"_index": index, "_id": doc.ID, "version": doc.Version, "version_type": "external_gte",
		}})
		_ = enc.Encode(doc)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/_bulk", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int `json:"status"`
			Error  any `json:"error"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("bulk response: %w", err)
	}
	if result.Errors {
		for _, item := range result.Items {
			for _, r := range item {
				// 409 = a newer version is already there; fine.
				if r.Error != nil && r.Status != http.StatusConflict {
					return fmt.Errorf("bulk item failed: %v", r.Error)
				}
			}
		}
	}
	return nil
}

func (e *Elastic) indicesBehindAlias(ctx context.Context) ([]string, error) {
	status, resp, err := e.do(ctx, http.MethodGet, "/_alias/"+e.alias, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status >= 300 {
		return nil, fmt.Errorf("get alias: %d %s", status, resp)
	}
	var m map[string]any
	if err := json.Unmarshal(resp, &m); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m))
	for idx := range m {
		out = append(out, idx)
	}
	return out, nil
}
