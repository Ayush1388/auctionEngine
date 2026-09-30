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
		return err
	}

	if existingUser.ActivatedAt != nil {
		return ErrUserAlreadyActivated
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
			return LoginResult{}, ErrInvalidCredentials
		}

		return LoginResult{}, fmt.Errorf(
			"failed to find user: %w",
			err,
		)
	}

	if existingUser.ActivatedAt == nil {
		return LoginResult{}, ErrAccountNotActivated
	}

	// CheckPassword returns an error when the password
	// does not match or the stored hash is invalid.
	if err := CheckPassword(
		input.Password,
		existingUser.PasswordHash,
	); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	accessToken, err := s.jwt.GenerateToken(
		existingUser.ID,
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
