package search

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// Service answers search requests from the primary backend (Elasticsearch,
// if configured) and falls back to PostgreSQL when it fails.
//
// The fallback is the "graceful degradation" principle used throughout the
// project: an optional dependency failing should make a feature worse, not
// make it disappear. The response says which backend answered.
type Service struct {
	primary  Backend // may be nil
	fallback Backend
	logger   *slog.Logger
}

func NewService(primary, fallback Backend, logger *slog.Logger) *Service {
	return &Service{primary: primary, fallback: fallback, logger: logger}
}

// SearchInput is the raw query string input.
type SearchInput struct {
	Text   string
	Status string
	Limit  string
	Cursor string
}

func (s *Service) Search(ctx context.Context, in SearchInput) (Page, error) {
	q, err := parseQuery(in)
	if err != nil {
		return Page{}, err
	}

	if s.primary != nil {
		page, err := s.primary.Search(ctx, q)
		if err == nil {
			return page, nil
		}
		if err == ErrInvalidCursor {
			return Page{}, invalidCursor()
		}
		s.logger.Warn("search backend failed, falling back", "backend", s.primary.Name(), "error", err)
	}

	page, err := s.fallback.Search(ctx, q)
	if err == ErrInvalidCursor {
		return Page{}, invalidCursor()
	}
	return page, err
}

func (s *Service) Suggest(ctx context.Context, prefix, limitStr string) ([]Suggestion, error) {
	prefix = strings.TrimSpace(prefix)
	problems := &validation.Error{}
	if len(prefix) < 2 || len(prefix) > 50 {
		problems.Add("q", "must be 2 to 50 characters")
	}
	limit := 5
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 || n > 10 {
			problems.Add("limit", "must be between 1 and 10")
		}
		limit = n
	}
	if err := problems.OrNil(); err != nil {
		return nil, err
	}

	if s.primary != nil {
		out, err := s.primary.Suggest(ctx, prefix, limit)
		if err == nil {
			return out, nil
		}
		s.logger.Warn("suggest backend failed, falling back", "backend", s.primary.Name(), "error", err)
	}
	return s.fallback.Suggest(ctx, prefix, limit)
}

func parseQuery(in SearchInput) (Query, error) {
	problems := &validation.Error{}

	q := Query{Text: strings.TrimSpace(in.Text), Status: strings.ToUpper(strings.TrimSpace(in.Status)), Cursor: in.Cursor, Limit: DefaultLimit}

	if q.Text == "" || len(q.Text) > 200 {
		problems.Add("q", "must be 1 to 200 characters")
	}
	switch q.Status {
	case "", "NOT_ACTIVE", "ACTIVE", "COMPLETED", "CANCELLED":
	default:
		problems.Add("status", "must be one of: NOT_ACTIVE, ACTIVE, COMPLETED, CANCELLED")
	}
	if in.Limit != "" {
		n, err := strconv.Atoi(in.Limit)
		if err != nil || n < 1 || n > MaxLimit {
			problems.Add("limit", "must be a whole number between 1 and 50")
		}
		q.Limit = n
	}

	return q, problems.OrNil()
}

func invalidCursor() error {
	problems := &validation.Error{}
	problems.Add("cursor", "is invalid")
	return problems
}
