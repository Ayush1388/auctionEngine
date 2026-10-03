package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStatusCodes(t *testing.T) {
	h := newHandler()
	if c := do(t, h, "POST", "/notes", `{"text":"a"}`, nil).Code; c != 201 {
		t.Fatalf("create=%d want 201", c)
	}
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/notes/1", "", 200},
		{"GET", "/notes/99", "", 404},
		{"GET", "/notes/abc", "", 400},
		{"POST", "/notes", `{"text":""}`, 400},
		{"POST", "/notes", `{"nope":1}`, 400}, // unknown field rejected
		{"DELETE", "/notes", "", 405},         // path exists, wrong method
		{"GET", "/old", "", 301},
		{"GET", "/campaign", "", 302},
		{"GET", "/status/418", "", 418},
	}
	for _, c := range cases {
		if got := do(t, h, c.method, c.path, c.body, nil).Code; got != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

func TestIdempotency(t *testing.T) {
	h := newHandler()
	// POST twice -> two different resources (NOT idempotent).
	a := do(t, h, "POST", "/notes", `{"text":"x"}`, nil).Header().Get("Location")
	b := do(t, h, "POST", "/notes", `{"text":"x"}`, nil).Header().Get("Location")
	if a == b {
		t.Fatalf("POST should create distinct resources, got %s twice", a)
	}
	// PUT twice -> same final state (idempotent).
	do(t, h, "PUT", a, `{"text":"final"}`, nil)
	first := do(t, h, "GET", a, "", nil).Body.String()
	do(t, h, "PUT", a, `{"text":"final"}`, nil)
	second := do(t, h, "GET", a, "", nil).Body.String()
	if first != second {
		t.Fatalf("PUT changed state on repeat:\n%s\n%s", first, second)
	}
	// DELETE twice -> note is gone either way (204 then 404).
	if c := do(t, h, "DELETE", a, "", nil).Code; c != 204 {
		t.Fatalf("first delete=%d", c)
	}
	if c := do(t, h, "DELETE", a, "", nil).Code; c != 404 {
		t.Fatalf("second delete=%d", c)
	}
}

func TestPatchVsPut(t *testing.T) {
	h := newHandler()
	do(t, h, "POST", "/notes", `{"text":"orig"}`, nil)
	do(t, h, "PATCH", "/notes/1", `{}`, nil) // nothing sent -> text must stay
	if !strings.Contains(do(t, h, "GET", "/notes/1", "", nil).Body.String(), `"orig"`) {
		t.Fatal("PATCH with no fields must not erase text")
	}
}

func TestConditionalGet(t *testing.T) {
	h := newHandler()
	do(t, h, "POST", "/notes", `{"text":"cache me"}`, nil)
	first := do(t, h, "GET", "/notes/1", "", nil)
	tag := first.Header().Get("ETag")
	if tag == "" || first.Header().Get("Cache-Control") == "" {
		t.Fatal("missing ETag/Cache-Control")
	}
	if c := do(t, h, "GET", "/notes/1", "", map[string]string{"If-None-Match": tag}); c.Code != 304 || c.Body.Len() != 0 {
		t.Fatalf("matching ETag: code=%d body=%d", c.Code, c.Body.Len())
	}
	do(t, h, "PUT", "/notes/1", `{"text":"changed"}`, nil) // resource changes -> new ETag
	if c := do(t, h, "GET", "/notes/1", "", map[string]string{"If-None-Match": tag}); c.Code != 200 {
		t.Fatalf("stale ETag should get 200, got %d", c.Code)
	}
}

func TestContentNegotiation(t *testing.T) {
	h := newHandler()
	do(t, h, "POST", "/notes", `{"text":"n"}`, nil)
	x := do(t, h, "GET", "/notes/1", "", map[string]string{"Accept": "application/xml"})
	if x.Header().Get("Content-Type") != "application/xml" || !strings.Contains(x.Body.String(), "<note>") {
		t.Fatalf("xml negotiation failed: %s", x.Body.String())
	}
	if c := do(t, h, "GET", "/notes/1", "", map[string]string{"Accept": "image/png"}).Code; c != 406 {
		t.Fatalf("unsupported Accept = %d, want 406", c)
	}
	es := do(t, h, "GET", "/hello", "", map[string]string{"Accept-Language": "es-ES,es;q=0.9"})
	if !strings.Contains(es.Body.String(), "Hola") {
		t.Fatalf("language negotiation failed: %s", es.Body.String())
	}
}

func TestCORS(t *testing.T) {
	h := newHandler()
	const origin = "http://localhost:5173"

	// Simple request from an allowed origin.
	r := do(t, h, "GET", "/hello", "", map[string]string{"Origin": origin})
	if r.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatal("allowed origin must be echoed")
	}
	// Disallowed origin: no CORS header -> the browser blocks it.
	r = do(t, h, "GET", "/hello", "", map[string]string{"Origin": "http://evil.example"})
	if r.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin must NOT get the header")
	}
	// Pre-flight.
	r = do(t, h, "OPTIONS", "/notes/1", "", map[string]string{
		"Origin":                         origin,
		"Access-Control-Request-Method":  "PUT",
		"Access-Control-Request-Headers": "authorization",
	})
	if r.Code != 204 || r.Header().Get("Access-Control-Max-Age") == "" ||
		!strings.Contains(r.Header().Get("Access-Control-Allow-Methods"), "PUT") ||
		!strings.Contains(r.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("bad preflight: %d %v", r.Code, r.Header())
	}
}

func TestGzip(t *testing.T) {
	h := newHandler()
	plain := do(t, h, "GET", "/big", "", nil)
	zipped := do(t, h, "GET", "/big", "", map[string]string{"Accept-Encoding": "gzip"})
	if zipped.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected gzip")
	}
	if zipped.Body.Len()*10 > plain.Body.Len() {
		t.Fatalf("compression too weak: %d -> %d", plain.Body.Len(), zipped.Body.Len())
	}
	zipLen := zipped.Body.Len()
	zr, err := gzip.NewReader(zipped.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if len(got) != plain.Body.Len() {
		t.Fatal("decompressed size differs from plain response")
	}
	t.Logf("plain=%d bytes gzip=%d bytes", plain.Body.Len(), zipLen)
}

func TestMultipartUpload(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf) // generates the boundary string
	fw, _ := mw.CreateFormFile("photo", "car.jpg")
	fw.Write(bytes.Repeat([]byte("x"), 5000))
	mw.Close()

	req := httptest.NewRequest("POST", "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, req)
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"bytes":5000`) {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSSE(t *testing.T) {
	srv := httptest.NewServer(newHandler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content-type=%s", resp.Header.Get("Content-Type"))
	}
	sc := bufio.NewScanner(resp.Body)
	chunks := 0
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data: chunk") {
			chunks++
		}
	}
	if chunks != 5 {
		t.Fatalf("got %d chunks, want 5", chunks)
	}
}

func TestIdempotencyKey(t *testing.T) {
	h := newHandler()
	key := map[string]string{"Idempotency-Key": "abc-123"}
	first := do(t, h, "POST", "/notes", `{"text":"pay once"}`, key)
	retry := do(t, h, "POST", "/notes", `{"text":"pay once"}`, key) // user double-clicked / network retry
	if first.Code != 201 || retry.Code != 201 || first.Body.String() != retry.Body.String() {
		t.Fatalf("retry must replay the first response: %d %s | %d %s", first.Code, first.Body.String(), retry.Code, retry.Body.String())
	}
	if retry.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatal("replay marker missing")
	}
	// Only ONE note exists: the next create gets id 2, not 3.
	next := do(t, h, "POST", "/notes", `{"text":"other"}`, nil)
	if !strings.Contains(next.Body.String(), `"id":2`) {
		t.Fatalf("duplicate was executed twice: %s", next.Body.String())
	}
}
