package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ayush1388/auctionEngine/internal/database"
)

var (
	ErrEmailAlreadyExists     = errors.New("email already exists")
	ErrInvalidActivationToken = errors.New("invalid or expired activation token")
	ErrUserNotFound           = errors.New("user not found")
	ErrUserAlreadyActivated   = errors.New("user already activated")
)

// Repository reads and writes users. It never starts transactions itself;
// the service decides the boundary and passes a transaction via WithTx.
type Repository struct {
	db database.DBTX
}

func NewRepository(db database.DBTX) *Repository {
	return &Repository{
		db: db,
	}
}

// WithTx returns a copy of the repository that runs its queries in tx.
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{
		db: tx,
	}
}

// CreateActivated inserts a user who can sign in straight away (no activation email).
func (r *Repository) CreateActivated(ctx context.Context, user User) error {
	_, err := r.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, $3, $4)`,
		user.ID, user.Email, user.PasswordHash, user.ActivatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *Repository) Create(
	ctx context.Context,
	user User,
	activationTokenHash string,
	activationTokenExpiresAt time.Time,
) error {
	_, err := r.db.Exec(
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
			activated_at,
			role
		FROM users
		WHERE email = $1
		`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.ActivatedAt,
		&user.Role,
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
			activated_at,
			role
		FROM users
		WHERE id = $1
		`,
		id,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.ActivatedAt,
		&user.Role,
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
) error {
	result, err := r.db.Exec(
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

	return nil
}

func (r *Repository) UpdatePasswordHash(
	ctx context.Context,
	userID uuid.UUID,
	passwordHash string,
) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2`,
		passwordHash,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to update password hash: %w",
			err,
		)
	}

	return nil
}

// SetResetToken stores the hash of a new reset link, replacing any earlier one.
func (r *Repository) SetResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_reset_hash = $1, password_reset_expires_at = $2 WHERE id = $3`,
		tokenHash, expiresAt, userID)
	if err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}
	return nil
}

// GetByResetToken finds the account a live reset link belongs to.
func (r *Repository) GetByResetToken(ctx context.Context, tokenHash string, now time.Time) (User, error) {
	var u User
	err := r.db.QueryRow(ctx, `
		SELECT id, email FROM users
		WHERE password_reset_hash = $1 AND password_reset_expires_at > $2`,
		tokenHash, now).Scan(&u.ID, &u.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidResetToken
		}
		return User{}, fmt.Errorf("failed to look up reset token: %w", err)
	}
	return u, nil
}

// ConsumeResetToken sets the new password and burns the link in one statement, so a link works once
// even if two requests race. A reset by email also proves the address, so it activates the account.
func (r *Repository) ConsumeResetToken(ctx context.Context, userID uuid.UUID, tokenHash, passwordHash string, now time.Time) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET
			password_hash = $1,
			password_reset_hash = NULL,
			password_reset_expires_at = NULL,
			activated_at = COALESCE(activated_at, $4)
		WHERE id = $2 AND password_reset_hash = $3 AND password_reset_expires_at > $4`,
		passwordHash, userID, tokenHash, now)
	if err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidResetToken
	}
	return nil
}
