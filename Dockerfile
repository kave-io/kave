# syntax=docker/dockerfile:1
# Full standalone build — produces both server and cli images as named targets.
# Usage:
#   docker build --target server -t kave-server .
#   docker build --target cli   -t kave-cli .

# ── Stage 1: dashboard ────────────────────────────────────────────────────────
FROM oven/bun:1.2 AS dashboard
WORKDIR /build/dashboard
COPY dashboard/package.json dashboard/bun.lockb* ./
RUN bun install --frozen-lockfile
COPY dashboard/ .
# vite.config outDir is ../server/ui/dist (relative to dashboard/), so output
# lands at /build/server/ui/dist inside this stage.
RUN bun run build

# ── Stage 2: Go builder ───────────────────────────────────────────────────────
FROM golang:1.26-bookworm AS builder
WORKDIR /build
RUN apt-get update && apt-get install -y --no-install-recommends gcc && \
    rm -rf /var/lib/apt/lists/*

# Module files (cache layer — invalidated only on dep changes)
COPY core/go.mod core/go.sum ./core/
COPY server/go.mod server/go.sum ./server/
COPY cli/go.mod cli/go.sum ./cli/
COPY proto/gen/go.mod proto/gen/go.sum ./proto/gen/
COPY cmd/lint-architecture/go.mod cmd/lint-architecture/go.sum ./cmd/lint-architecture/

# Copy source
COPY core/ ./core/
COPY server/ ./server/
COPY cli/ ./cli/
COPY proto/ ./proto/
COPY cmd/ ./cmd/

# Dashboard output embedded into the server binary via //go:embed all:dist
COPY --from=dashboard /build/server/ui/dist ./server/ui/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Server: CGO required (DuckDB + sqlite3 use pre-built static libs linked via cgo)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd server && CGO_ENABLED=1 go build \
    -ldflags="-s -w -X main.buildVersion=${VERSION}" \
    -o /out/kave-server .

# CLI: pure Go, no CGO needed
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd cli && CGO_ENABLED=0 go build \
    -ldflags="-s -w \
      -X github.com/kave-io/kave/cli/internal/version.Version=${VERSION} \
      -X github.com/kave-io/kave/cli/internal/version.Commit=${COMMIT} \
      -X github.com/kave-io/kave/cli/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/kave .

# ── Stage 3: server image ─────────────────────────────────────────────────────
# The server uses CGO through DuckDB and sqlite3, so the runtime image must
# include glibc. distroless/static is only safe for the pure-Go CLI image.
FROM gcr.io/distroless/base-debian12 AS server
COPY --from=builder /out/kave-server /usr/local/bin/kave-server
EXPOSE 18080 19090
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/kave-server"]

# ── Stage 4: cli image ────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS cli
COPY --from=builder /out/kave /usr/local/bin/kave
ENTRYPOINT ["/usr/local/bin/kave"]
