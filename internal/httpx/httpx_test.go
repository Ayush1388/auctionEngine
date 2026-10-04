package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ayush1388/auctionEngine/internal/validation"
)

func TestReadJSON(t *testing.T) {
	type body struct {
		Name  string `json:"name"`
		Price int64  `json:"price"`
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"valid", `{"name":"lamp","price":10}`, ""},
		{"empty", ``, "body must not be empty"},
		{"malformed", `{"name":`, "body contains malformed JSON"},
		{"wrong type", `{"price":"ten"}`, `field "price" has the wrong type`},
		{"unknown field", `{"colour":"red"}`, `body contains unknown field "colour"`},
		{"two objects", `{"name":"a"}{"name":"b"}`, "body must contain a single JSON object"},
		{"too large", `{"name":"` + strings.Repeat("x", maxBodyBytes) + `"}`, "body must not be larger than"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.input))
			w := httptest.NewRecorder()

			var dst body
			err := ReadJSON(w, r, &dst)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestServerErrorHidesDetails(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	ServerError(w, r, errors.New(`ERROR: relation "users" does not exist`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "relation") {
		t.Fatalf("internal error leaked to client: %s", w.Body.String())
	}
}

func TestBadRequestKeepsFields(t *testing.T) {
	w := httptest.NewRecorder()
	problems := &validation.Error{}
	problems.Add("email", "is required")

	BadRequest(w, problems)

	var got errorResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusBadRequest || got.Fields["email"] != "is required" {
		t.Fatalf("got %d %+v", w.Code, got)
	}
}
