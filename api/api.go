// Package api holds the machine-readable API contract.
//
// openapi.json is an OpenAPI 3.1 document: a standard, tool-readable
// description of every endpoint, request body, response and error. From it,
// tools can render interactive docs (Swagger UI, Redoc), generate client
// SDKs, and validate requests. It's embedded into the binary and served at
// GET /v1/openapi.json. A test (server/openapi_test.go) fails if a route is
// added to the router but not documented here, or the other way round.
package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var OpenAPI []byte

// ServeOpenAPI writes the OpenAPI document.
func ServeOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(OpenAPI)
}
