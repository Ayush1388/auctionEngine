package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/auth"
	"github.com/Ayush1388/auctionEngine/internal/handlers"
	"github.com/Ayush1388/auctionEngine/internal/server"
	"github.com/Ayush1388/auctionEngine/internal/testdb"
	"github.com/Ayush1388/auctionEngine/internal/user"
)

type api struct {
	t       *testing.T
	pool    *pgxpool.Pool
	handler http.Handler
	jwt     *user.JWTService
	clock   *time.Time
}

// newAPI wires the real router, services and a fresh database schema.
func newAPI(t *testing.T) *api {
	t.Helper()

	pool := testdb.New(t)
	jwt := user.NewJWTService("0123456789abcdef0123456789abcdef", "test", time.Hour)

	clock := time.Now().UTC()
	auctionService := auction.NewService(pool, auction.NewRepository(pool))
	a := &api{t: t, pool: pool, jwt: jwt, clock: &clock}
	auctionService.SetClock(func() time.Time { return *a.clock })

	a.handler = server.Routes(
		handlers.NewUserHandler(user.NewService(pool, user.NewRepository(pool), jwt)),
		handlers.NewAuctionHandler(auctionService),
		auth.NewMiddleware(jwt),
	)
	return a
}

// newUser inserts an activated user and returns its ID and a bearer token.
func (a *api) newUser() (uuid.UUID, string) {
	a.t.Helper()

	id := uuid.New()
	if _, err := a.pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, activated_at) VALUES ($1, $2, 'x', now())`,
		id, id.String()+"@example.com",
	); err != nil {
		a.t.Fatal(err)
	}

	token, err := a.jwt.GenerateToken(id)
	if err != nil {
		a.t.Fatal(err)
	}
	return id, token
}

func (a *api) request(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	a.t.Helper()

	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)

	return w, decode(a.t, w)
}

func (a *api) createAuction(token string, startsIn, duration time.Duration) map[string]any {
	a.t.Helper()

	starts := a.clock.Add(startsIn)
	w, body := a.request("POST", "/v1/auctions", token, auctionBody(starts, starts.Add(duration)))
	if w.Code != http.StatusCreated {
		a.t.Fatalf("create auction: %d %v", w.Code, body)
	}
	return body
}

func auctionBody(starts, ends time.Time) string {
	return `{
		"item": {"name": "Vintage camera", "type": "electronics", "description": "Works"},
		"starting_price": 50000,
		"starts_at": "` + starts.Format(time.RFC3339Nano) + `",
		"ends_at": "` + ends.Format(time.RFC3339Nano) + `"
	}`
}

func TestCreateAuctionHTTP(t *testing.T) {
	a := newAPI(t)
	ownerID, token := a.newUser()
	starts := a.clock.Add(time.Hour)

	w, body := a.request("POST", "/v1/auctions", "", auctionBody(starts, starts.Add(time.Hour)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d %v", w.Code, body)
	}

	w, body = a.request("POST", "/v1/auctions", token, auctionBody(starts, starts.Add(time.Hour)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %v", w.Code, body)
	}
	if body["owner_id"] != ownerID.String() || body["status"] != "NOT_ACTIVE" || body["current_bid"] != nil {
		t.Fatalf("unexpected body: %v", body)
	}
	if loc := w.Header().Get("Location"); loc != "/v1/auctions/"+body["id"].(string) {
		t.Fatalf("Location = %q", loc)
	}

	w, body = a.request("POST", "/v1/auctions", token, auctionBody(starts, starts.Add(-time.Hour)))
	fields, _ := body["fields"].(map[string]any)
	if w.Code != http.StatusBadRequest || fields["ends_at"] == nil {
		t.Fatalf("invalid times: %d %v", w.Code, body)
	}
}

func TestCreateAuctionIgnoresOwnerInBody(t *testing.T) {
	a := newAPI(t)
	_, token := a.newUser()
	starts := a.clock.Add(time.Hour)

	body := strings.Replace(auctionBody(starts, starts.Add(time.Hour)), `"item"`, `"owner_id": "`+uuid.NewString()+`", "item"`, 1)
	w, _ := a.request("POST", "/v1/auctions", token, body)

	// Unknown fields are rejected outright, so there is no way to set the owner.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 for owner_id in body", w.Code)
	}
}

func TestGetAuctionHTTP(t *testing.T) {
	a := newAPI(t)
	_, token := a.newUser()
	created := a.createAuction(token, time.Hour, time.Hour)
	id := created["id"].(string)

	// Public: no token needed.
	w, body := a.request("GET", "/v1/auctions/"+id, "", "")
	if w.Code != http.StatusOK || body["id"] != id {
		t.Fatalf("get: %d %v", w.Code, body)
	}
	item, _ := body["item"].(map[string]any)
	if item["name"] != "Vintage camera" {
		t.Fatalf("item missing from response: %v", body)
	}

	w, _ = a.request("GET", "/v1/auctions/"+uuid.NewString(), "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", w.Code)
	}

	w, _ = a.request("GET", "/v1/auctions/not-a-uuid", "", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed id: %d", w.Code)
	}
}

func TestListAuctionsHTTP(t *testing.T) {
	a := newAPI(t)
	_, aliceToken := a.newUser()
	_, bobToken := a.newUser()

	for i := 0; i < 3; i++ {
		a.createAuction(aliceToken, time.Hour, time.Hour)
	}
	a.createAuction(bobToken, time.Hour, time.Hour)

	w, body := a.request("GET", "/v1/auctions?limit=2", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %v", w.Code, body)
	}
	if got := len(body["auctions"].([]any)); got != 2 || body["next_cursor"] == nil {
		t.Fatalf("first page: %d auctions, next_cursor=%v", got, body["next_cursor"])
	}

	w, body = a.request("GET", "/v1/auctions?limit=2&cursor="+body["next_cursor"].(string), "", "")
	if got := len(body["auctions"].([]any)); w.Code != http.StatusOK || got != 2 || body["next_cursor"] != nil {
		t.Fatalf("last page: %d, %d auctions, next_cursor=%v", w.Code, got, body["next_cursor"])
	}

	w, body = a.request("GET", "/v1/auctions?owner=me", aliceToken, "")
	if got := len(body["auctions"].([]any)); w.Code != http.StatusOK || got != 3 {
		t.Fatalf("owner=me: %d, %d auctions", w.Code, got)
	}

	w, _ = a.request("GET", "/v1/auctions?owner=me", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("owner=me without token: %d", w.Code)
	}

	w, _ = a.request("GET", "/v1/auctions", "not-a-real-token", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token on a public route should still be rejected: %d", w.Code)
	}

	w, body = a.request("GET", "/v1/auctions?status=nope&limit=500", "", "")
	fields, _ := body["fields"].(map[string]any)
	if w.Code != http.StatusBadRequest || fields["status"] == nil || fields["limit"] == nil {
		t.Fatalf("bad params: %d %v", w.Code, body)
	}

	w, body = a.request("GET", "/v1/auctions?status=CANCELLED", "", "")
	if w.Code != http.StatusOK || string(mustJSON(t, body["auctions"])) != "[]" {
		t.Fatalf("empty result should be [] not null: %v", body)
	}
}

func TestCancelAuctionHTTP(t *testing.T) {
	a := newAPI(t)
	_, ownerToken := a.newUser()
	_, otherToken := a.newUser()

	id := a.createAuction(ownerToken, time.Hour, time.Hour)["id"].(string)
	path := "/v1/auctions/" + id + "/cancel"

	if w, _ := a.request("POST", path, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w, _ := a.request("POST", path, otherToken, ""); w.Code != http.StatusForbidden {
		t.Fatalf("not the owner: %d", w.Code)
	}
	if w, _ := a.request("POST", "/v1/auctions/"+uuid.NewString()+"/cancel", ownerToken, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown auction: %d", w.Code)
	}

	w, body := a.request("POST", path, ownerToken, "")
	if w.Code != http.StatusOK || body["status"] != "CANCELLED" {
		t.Fatalf("owner: %d %v", w.Code, body)
	}

	if w, _ := a.request("POST", path, ownerToken, ""); w.Code != http.StatusConflict {
		t.Fatalf("cancel twice: %d", w.Code)
	}
}
