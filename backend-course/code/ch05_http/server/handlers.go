package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Note is the resource we expose. XMLName lets the same struct be XML.
type Note struct {
	XMLName   xml.Name  `json:"-" xml:"note"`
	ID        int       `json:"id" xml:"id"`
	Text      string    `json:"text" xml:"text"`
	UpdatedAt time.Time `json:"updated_at" xml:"updated_at"`
}

// store is an in-memory "database" guarded by a mutex (many goroutines).
type store struct {
	mu     sync.RWMutex
	nextID int
	notes  map[int]Note
}

func newStore() *store { return &store{nextID: 1, notes: map[int]Note{}} }

type api struct{ s *store }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// routes maps (method, path) -> handler using Go 1.22 ServeMux patterns.
// A path that exists with another method automatically gets 405 + Allow.
func (a *api) routes() http.Handler {
	mux := http.NewServeMux()

	// CRUD on notes: POST create, GET read, PUT replace, PATCH update, DELETE.
	mux.Handle("POST /notes", Idempotent(http.HandlerFunc(a.create))) // safe to retry with Idempotency-Key
	mux.HandleFunc("GET /notes/{id}", a.get)
	mux.HandleFunc("PUT /notes/{id}", a.replace)
	mux.HandleFunc("PATCH /notes/{id}", a.patch)
	mux.HandleFunc("DELETE /notes/{id}", a.remove)

	// Content negotiation, compression, redirects, large payloads, SSE.
	mux.HandleFunc("GET /hello", hello)
	mux.Handle("GET /big", Gzip(http.HandlerFunc(big)))
	mux.HandleFunc("GET /old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusMovedPermanently) // 301
	})
	mux.HandleFunc("GET /campaign", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound) // 302
	})
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "new home") })
	mux.HandleFunc("POST /upload", upload)
	mux.HandleFunc("GET /stream", stream)
	mux.HandleFunc("GET /status/{code}", statusDemo)

	return mux
}

func (a *api) id(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id must be a number") // 400
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // cap body at 1 MB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// POST is NOT idempotent: calling twice creates two notes. -> 201 + Location.
func (a *api) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	a.s.mu.Lock()
	n := Note{ID: a.s.nextID, Text: in.Text, UpdatedAt: time.Now().UTC().Truncate(time.Second)}
	a.s.notes[n.ID] = n
	a.s.nextID++
	a.s.mu.Unlock()

	w.Header().Set("Location", "/notes/"+strconv.Itoa(n.ID))
	writeJSON(w, http.StatusCreated, n)
}

// etag is a strong validator: a hash of the representation we would send.
func etag(n Note) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d", n.ID, n.Text, n.UpdatedAt.Unix())))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// GET is safe + idempotent + cacheable. Conditional GET returns 304.
func (a *api) get(w http.ResponseWriter, r *http.Request) {
	id, ok := a.id(w, r)
	if !ok {
		return
	}
	a.s.mu.RLock()
	n, found := a.s.notes[id]
	a.s.mu.RUnlock()
	if !found {
		writeErr(w, http.StatusNotFound, "note not found") // 404
		return
	}

	tag := etag(n)
	h := w.Header()
	h.Set("ETag", tag)
	h.Set("Last-Modified", n.UpdatedAt.UTC().Format(http.TimeFormat))
	h.Set("Cache-Control", "max-age=10") // fresh for 10s, then revalidate

	// RFC 9110: If-None-Match wins over If-Modified-Since.
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		if inm == tag || inm == "*" {
			w.WriteHeader(http.StatusNotModified) // 304: no body, use your cache
			return
		}
	} else if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil && !n.UpdatedAt.After(t) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	negotiate(w, r, n)
}

// PUT replaces the WHOLE resource. Idempotent: same body 10x == same state.
func (a *api) replace(w http.ResponseWriter, r *http.Request) {
	id, ok := a.id(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if _, found := a.s.notes[id]; !found {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	n := Note{ID: id, Text: in.Text, UpdatedAt: time.Now().UTC().Truncate(time.Second)}
	a.s.notes[id] = n
	writeJSON(w, http.StatusOK, n)
}

// PATCH changes only the fields that were sent (pointer == "was it present?").
func (a *api) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := a.id(w, r)
	if !ok {
		return
	}
	var in struct {
		Text *string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	n, found := a.s.notes[id]
	if !found {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	if in.Text != nil {
		n.Text = *in.Text
	}
	n.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	a.s.notes[id] = n
	writeJSON(w, http.StatusOK, n)
}

// DELETE is idempotent in STATE: after 1 or 10 calls the note is gone.
// (The first returns 204, later ones 404 - the response differs, the state does not.)
func (a *api) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := a.id(w, r)
	if !ok {
		return
	}
	a.s.mu.Lock()
	_, found := a.s.notes[id]
	delete(a.s.notes, id)
	a.s.mu.Unlock()
	if !found {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204: success, nothing to return
}

// negotiate picks JSON or XML from the Accept header.
func negotiate(w http.ResponseWriter, r *http.Request, v Note) {
	accept := r.Header.Get("Accept")
	w.Header().Add("Vary", "Accept")
	switch {
	case strings.Contains(accept, "application/xml"), strings.Contains(accept, "text/xml"):
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_ = xml.NewEncoder(w).Encode(v)
	case accept == "", strings.Contains(accept, "application/json"), strings.Contains(accept, "*/*"):
		writeJSON(w, http.StatusOK, v)
	default:
		writeErr(w, http.StatusNotAcceptable, "supported: application/json, application/xml") // 406
	}
}

// hello shows language negotiation through Accept-Language.
func hello(w http.ResponseWriter, r *http.Request) {
	lang := "en"
	msg := map[string]string{"en": "Hello", "es": "Hola", "hi": "Namaste"}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		code := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		code = strings.SplitN(code, "-", 2)[0]
		if _, ok := msg[code]; ok {
			lang = code
			break
		}
	}
	w.Header().Add("Vary", "Accept-Language")
	w.Header().Set("Content-Language", lang)
	writeJSON(w, http.StatusOK, map[string]string{"lang": lang, "message": msg[lang]})
}

// big writes ~4 MB of repetitive JSON so compression has something to do.
func big(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, "[")
	for i := 0; i < 50000; i++ {
		if i > 0 {
			io.WriteString(w, ",")
		}
		fmt.Fprintf(w, `{"id":%d,"name":"auction item","status":"active"}`, i)
	}
	io.WriteString(w, "]")
}

// upload reads a multipart/form-data body as a STREAM (never the whole file
// in memory) and reports how many bytes each file part contained.
func upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30) // 2 GB hard ceiling
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "expected multipart/form-data: "+err.Error())
		return
	}
	type result struct {
		Field string `json:"field"`
		File  string `json:"filename"`
		Bytes int64  `json:"bytes"`
	}
	var out []result
	for {
		part, err := mr.NextPart() // splits the body on the boundary string
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		n, _ := io.Copy(io.Discard, part) // stream: replace Discard with a file
		out = append(out, result{part.FormName(), part.FileName(), n})
	}
	writeJSON(w, http.StatusCreated, out)
}

// stream pushes Server-Sent Events: one long-lived response, many chunks.
func stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for i := 1; i <= 5; i++ {
		select {
		case <-r.Context().Done(): // client went away: stop working
			return
		case <-tick.C:
			fmt.Fprintf(w, "id: %d\ndata: chunk %d\n\n", i, i) // SSE frame ends with blank line
			fl.Flush()                                         // push it to the client NOW
		}
	}
}

// statusDemo lets you see any status code: GET /status/418
func statusDemo(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 200 || code > 599 {
		writeErr(w, http.StatusBadRequest, "code must be 200-599")
		return
	}
	if code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "5") // tell the client when to come back
	}
	if code == http.StatusNoContent || code == http.StatusNotModified {
		w.WriteHeader(code)
		return
	}
	writeJSON(w, code, map[string]any{"status": code, "text": http.StatusText(code)})
}
