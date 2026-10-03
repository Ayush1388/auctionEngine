package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Elastic talks to Elasticsearch over its REST API with plain net/http. No
// client library: every request is visible here, which makes it easy to see
// exactly what Elasticsearch is asked to do.
//
// Index naming (zero-downtime reindexing)
//
//	alias "auctions" ──► index "auctions_v1"         (today)
//	alias "auctions" ──► index "auctions_v20261001…" (after Reindex)
//
// Reads and writes always use the alias. Reindex builds a fresh index next
// to the live one, then moves the alias in one atomic call, so searches
// never see a half-built index.
type Elastic struct {
	baseURL string
	alias   string
	http    *http.Client
}

func NewElastic(baseURL, alias string) *Elastic {
	return &Elastic{
		baseURL: strings.TrimRight(baseURL, "/"),
		alias:   alias,
		// A search engine that hangs must not hang our API: every call
		// has a deadline, and Service falls back to PostgreSQL on error.
		http: &http.Client{Timeout: 3 * time.Second},
	}
}

func (e *Elastic) Name() string { return "elasticsearch" }

// mapping defines how documents are analysed. This is where search quality
// comes from:
//
//   - "folding": lowercase + ASCII folding, so "Café" matches "cafe".
//   - "autocomplete": edge n-grams at index time. "camera" is stored as
//     "ca", "cam", "came", "camer", "camera", so a prefix query is a plain
//     term lookup in the inverted index, which is very fast.
//   - keyword fields (status, type, owner_id) are stored unanalysed for
//     exact filters.
//   - "dynamic": "strict" rejects documents with unexpected fields, so a
//     typo can't silently create a new field mapping.
var mapping = map[string]any{
	"settings": map[string]any{
		"number_of_shards":   1,
		"number_of_replicas": 0,
		"analysis": map[string]any{
			"filter": map[string]any{
				"edge_2_20": map[string]any{"type": "edge_ngram", "min_gram": 2, "max_gram": 20},
			},
			"analyzer": map[string]any{
				"folding": map[string]any{
					"tokenizer": "standard",
					"filter":    []string{"lowercase", "asciifolding"},
				},
				"autocomplete": map[string]any{
					"tokenizer": "standard",
					"filter":    []string{"lowercase", "asciifolding", "edge_2_20"},
				},
			},
		},
	},
	"mappings": map[string]any{
		"dynamic": "strict",
		"properties": map[string]any{
			"id": map[string]any{"type": "keyword"},
			"title": map[string]any{
				"type":     "text",
				"analyzer": "folding",
				"fields": map[string]any{
					// Indexed with n-grams, searched WITHOUT them: the
					// query "cam" must match the stored gram "cam", not be
					// split into "ca" + "cam" itself.
					"suggest": map[string]any{"type": "text", "analyzer": "autocomplete", "search_analyzer": "folding"},
					"raw":     map[string]any{"type": "keyword"},
				},
			},
			"description":    map[string]any{"type": "text", "analyzer": "folding"},
			"type":           map[string]any{"type": "keyword"},
			"status":         map[string]any{"type": "keyword"},
			"owner_id":       map[string]any{"type": "keyword"},
			"starting_price": map[string]any{"type": "long"},
			"current_bid":    map[string]any{"type": "long"},
			"bid_count":      map[string]any{"type": "integer"},
			"ends_at":        map[string]any{"type": "date"},
			"created_at":     map[string]any{"type": "date"},
		},
	},
}

// EnsureIndex creates "<alias>_v1" and points the alias at it, unless the
// alias already exists. Safe to call on every start-up.
func (e *Elastic) EnsureIndex(ctx context.Context) error {
	status, _, err := e.do(ctx, http.MethodHead, "/_alias/"+e.alias, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}

	return e.createIndex(ctx, e.alias+"_v1", true)
}

func (e *Elastic) createIndex(ctx context.Context, name string, withAlias bool) error {
	body := map[string]any{"settings": mapping["settings"], "mappings": mapping["mappings"]}
	if withAlias {
		body["aliases"] = map[string]any{e.alias: map[string]any{}}
	}

	status, resp, err := e.do(ctx, http.MethodPut, "/"+name, body)
	if err != nil {
		return err
	}
	// Two instances starting at once may both try; the loser sees "already exists".
	if status >= 300 && !bytes.Contains(resp, []byte("resource_already_exists_exception")) {
		return fmt.Errorf("create index %s: %d %s", name, status, resp)
	}
	return nil
}

// Upsert writes doc with external versioning.
//
// version_type=external_gte means "accept this write only if its version is
// >= the stored one". Two events for the same auction can be processed out
// of order (retries, several workers); without this, an older snapshot that
// arrives last would overwrite the newer one and search would show a stale
// price until the next change.
func (e *Elastic) Upsert(ctx context.Context, doc Document) error {
	path := fmt.Sprintf("/%s/_doc/%s?version=%d&version_type=external_gte",
		e.alias, doc.ID, doc.Version)

	status, resp, err := e.do(ctx, http.MethodPut, path, doc)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		// A newer version is already indexed. That's success, not an error.
		return nil
	}
	if status >= 300 {
		return fmt.Errorf("index %s: %d %s", doc.ID, status, resp)
	}
	return nil
}

func (e *Elastic) Delete(ctx context.Context, id uuid.UUID) error {
	status, resp, err := e.do(ctx, http.MethodDelete, "/"+e.alias+"/_doc/"+id.String(), nil)
	if err != nil {
		return err
	}
	if status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("delete %s: %d %s", id, status, resp)
	}
	return nil
}

// Search runs a relevance-ranked query.
//
//   - multi_match over title (boosted ×3), type (×2) and description.
//   - fuzziness AUTO forgives typos: "camra" still finds "camera".
//   - filters (status) go in "filter", not "must": they don't affect the
//     score and Elasticsearch caches them.
//   - Pagination uses search_after with a total sort order (_score, then
//     created_at, then id as the tie-breaker): the same keyset idea as
//     GET /v1/auctions. from/size would get slower and less stable the
//     deeper you page.
func (e *Elastic) Search(ctx context.Context, q Query) (Page, error) {
	c, err := decodeCursor(q.Cursor, e.Name())
	if err != nil {
		return Page{}, err
	}

	must := []any{map[string]any{
		"multi_match": map[string]any{
			"query":     q.Text,
			"fields":    []string{"title^3", "type^2", "description"},
			"fuzziness": "AUTO",
			"operator":  "and",
		},
	}}

	var filter []any
	if q.Status != "" {
		filter = append(filter, map[string]any{"term": map[string]any{"status": q.Status}})
	}

	body := map[string]any{
		"size":    q.Limit + 1, // one extra tells us whether there's a next page
		"_source": false,       // we only need IDs; the database is the source of truth
		"query":   map[string]any{"bool": map[string]any{"must": must, "filter": filter}},
		"sort": []any{
			"_score",
			map[string]any{"created_at": "desc"},
			map[string]any{"id": "asc"},
		},
		"highlight": map[string]any{
			"fields": map[string]any{
				"title":       map[string]any{"number_of_fragments": 0},
				"description": map[string]any{"fragment_size": 120, "number_of_fragments": 1},
			},
		},
	}
	if len(c.After) > 0 {
		body["search_after"] = c.After
	}

	status, resp, err := e.do(ctx, http.MethodPost, "/"+e.alias+"/_search", body)
	if err != nil {
		return Page{}, err
	}
	if status >= 300 {
		return Page{}, fmt.Errorf("search: %d %s", status, resp)
	}

	var result struct {
		Hits struct {
			Hits []struct {
				ID        string              `json:"_id"`
				Sort      []any               `json:"sort"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return Page{}, fmt.Errorf("decode search response: %w", err)
	}

	page := Page{Backend: e.Name()}
	hits := result.Hits.Hits
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
		page.NextCursor = encodeCursor(cursor{Backend: e.Name(), After: hits[len(hits)-1].Sort})
	}

	for _, h := range hits {
		id, err := uuid.Parse(h.ID)
		if err != nil {
			continue
		}
		hit := Hit{ID: id}
		if frag := h.Highlight["title"]; len(frag) > 0 {
			hit.Highlight = frag[0]
		} else if frag := h.Highlight["description"]; len(frag) > 0 {
			hit.Highlight = frag[0]
		}
		page.Hits = append(page.Hits, hit)
	}

	return page, nil
}

// Suggest returns up to limit active auctions whose title has a word
// starting with prefix, using the edge n-gram subfield.
func (e *Elastic) Suggest(ctx context.Context, prefix string, limit int) ([]Suggestion, error) {
	body := map[string]any{
		"size":    limit,
		"_source": []string{"id", "title"},
		"query": map[string]any{"bool": map[string]any{
			"must":   map[string]any{"match": map[string]any{"title.suggest": map[string]any{"query": prefix, "operator": "and"}}},
			"filter": map[string]any{"term": map[string]any{"status": "ACTIVE"}},
		}},
	}

	status, resp, err := e.do(ctx, http.MethodPost, "/"+e.alias+"/_search", body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("suggest: %d %s", status, resp)
	}

	var result struct {
		Hits struct {
			Hits []struct {
				Source Suggestion `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode suggest response: %w", err)
	}

	out := make([]Suggestion, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		out = append(out, h.Source)
	}
	return out, nil
}

// Refresh makes recent writes searchable immediately. Elasticsearch normally
// does this about once a second ("near real-time search"); tests call it so
// they don't have to sleep.
func (e *Elastic) Refresh(ctx context.Context) error {
	status, resp, err := e.do(ctx, http.MethodPost, "/"+e.alias+"/_refresh", nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("refresh: %d %s", status, resp)
	}
	return nil
}

// do sends one JSON request and returns the status and body.
func (e *Elastic) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("elasticsearch %s %s: %w", method, redactURL(path), err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, b, nil
}

func redactURL(p string) string {
	if u, err := url.Parse(p); err == nil {
		return u.Path
	}
	return p
}

// Ping checks the cluster is reachable and not red (readiness, v1.0).
func (e *Elastic) Ping(ctx context.Context) error {
	code, body, err := e.do(ctx, http.MethodGet, "/_cluster/health", nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("elasticsearch health: status %d", code)
	}
	var h struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return err
	}
	if h.Status == "red" {
		return errors.New("elasticsearch cluster is red")
	}
	return nil
}
