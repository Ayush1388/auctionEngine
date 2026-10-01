# Common tasks. Run `make help` to list them.

GO        ?= go
GOBIN     ?= $(shell $(GO) env GOPATH)/bin
PROTO_DIR := api/proto
GEN_DIR   := internal/gen

.PHONY: help build test test-race lint proto proto-tools migrate run infra infra-down up down e2e rollout logs

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

build: ## build every binary into ./bin
	$(GO) build -o bin/ ./cmd/...

test: ## unit + integration tests (integration needs TEST_DATABASE_URL etc.)
	$(GO) test ./...

test-race: ## the full suite under the race detector, as CI runs it
	$(GO) test -race -count=1 ./...

lint: ## formatting and vet
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	$(GO) vet ./...

proto-tools: ## install the protobuf code generators
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

proto: proto-tools ## regenerate gRPC code from api/proto
	@if [ -z "$(USE_PROTOGEN)" ] && command -v protoc >/dev/null; then \
		protoc -I $(PROTO_DIR) --go_out=$(GEN_DIR)/biddingv1 --go_opt=paths=source_relative \
		  --go-grpc_out=$(GEN_DIR)/biddingv1 --go-grpc_opt=paths=source_relative bidding/v1/bidding.proto; \
		mv $(GEN_DIR)/biddingv1/bidding/v1/*.go $(GEN_DIR)/biddingv1/ && rm -r $(GEN_DIR)/biddingv1/bidding; \
	else \
		cd tools/protogen && $(GO) run . -I ../../$(PROTO_DIR) \
		  -plugin go=$(GOBIN)/protoc-gen-go:paths=source_relative \
		  -plugin grpc=$(GOBIN)/protoc-gen-go-grpc:paths=source_relative \
		  -out /tmp/auctionengine-gen bidding/v1/bidding.proto && \
		cp /tmp/auctionengine-gen/bidding/v1/*.go ../../$(GEN_DIR)/biddingv1/; \
	fi

migrate: ## apply database migrations
	$(GO) run ./cmd/migrate

run: ## run the API (reads .env)
	$(GO) run ./cmd/api

infra: ## start only the dependencies (Postgres, Redis, Elasticsearch, Kafka) for `make run`
	docker compose up -d

infra-down: ## stop the dependencies
	docker compose down

up: ## deploy the full production stack locally (needs deploy/.env, see deploy/.env.example)
	docker compose -f deploy/compose.yml up -d --build --wait

down: ## stop the full stack
	docker compose -f deploy/compose.yml down

logs: ## follow the full stack's logs
	docker compose -f deploy/compose.yml logs -f --tail 50

e2e: ## end-to-end test against the running full stack
	$(GO) test -tags e2e -v -count=1 -timeout 15m ./e2e

rollout: ## zero-downtime deploy of a new image (IMAGE=…)
	./deploy/rollout.sh
