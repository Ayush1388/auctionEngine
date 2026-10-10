# syntax=docker/dockerfile:1
#
# One image, every binary. Each container picks its program with `command:`
# (see deploy/compose.yml):
#
#   /app/api          HTTP API + WebSockets + outbox and lifecycle workers
#   /app/biddingsvc   the bidding service, over gRPC (v0.9)
#   /app/bidworker    Kafka bid consumers (v0.8)
#   /app/migrate      schema migrations, run once before a release
#   /app/admin        operator commands (promote, reindex)
#
# Why one image: the binaries share all their code, so building them
# together is fast, and every service in a release is guaranteed to run the
# same commit.

# ---------------------------------------------------------------------------
# Stage 1: build
# ---------------------------------------------------------------------------
# $BUILDPLATFORM is the machine running the build. Go cross-compiles, so an
# arm64 image (e.g. for an ARM cloud VM) is built natively on an amd64 CI
# runner without slow CPU emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first, in their own layer: they change far less often than
# the code, so most rebuilds reuse this layer from cache.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64

# CGO_ENABLED=0: fully static binaries, no libc needed at runtime.
# -trimpath: no local file paths baked into the binary.
# -s -w: drop symbol tables and DWARF debug info (about 30% smaller).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ \
      ./cmd/api ./cmd/biddingsvc ./cmd/bidworker ./cmd/migrate ./cmd/admin

# ---------------------------------------------------------------------------
# Stage 2: runtime
# ---------------------------------------------------------------------------
# Alpine rather than "scratch": it has CA certificates (TLS to SMTP and
# managed databases), time zones, and busybox `wget` for container health
# checks. The Go toolchain and source code stay behind in the build stage.
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 app && \
    adduser -S -u 10001 -G app app

WORKDIR /app

COPY --from=build /out/ /app/
COPY migrations /app/migrations
# the demo catalogue (DEMO_SEED=true), the same file the frontend reads
COPY frontend/app/catalog.json /app/catalog.json

# Never run as root: if the process is compromised, the attacker is an
# unprivileged user inside the container.
USER app

# 4000: public HTTP. 9090: internal admin listener (metrics, pprof).
EXPOSE 4000 9090

ENTRYPOINT []
CMD ["/app/api"]
