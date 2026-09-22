package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrAccountNotActivated = errors.New("account is not activated")
)

type RegisterInput struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=15,max=128"`
}

type ResendActivationInput struct {
	Email string `validate:"required,email"`
}

type LoginInput struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required"`
}

type LoginResult struct {
	User        User
	AccessToken string
}

type Service struct {
	repository *Repository
	validator  *validator.Validate
	jwt        *JWTService
}

func NewService(
	repository *Repository,
	jwtService *JWTService,
) *Service {
	return &Service{
		repository: repository,
		validator:  validator.New(),
		jwt:        jwtService,
	}
}

func (s *Service) Register(
	ctx context.Context,
	input RegisterInput,
) (User, error) {
	input.Email = strings.TrimSpace(input.Email)

	if err := s.validator.Struct(input); err != nil {
		return User{}, fmt.Errorf(
			"invalid registration input: %w",
			err,
		)
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

	activationTokenExpiresAt := time.Now().Add(24 * time.Hour)

	newUser := User{
		ID:           uuid.New(),
		Email:        input.Email,
		PasswordHash: passwordHash,
	}

	err = s.repository.Create(
		ctx,
		newUser,
		tokenHash,
		activationTokenExpiresAt,
		rawToken,
	)
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
		return fmt.Errorf(
			"invalid resend activation input: %w",
			err,
		)
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

	activationTokenExpiresAt := time.Now().Add(24 * time.Hour)

	if err := s.repository.UpdateActivationToken(
		ctx,
		existingUser.ID,
		tokenHash,
		activationTokenExpiresAt,
		rawToken,
		existingUser.Email,
	); err != nil {
		return err
	}

	return nil
}

func (s *Service) Login(
	ctx context.Context,
	input LoginInput,
) (LoginResult, error) {
	input.Email = strings.TrimSpace(input.Email)

	if err := s.validator.Struct(input); err != nil {
		return LoginResult{}, fmt.Errorf(
			"invalid login input: %w",
			err,
		)
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
