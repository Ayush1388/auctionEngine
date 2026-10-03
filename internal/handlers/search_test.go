package handlers_test

import (
	"net/http"
	"testing"
	"time"
)

func TestSearchHTTP(t *testing.T) {
	a := newAPI(t)
	_, token := a.newUser()

	created := a.createAuction(token, time.Hour, time.Hour) // "Vintage camera"

	w, body := a.request("GET", "/v1/auctions/search?q=camera", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("search: %d %v", w.Code, body)
	}
	results := body["auctions"].([]any)
	if len(results) != 1 || results[0].(map[string]any)["id"] != created["id"] {
		t.Fatalf("results = %v", results)
	}
	if body["backend"] != "postgres" || body["next_cursor"] != nil {
		t.Fatalf("unexpected envelope: %v", body)
	}

	if w, _ := a.request("GET", "/v1/auctions/search?q=", "", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("empty query: %d", w.Code)
	}
	if w, _ := a.request("GET", "/v1/auctions/search?q=camera&cursor=not-base64!", "", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("garbage cursor: %d", w.Code)
	}

	w, body = a.request("GET", "/v1/auctions/suggest?q=vi", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("suggest: %d %v", w.Code, body)
	}
	// The auction isn't active yet, so nothing is suggested, and the result
	// is [] not null.
	if string(mustJSON(t, body["suggestions"])) != "[]" {
		t.Fatalf("suggestions = %v", body["suggestions"])
	}
}
