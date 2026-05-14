.PHONY: help dev dev-server dev-dashboard serve build build-server dashboard-build \
        test test-fast lint fmt vet migrate clean cli-docs \
        kave-local-install kave-local-uninstall kave-local-reinstall \
        buf-build buf-up buf-down buf-shell buf-version buf-lint buf-format-check \
        buf-format buf-generate buf-breaking buf-test buf-clean buf-quick-dev
.NOTPARALLEL: kave-local-install kave-local-uninstall kave-local-reinstall

DASHBOARD_DIR := dashboard
SERVER_DIR    := server
BUF_COMPOSE_FILE := compose.buf.yaml
BUF_CONTAINER := buf

help:
	@echo "Kave — Control Plane for AI Agents"
	@echo ""
	@echo "Dev:"
	@echo "  make dev               Run server (hot reload) + dashboard dev server in parallel"
	@echo "  make dev-server        Hot reload Go server only (Air)"
	@echo "  make dev-dashboard     Vite dev server only (port 5173, proxies /api to :8080)"
	@echo "  make serve             Run the Go server with HTTP + gRPC"
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
	@echo "  make cli-docs          Regenerate Cobra CLI docs"
	@echo ""
	@echo "Ops:"
	@echo "  make migrate           Run database migrations"
	@echo "  make clean             Remove binaries, dist, and cache"
	@echo ""
	@echo "Local Install:"
	@echo "  make kave-local-install    Build/install user binaries, write ~/.kave/kave.yaml, and enable user systemd service"
	@echo "  make kave-local-uninstall  Stop/disable service and remove installed files for a fresh reinstall"
	@echo "  make kave-local-reinstall  Uninstall then install in one command"

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

serve:
	cd $(SERVER_DIR) && go run .

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
	cd cli        && go test -v ./...
	cd proto/gen  && go test -v ./...
	cd cmd/lint-architecture && go test -v ./...

test-fast:
	cd core       && go test -count=1 ./...
	cd server     && go test -count=1 ./...
	cd cli        && go test -count=1 ./...
	cd proto/gen  && go test -count=1 ./...
	cd cmd/lint-architecture && go test -count=1 ./...

# ── Quality ───────────────────────────────────────────────────────────────────

lint:
	@command -v staticcheck >/dev/null 2>&1 || (echo "staticcheck not installed: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	cd core       && staticcheck ./...
	cd server     && staticcheck ./...
	cd cli        && staticcheck ./...
	cd cmd/lint-architecture && staticcheck ./...

fmt:
	cd core       && gofmt -w -l .
	cd server     && gofmt -w -l .
	cd cli        && gofmt -w -l .
	cd cmd/lint-architecture && gofmt -w -l .
	@echo "Formatted all Go files"

vet:
	cd core       && go vet ./...
	cd server     && go vet ./...
	cd cli        && go vet ./...
	cd proto/gen  && go vet ./...
	cd cmd/lint-architecture && go vet ./...

# ── Buf / Protobuf ────────────────────────────────────────────────────────────

buf-build:
	@echo "Building buf toolchain image..."
	@docker compose -f $(BUF_COMPOSE_FILE) build

buf-up:
	@echo "Starting buf toolchain container..."
	@docker compose -f $(BUF_COMPOSE_FILE) up -d

buf-down:
	@echo "Stopping buf toolchain container..."
	@docker compose -f $(BUF_COMPOSE_FILE) down

buf-shell: buf-up
	@echo "Opening shell in buf toolchain container..."
	@docker exec -it $(BUF_CONTAINER) /bin/sh

buf-version: buf-up
	@docker exec $(BUF_CONTAINER) buf --version

buf-lint: buf-up
	@docker exec $(BUF_CONTAINER) buf lint

buf-format-check: buf-up
	@docker exec $(BUF_CONTAINER) buf format --diff

buf-format: buf-up
	@docker exec $(BUF_CONTAINER) buf format -w

buf-generate: buf-up
	@mkdir -p dashboard/src/gen
	@find dashboard/src/gen -mindepth 1 -delete
	@docker exec $(BUF_CONTAINER) buf generate
	@git checkout -- proto/gen/go.mod proto/gen/go.sum 2>/dev/null || true

buf-breaking: buf-up
	@docker exec $(BUF_CONTAINER) buf breaking --against '.git#branch=main'

buf-test: buf-up
	@$(MAKE) buf-lint
	@$(MAKE) buf-format-check
	@$(MAKE) buf-breaking

buf-clean: buf-down
	@docker compose -f $(BUF_COMPOSE_FILE) down -v

buf-quick-dev: buf-format buf-lint buf-generate

# ── Ops ───────────────────────────────────────────────────────────────────────

migrate:
	cd $(SERVER_DIR) && go run . 2>&1 | head -5 || true

clean:
	rm -f server/kave-server cli/kave-cli
	rm -rf server/ui/dist/*
	find . -name "*.test" -type f -delete
	go clean -testcache
	@echo "Cleaned"

cli-docs:
	@echo "Docs live in kave-io/kave-docs. Set KAVE_DOCS_DIR to its path, e.g.:"
	@echo "  KAVE_DOCS_DIR=../kave-docs make cli-docs"
	@test -n "$(KAVE_DOCS_DIR)" || (echo "error: KAVE_DOCS_DIR not set" && exit 1)
	cd cli && env GOCACHE=/tmp/go-build go run ./tools/gen-docs $(KAVE_DOCS_DIR)/src/content/docs/cli/reference

kave-local-install:
	@set -eu; \
	BIN_DIR="$(HOME)/.local/bin"; \
	CFG_DIR="$(HOME)/.kave"; \
	SVC_DIR="$(HOME)/.config/systemd/user"; \
	SVC_FILE="$$SVC_DIR/kave.service"; \
	mkdir -p "$$BIN_DIR" "$$CFG_DIR" "$$SVC_DIR"; \
	echo "Building CLI binary -> $$BIN_DIR/kave"; \
	cd cli && go build -o "$$BIN_DIR/kave" .; \
	cd - >/dev/null; \
	echo "Building daemon binary -> $$BIN_DIR/kave-server"; \
	cd server && go build -o "$$BIN_DIR/kave-server" .; \
	cd - >/dev/null; \
	KAVE_BIN_DIR="$$BIN_DIR" scripts/install-codex-shim; \
	echo "Writing local config -> $$CFG_DIR/kave.yaml"; \
	printf '%s\n' \
		'apiVersion: kave.io/v1' \
		'kind: KaveConfig' \
		'' \
		'daemon:' \
		'  address: 127.0.0.1:19090' \
		'  proxy_address: 127.0.0.1:18081' \
		'  data_dir: ~/.kave' \
		'  log_level: info' \
		'  log_format: text' \
		'' \
		'contexts:' \
		'  - name: default' \
		'    server: 127.0.0.1:19090' \
		'    user: local@kave.local' \
		'    project: kave-local' \
		'    env: dev' \
		'currentContext: default' \
		'' \
		'server:' \
		'  addr: 127.0.0.1:18080' \
		'' \
		'grpc:' \
		'  addr: 127.0.0.1:19090' \
		'' \
		'storage:' \
		'  defaults:' \
		'    app:' \
		'      kind: sqlite' \
		'      path: ${HOME}/.kave/app.db' \
		'    span:' \
		'      kind: duckdb' \
		'      path: ${HOME}/.kave/spans.duckdb' > "$$CFG_DIR/kave.yaml"; \
	echo "Writing user systemd unit -> $$SVC_FILE"; \
	printf '%s\n' \
		'[Unit]' \
		'Description=Kave Daemon' \
		'After=network-online.target' \
		'Wants=network-online.target' \
		'' \
		'[Service]' \
		'Type=simple' \
		'WorkingDirectory=%h/.kave' \
		'Environment=HOME=%h' \
		'ExecStart=%h/.local/bin/kave-server' \
		'Restart=always' \
		'RestartSec=2' \
		'' \
		'[Install]' \
		'WantedBy=default.target' > "$$SVC_FILE"; \
	systemctl --user daemon-reload; \
	systemctl --user enable --now kave.service; \
	echo "Installed and started kave.service (user unit)."

kave-local-uninstall:
	@set -eu; \
	SVC_FILE="$(HOME)/.config/systemd/user/kave.service"; \
	systemctl --user disable --now kave.service >/dev/null 2>&1 || true; \
	rm -f "$$SVC_FILE"; \
	systemctl --user daemon-reload >/dev/null 2>&1 || true; \
	systemctl --user reset-failed >/dev/null 2>&1 || true; \
	KAVE_BIN_DIR="$(HOME)/.local/bin" scripts/uninstall-codex-shim; \
	rm -f "$(HOME)/.local/bin/kave" "$(HOME)/.local/bin/kave-server"; \
	rm -rf "$(HOME)/.kave"; \
	echo "Removed user service, binaries, and ~/.kave."

kave-local-reinstall:
	@$(MAKE) kave-local-uninstall
	@$(MAKE) kave-local-install
