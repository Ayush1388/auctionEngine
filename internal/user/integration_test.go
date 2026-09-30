package user_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

const testPassword = "correct-horse-battery-staple"

func newService(t *testing.T) (*user.Service, func(query string, args ...any) int) {
	t.Helper()

	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	service := user.NewService(user.NewRepository(pool), jwt)

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
			t.Fatalf("count query: %v", err)
		}
		return n
	}

	return service, count
}

func TestRegisterActivateLogin(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	service := user.NewService(user.NewRepository(pool), jwt)

	registered, err := service.Register(ctx, user.RegisterInput{
		Email:    "alice@example.com",
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The user row and the activation email event are written together.
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM outbox_events
		WHERE event_type = 'email.activation'
	`).Scan(&payload); err != nil {
		t.Fatalf("expected one activation outbox event: %v", err)
	}

	var event struct {
		To              string `json:"to"`
		ActivationToken string `json:"activation_token"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.To != "alice@example.com" || event.ActivationToken == "" {
		t.Fatalf("unexpected event payload: %+v", event)
	}

	// Login is refused until the account is activated.
	_, err = service.Login(ctx, user.LoginInput{Email: "alice@example.com", Password: testPassword})
	if !errors.Is(err, user.ErrAccountNotActivated) {
		t.Fatalf("Login before activation: got %v, want ErrAccountNotActivated", err)
	}

	if err := service.Activate(ctx, event.ActivationToken); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Tokens are single use.
	if err := service.Activate(ctx, event.ActivationToken); !errors.Is(err, user.ErrInvalidActivationToken) {
		t.Fatalf("second Activate: got %v, want ErrInvalidActivationToken", err)
	}

	result, err := service.Login(ctx, user.LoginInput{Email: "alice@example.com", Password: testPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.User.ID != registered.ID || result.AccessToken == "" {
		t.Fatalf("unexpected login result: %+v", result)
	}

	claims, err := jwt.ValidateToken(result.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != registered.ID {
		t.Fatalf("token user = %s, want %s", claims.UserID, registered.ID)
	}

	_, err = service.Login(ctx, user.LoginInput{Email: "alice@example.com", Password: "wrong-password-123"})
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("Login with wrong password: got %v, want ErrInvalidCredentials", err)
	}
}

func TestRegisterDuplicateEmailWritesNothing(t *testing.T) {
	service, count := newService(t)
	ctx := context.Background()
	input := user.RegisterInput{Email: "bob@example.com", Password: testPassword}

	if _, err := service.Register(ctx, input); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	if _, err := service.Register(ctx, input); !errors.Is(err, user.ErrEmailAlreadyExists) {
		t.Fatalf("second Register: got %v, want ErrEmailAlreadyExists", err)
	}

	// The failed registration must not leave a second outbox event behind.
	if n := count("SELECT count(*) FROM users"); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
	if n := count("SELECT count(*) FROM outbox_events"); n != 1 {
		t.Fatalf("outbox_events = %d, want 1", n)
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	service, _ := newService(t)

	_, err := service.Login(context.Background(), user.LoginInput{
		Email:    "nobody@example.com",
		Password: testPassword,
	})
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}
