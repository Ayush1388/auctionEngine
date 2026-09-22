package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailAlreadyExists     = errors.New("email already exists")
	ErrInvalidActivationToken = errors.New("invalid or expired activation token")
	ErrUserNotFound           = errors.New("user not found")
	ErrUserAlreadyActivated   = errors.New("user already activated")
)

const activationEmailEventType = "email.activation"

type ActivationEmailEvent struct {
	To              string `json:"to"`
	ActivationToken string `json:"activation_token"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) Create(
	ctx context.Context,
	user User,
	activationTokenHash string,
	activationTokenExpiresAt time.Time,
	activationToken string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin user registration transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO users (
			id,
			email,
			password_hash,
			activation_token_hash,
			activation_token_expires_at
		)
		VALUES ($1, $2, $3, $4, $5)
		`,
		user.ID,
		user.Email,
		user.PasswordHash,
		activationTokenHash,
		activationTokenExpiresAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailAlreadyExists
		}

		return fmt.Errorf(
			"failed to create user: %w",
			err,
		)
	}

	activationEmailPayload := ActivationEmailEvent{
		To:              user.Email,
		ActivationToken: activationToken,
	}

	payload, err := json.Marshal(activationEmailPayload)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal activation email payload: %w",
			err,
		)
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
		activationEmailEventType,
		payload,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to create activation email outbox event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit user registration transaction: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) GetByEmail(
	ctx context.Context,
	email string,
) (User, error) {
	var user User

	err := r.db.QueryRow(
		ctx,
		`
		SELECT
			id,
			email,
			password_hash,
			activated_at
		FROM users
		WHERE email = $1
		`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.ActivatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}

		return User{}, fmt.Errorf(
			"failed to get user by email: %w",
			err,
		)
	}

	return user, nil
}

func (r *Repository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (User, error) {
	var user User

	err := r.db.QueryRow(
		ctx,
		`
		SELECT
			id,
			email,
			password_hash,
			activated_at
		FROM users
		WHERE id = $1
		`,
		id,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.ActivatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}

		return User{}, fmt.Errorf(
			"failed to get user by ID: %w",
			err,
		)
	}

	return user, nil
}

func (r *Repository) Activate(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) error {
	result, err := r.db.Exec(
		ctx,
		`
		UPDATE users
		SET
			activated_at = $1,
			activation_token_hash = NULL,
			activation_token_expires_at = NULL
		WHERE
			activation_token_hash = $2
			AND activation_token_expires_at > $1
			AND activated_at IS NULL
		`,
		now,
		tokenHash,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to activate user: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrInvalidActivationToken
	}

	return nil
}

func (r *Repository) GetActivationUser(
	ctx context.Context,
	email string,
) (User, error) {
	var user User

	err := r.db.QueryRow(
		ctx,
		`
		SELECT
			id,
			email,
			activated_at
		FROM users
		WHERE email = $1
		`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.ActivatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}

		return User{}, fmt.Errorf(
			"failed to get activation user: %w",
			err,
		)
	}

	return user, nil
}

func (r *Repository) UpdateActivationToken(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
	rawToken string,
	email string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin resend activation transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(
		ctx,
		`
		UPDATE users
		SET
			activation_token_hash = $1,
			activation_token_expires_at = $2
		WHERE
			id = $3
			AND activated_at IS NULL
		`,
		tokenHash,
		expiresAt,
		userID,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to update activation token: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrUserAlreadyActivated
	}

	activationEmailPayload := ActivationEmailEvent{
		To:              email,
		ActivationToken: rawToken,
	}

	payload, err := json.Marshal(activationEmailPayload)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal activation email payload: %w",
			err,
		)
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
		activationEmailEventType,
		payload,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to create activation email outbox event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit resend activation transaction: %w",
			err,
		)
	}

	return nil
}
