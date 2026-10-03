// Package e2e holds the end-to-end test: it drives a fully deployed stack
// (deploy/compose.yml) from the outside, exactly like a real client, and
// checks every part of the system worked together.
//
// It only builds with the "e2e" tag, so `go test ./...` never runs it:
//
//	docker compose -f deploy/compose.yml up -d --build --wait
//	go test -tags e2e -v -count=1 ./e2e
//
// CI runs it on every pull request (.github/workflows/ci.yml, job "e2e").
package e2e
