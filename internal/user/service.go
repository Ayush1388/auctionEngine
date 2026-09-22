package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type RegisterInput struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=15,max=128"`
}

type ResendActivationInput struct {
	Email string `validate:"required,email"`
}

type Service struct {
	repository *Repository
	validator  *validator.Validate
}

func NewService(
	repository *Repository,
) *Service {
	return &Service{
		repository: repository,
		validator:  validator.New(),
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
