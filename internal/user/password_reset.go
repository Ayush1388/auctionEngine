package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/email"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
)

// Forgot password. The link is single-use and lasts an hour; only the SHA-256 hash of its token
// is stored (the same scheme as activation tokens). Asking for a link never reveals whether an
// address has an account.

const passwordResetTTL = time.Hour

var ErrInvalidResetToken = errors.New("invalid or expired reset link")

type ForgotPasswordInput struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordInput struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// ForgotPassword queues a reset email when the address has an account, and does nothing otherwise.
func (s *Service) ForgotPassword(ctx context.Context, input ForgotPasswordInput) error {
	input.Email = strings.TrimSpace(input.Email)
	if err := s.validator.Struct(input); err != nil {
		return err
	}

	u, err := s.repository.GetActivationUser(ctx, input.Email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return err
	}

	rawToken, tokenHash, err := GenerateActivationToken()
	if err != nil {
		return fmt.Errorf("failed to generate reset token: %w", err)
	}

	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := s.repository.WithTx(tx).SetResetToken(ctx, u.ID, tokenHash, time.Now().Add(passwordResetTTL)); err != nil {
			return err
		}
		return outbox.Enqueue(ctx, tx, email.EventTypePasswordReset, email.PasswordResetEvent{To: u.Email, ResetToken: rawToken})
	})
}

// ResetPassword sets a new password from a reset link and returns the user's id, so the caller can
// sign every other session out. The new password has to meet the password policy.
func (s *Service) ResetPassword(ctx context.Context, input ResetPasswordInput) (uuid.UUID, error) {
	input.Token = strings.TrimSpace(input.Token)
	if err := s.validator.Struct(input); err != nil {
		return uuid.Nil, err
	}

	tokenHash := HashActivationToken(input.Token)
	u, err := s.repository.GetByResetToken(ctx, tokenHash, time.Now())
	if err != nil {
		return uuid.Nil, err
	}
	if err := ValidatePassword(u.Email, input.Password); err != nil {
		return uuid.Nil, err
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to hash password: %w", err)
	}
	if err := s.repository.ConsumeResetToken(ctx, u.ID, tokenHash, hash, time.Now()); err != nil {
		return uuid.Nil, err
	}
	return u.ID, nil
}
