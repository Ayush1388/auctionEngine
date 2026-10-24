package search

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/breaker"
	"github.com/Ayush1388/auctionEngine/internal/chaos"
)

// Guarded puts a circuit breaker in front of Elasticsearch (v1.0).
//
// Without it, every search during an Elasticsearch outage waits for the
// HTTP timeout (3 s) before Service falls back to PostgreSQL: every search
// is slow, and the API holds thousands of goroutines waiting. With it,
// after a few consecutive failures searches skip Elasticsearch entirely and
// go straight to the fallback, which takes milliseconds. A trial request
// every Cooldown detects recovery.
//
// The indexer writes go through the same breaker. While it is open,
// indexing events fail fast and the outbox retries them later, instead of
// each one waiting for the timeout.
type Guarded struct {
	es *Elastic
	b  *breaker.Breaker
}

// NewGuarded wraps es with breaker b. Create b with SearchFailure as its
// IsFailure, so an invalid cursor (the caller's mistake) isn't counted.
func NewGuarded(es *Elastic, b *breaker.Breaker) *Guarded { return &Guarded{es: es, b: b} }

// SearchFailure: only errors that mean Elasticsearch is unhealthy count.
func SearchFailure(err error) bool {
	return breaker.DefaultIsFailure(err) && !errors.Is(err, ErrInvalidCursor)
}

func (g *Guarded) Name() string { return g.es.Name() }

func (g *Guarded) Search(ctx context.Context, q Query) (page Page, err error) {
	err = g.b.Do(ctx, func(ctx context.Context) error {
		if err = chaos.Fail(chaos.Elasticsearch); err != nil {
			return err
		}
		page, err = g.es.Search(ctx, q)
		return err
	})
	return page, err
}

func (g *Guarded) Suggest(ctx context.Context, prefix string, limit int) (out []Suggestion, err error) {
	err = g.b.Do(ctx, func(ctx context.Context) error {
		if err = chaos.Fail(chaos.Elasticsearch); err != nil {
			return err
		}
		out, err = g.es.Suggest(ctx, prefix, limit)
		return err
	})
	return out, err
}

func (g *Guarded) Upsert(ctx context.Context, doc Document) error {
	return g.b.Do(ctx, func(ctx context.Context) error {
		if err := chaos.Fail(chaos.Elasticsearch); err != nil {
			return err
		}
		return g.es.Upsert(ctx, doc)
	})
}

func (g *Guarded) Delete(ctx context.Context, id uuid.UUID) error {
	return g.b.Do(ctx, func(ctx context.Context) error {
		if err := chaos.Fail(chaos.Elasticsearch); err != nil {
			return err
		}
		return g.es.Delete(ctx, id)
	})
}
