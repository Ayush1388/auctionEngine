package user_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

const testPassword = "correct-horse-battery-staple"

func newService(t *testing.T) (*user.Service, func(query string, args ...any) int) {
	t.Helper()

	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)
	service := user.NewService(pool, user.NewRepository(pool), jwt)

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
	service := user.NewService(pool, user.NewRepository(pool), jwt)

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

	// A wrong password on an unactivated account says nothing about the
	// account's state.
	_, err = service.Login(ctx, user.LoginInput{Email: "alice@example.com", Password: "wrong-password-123"})
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("wrong password before activation: got %v, want ErrInvalidCredentials", err)
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

func TestRegisterRollsBackUserWhenOutboxFails(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	service := user.NewService(pool, user.NewRepository(pool), user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour))

	// Make the outbox insert fail after the user insert has succeeded.
	if _, err := pool.Exec(ctx, "ALTER TABLE outbox_events RENAME TO outbox_events_gone"); err != nil {
		t.Fatal(err)
	}

	_, err := service.Register(ctx, user.RegisterInput{Email: "carol@example.com", Password: testPassword})
	if err == nil {
		t.Fatal("expected Register to fail when the outbox is unavailable")
	}

	var users int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("user row survived a failed registration (users = %d)", users)
	}
}

func TestResendActivationDoesNotRevealAccounts(t *testing.T) {
	service, count := newService(t)
	ctx := context.Background()

	if err := service.ResendActivation(ctx, user.ResendActivationInput{Email: "ghost@example.com"}); err != nil {
		t.Fatalf("unknown email: got %v, want nil", err)
	}
	if n := count("SELECT count(*) FROM outbox_events"); n != 0 {
		t.Fatalf("an email was queued for an unknown address (%d events)", n)
	}

	if _, err := service.Register(ctx, user.RegisterInput{Email: "frank@example.com", Password: testPassword}); err != nil {
		t.Fatal(err)
	}
	if err := service.ResendActivation(ctx, user.ResendActivationInput{Email: "frank@example.com"}); err != nil {
		t.Fatalf("resend: %v", err)
	}
	if n := count("SELECT count(*) FROM outbox_events"); n != 2 {
		t.Fatalf("outbox_events = %d, want 2 (register + resend)", n)
	}
}

func TestLoginUpgradesWeakPasswordHash(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	service := user.NewService(pool, user.NewRepository(pool), user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour))

	// A hash made with the minimum parameters, as if created by an older
	// version with a lower cost.
	weak := weakHash(t, testPassword)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES (gen_random_uuid(), 'gina@example.com', $1, now())`,
		weak,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Login(ctx, user.LoginInput{Email: "gina@example.com", Password: testPassword}); err != nil {
		t.Fatalf("login with old hash: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = 'gina@example.com'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == weak || user.NeedsRehash(stored) {
		t.Fatal("hash was not upgraded after login")
	}

	if _, err := service.Login(ctx, user.LoginInput{Email: "gina@example.com", Password: testPassword}); err != nil {
		t.Fatalf("login after upgrade: %v", err)
	}
}

func weakHash(t *testing.T, password string) string {
	t.Helper()

	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=1,p=1$%s$%s", 8*1024,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}
