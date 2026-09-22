package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID          uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Attempts    int
	AvailableAt time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(
	ctx context.Context,
	tx pgx.Tx,
	eventType string,
	payload any,
) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO outbox_events (
			id,
			event_type,
			payload
		)
		VALUES ($1, $2, $3)
		`,
		uuid.New(),
		eventType,
		payloadBytes,
	)

	if err != nil {
		return fmt.Errorf("failed to create outbox event: %w", err)
	}

	return nil
}
