// Package httpx holds the JSON request and response helpers shared by every
// handler, so errors look the same across the API:
//
//	{"error": "message"}
//	{"error": "invalid input", "fields": {"email": "is required"}}
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Ayush1388/auctionEngine/internal/logctx"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

const maxBodyBytes = 1 << 20 // 1 MB

type errorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields,omitempty"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// Error writes {"error": message}.
func Error(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, errorResponse{Error: message})
}

// ValidationError writes a 400 listing what is wrong with each field.
func ValidationError(w http.ResponseWriter, problems *validation.Error) {
	WriteJSON(w, http.StatusBadRequest, errorResponse{
		Error:  "invalid input",
		Fields: problems.Fields,
	})
}

// BadRequest writes a 400 for err. Validation errors keep their field list.
func BadRequest(w http.ResponseWriter, err error) {
	var problems *validation.Error
	if errors.As(err, &problems) {
		ValidationError(w, problems)
		return
	}
	Error(w, http.StatusBadRequest, err.Error())
}

// ServerError logs err with the request and returns a generic 500. Internal
// details such as SQL errors never reach the client.
func ServerError(w http.ResponseWriter, r *http.Request, err error) {
	logctx.From(r.Context()).Error(
		"internal server error",
		"method", r.Method,
		"path", r.URL.Path,
		"error", err,
	)
	Error(w, http.StatusInternalServerError, "internal server error")
}

// ReadJSON decodes exactly one JSON object from the request body into dst.
// It rejects unknown fields, bodies over 1 MB and trailing data, and returns
// errors that are safe to show the client.
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		var maxErr *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("body contains malformed JSON at position %d", syntaxErr.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains malformed JSON")
		case errors.As(err, &typeErr):
			return fmt.Errorf("field %q has the wrong type", typeErr.Field)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return fmt.Errorf("body contains unknown field %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
		case errors.As(err, &maxErr):
			return fmt.Errorf("body must not be larger than %d bytes", maxErr.Limit)
		default:
			return errors.New("body contains malformed JSON")
		}
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}

	return nil
}
