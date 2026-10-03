package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ayush1388/auctionEngine/internal/database"
	"github.com/Ayush1388/auctionEngine/internal/email"
	"github.com/Ayush1388/auctionEngine/internal/outbox"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrAccountNotActivated = errors.New("account is not activated")
)

const activationTokenTTL = 24 * time.Hour

type RegisterInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=15,max=128"`
}

type ResendActivationInput struct {
	Email string `json:"email" validate:"required,email"`
}

type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginResult struct {
	User        User
	AccessToken string
}

type Service struct {
	db         database.TxBeginner
	repository *Repository
	validator  *validation.Validator
	jwt        *JWTService
}

func NewService(
	db database.TxBeginner,
	repository *Repository,
	jwtService *JWTService,
) *Service {
	return &Service{
		db:         db,
		repository: repository,
		validator:  validation.New(),
		jwt:        jwtService,
	}
}

func (s *Service) Register(
	ctx context.Context,
	input RegisterInput,
) (User, error) {
	input.Email = strings.TrimSpace(input.Email)

	if err := s.validator.Struct(input); err != nil {
		return User{}, err
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, fmt.Errorf(
			"failed to hash password: %w",
			err,
		)
	}

	rawToken, tokenHash, err := GenerateActivationToken()
	if err != nil {
		return User{}, fmt.Errorf(
			"failed to generate activation token: %w",
			err,
		)
	}

	newUser := User{
		ID:           uuid.New(),
		Email:        input.Email,
		PasswordHash: passwordHash,
	}

	// The user and their activation email are committed together: either
	// both exist or neither does. See docs/decisions/0001.
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := s.repository.WithTx(tx).Create(
			ctx,
			newUser,
			tokenHash,
			time.Now().Add(activationTokenTTL),
		); err != nil {
			return err
		}

		return enqueueActivationEmail(ctx, tx, newUser.Email, rawToken)
	})
	if err != nil {
		return User{}, err
	}

	return newUser, nil
}

func (s *Service) Activate(
	ctx context.Context,
	rawToken string,
) error {
	rawToken = strings.TrimSpace(rawToken)

	if rawToken == "" {
		return ErrInvalidActivationToken
	}

	tokenHash := HashActivationToken(rawToken)

	if err := s.repository.Activate(
		ctx,
		tokenHash,
		time.Now(),
	); err != nil {
		return err
	}

	return nil
}

func (s *Service) ResendActivation(
	ctx context.Context,
	input ResendActivationInput,
) error {
	input.Email = strings.TrimSpace(input.Email)

	if err := s.validator.Struct(input); err != nil {
		return err
	}

	existingUser, err := s.repository.GetActivationUser(
		ctx,
		input.Email,
	)
	if err != nil {
		// Unknown emails succeed silently. The response must not reveal
		// whether an address has an account.
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return err
	}

	if existingUser.ActivatedAt != nil {
		return nil
	}

	rawToken, tokenHash, err := GenerateActivationToken()
	if err != nil {
		return fmt.Errorf(
			"failed to generate activation token: %w",
			err,
		)
	}

	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := s.repository.WithTx(tx).UpdateActivationToken(
			ctx,
			existingUser.ID,
			tokenHash,
			time.Now().Add(activationTokenTTL),
		); err != nil {
			// Activated between our read and this update.
			if errors.Is(err, ErrUserAlreadyActivated) {
				return nil
			}
			return err
		}

		return enqueueActivationEmail(ctx, tx, existingUser.Email, rawToken)
	})
}

func (s *Service) Login(
	ctx context.Context,
	input LoginInput,
) (LoginResult, error) {
	input.Email = strings.TrimSpace(input.Email)

	if err := s.validator.Struct(input); err != nil {
		return LoginResult{}, err
	}

	existingUser, err := s.repository.GetByEmail(
		ctx,
		input.Email,
	)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Do the same work as a real check so timing doesn't reveal
			// that the email isn't registered.
			_ = CheckPassword(input.Password, dummyHash)
			return LoginResult{}, ErrInvalidCredentials
		}

		return LoginResult{}, fmt.Errorf(
			"failed to find user: %w",
			err,
		)
	}

	// Verify the password before saying anything about the account.
	// Checking activation first would let anyone learn, without knowing
	// the password, that an email is registered but not activated.
	if err := CheckPassword(
		input.Password,
		existingUser.PasswordHash,
	); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	if existingUser.ActivatedAt == nil {
		return LoginResult{}, ErrAccountNotActivated
	}

	if NeedsRehash(existingUser.PasswordHash) {
		s.upgradePasswordHash(ctx, existingUser.ID, input.Password)
	}

	accessToken, err := s.jwt.GenerateToken(
		existingUser.ID,
		existingUser.Role,
	)
	if err != nil {
		return LoginResult{}, fmt.Errorf(
			"failed to generate access token: %w",
			err,
		)
	}

	return LoginResult{
		User:        existingUser,
		AccessToken: accessToken,
	}, nil
}

func (s *Service) GetByID(
	ctx context.Context,
	userID uuid.UUID,
) (User, error) {
	return s.repository.GetByID(ctx, userID)
}

func enqueueActivationEmail(
	ctx context.Context,
	tx pgx.Tx,
	to string,
	rawToken string,
) error {
	return outbox.Enqueue(
		ctx,
		tx,
		email.EventTypeActivationEmail,
		email.ActivationEmailEvent{
			To:              to,
			ActivationToken: rawToken,
		},
	)
}

// upgradePasswordHash re-hashes with the current parameters after a
// successful login, the only time the plaintext password is available.
// Failure is logged, not returned: the user has already logged in.
func (s *Service) upgradePasswordHash(
	ctx context.Context,
	userID uuid.UUID,
	password string,
) {
	hash, err := HashPassword(password)
	if err == nil {
		err = s.repository.UpdatePasswordHash(ctx, userID, hash)
	}
	if err != nil {
		slog.Warn("failed to upgrade password hash", "user_id", userID, "error", err)
	}
}
