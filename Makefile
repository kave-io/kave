.PHONY: help dev dev-server dev-dashboard build build-server dashboard-build \
        test test-fast lint fmt vet migrate clean

DASHBOARD_DIR := dashboard
SERVER_DIR    := server

help:
	@echo "Kave — Control Plane for AI Agents"
	@echo ""
	@echo "Dev:"
	@echo "  make dev               Run server (hot reload) + dashboard dev server in parallel"
	@echo "  make dev-server        Hot reload Go server only (Air)"
	@echo "  make dev-dashboard     Vite dev server only (port 5173, proxies /api to :8080)"
	@echo ""
	@echo "Build:"
	@echo "  make build             Build dashboard then compile server binary with embedded UI"
	@echo "  make build-server      Compile server binary only (assumes dashboard already built)"
	@echo "  make dashboard-build   Build dashboard into server/ui/dist/"
	@echo ""
	@echo "Quality:"
	@echo "  make test              Run all tests"
	@echo "  make test-fast         Run tests (cached)"
	@echo "  make lint              staticcheck on all Go modules"
	@echo "  make fmt               gofmt all Go files"
	@echo "  make vet               go vet all modules"
	@echo ""
	@echo "Ops:"
	@echo "  make migrate           Run database migrations"
	@echo "  make clean             Remove binaries, dist, and cache"

# ── Dev ───────────────────────────────────────────────────────────────────────

dev:
	@command -v air >/dev/null 2>&1 || (echo "air not installed: go install github.com/air-verse/air@latest" && exit 1)
	@command -v bun >/dev/null 2>&1 || (echo "bun not installed: https://bun.sh" && exit 1)
	@echo "Starting server (Air) + dashboard (Vite)..."
	@trap 'kill 0' INT; \
	  (cd $(SERVER_DIR) && air) & \
	  (cd $(DASHBOARD_DIR) && bun run dev) & \
	  wait

dev-server:
	@command -v air >/dev/null 2>&1 || (echo "air not installed: go install github.com/air-verse/air@latest" && exit 1)
	cd $(SERVER_DIR) && air

dev-dashboard:
	@command -v bun >/dev/null 2>&1 || (echo "bun not installed: https://bun.sh" && exit 1)
	cd $(DASHBOARD_DIR) && bun run dev

# ── Build ─────────────────────────────────────────────────────────────────────

build: dashboard-build build-server

build-server:
	cd $(SERVER_DIR) && go build -o kave-server .
	@echo "Built: server/kave-server"

dashboard-build:
	@command -v bun >/dev/null 2>&1 || (echo "bun not installed: https://bun.sh" && exit 1)
	cd $(DASHBOARD_DIR) && bun run build
	@echo "Dashboard built -> server/ui/dist/"

# ── Test ─────────────────────────────────────────────────────────────────────

test:
	cd core       && go test -v ./...
	cd server     && go test -v ./...
	cd connectors && go test -v ./...

test-fast:
	cd core       && go test -count=1 ./...
	cd server     && go test -count=1 ./...
	cd connectors && go test -count=1 ./...

# ── Quality ───────────────────────────────────────────────────────────────────

lint:
	@command -v staticcheck >/dev/null 2>&1 || (echo "staticcheck not installed: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	cd core       && staticcheck ./...
	cd server     && staticcheck ./...
	cd connectors && staticcheck ./...
	cd cli        && staticcheck ./...

fmt:
	cd core       && gofmt -w -l .
	cd server     && gofmt -w -l .
	cd connectors && gofmt -w -l .
	cd cli        && gofmt -w -l .
	@echo "Formatted all Go files"

vet:
	cd core       && go vet ./...
	cd server     && go vet ./...
	cd connectors && go vet ./...
	cd cli        && go vet ./...

# ── Ops ───────────────────────────────────────────────────────────────────────

migrate:
	cd $(SERVER_DIR) && go run . 2>&1 | head -5 || true

clean:
	rm -f server/kave-server cli/kave-cli
	rm -rf server/ui/dist/*
	find . -name "*.test" -type f -delete
	go clean -testcache
	@echo "Cleaned"
