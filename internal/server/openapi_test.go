package server

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Ayush1388/auctionEngine/api"
)

// The OpenAPI document and the router must describe the same API. Adding a
// route without documenting it (or the reverse) fails here.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(api.OpenAPI, &doc); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}

	var documented []string
	for path, ops := range doc.Paths {
		for method := range ops {
			documented = append(documented, strings.ToUpper(method)+" "+path)
		}
	}

	routed := Patterns()
	slices.Sort(documented)
	slices.Sort(routed)

	for _, r := range routed {
		if !slices.Contains(documented, r) {
			t.Errorf("route %q is not in api/openapi.json", r)
		}
	}
	for _, d := range documented {
		if !slices.Contains(routed, d) {
			t.Errorf("api/openapi.json documents %q, which the router doesn't serve", d)
		}
	}
}
