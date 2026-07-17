.PHONY: help dev dev-server dev-dashboard serve build build-server dashboard-build \
        test test-fast test-unit test-integration test-e2e test-contracts \
        bench fuzz loadgen lint lint-arch fmt vet migrate clean cli-docs \
        kave-local-install kave-local-uninstall kave-local-reinstall \
        buf-build buf-up buf-down buf-shell buf-version buf-lint buf-format-check \
        buf-format buf-generate buf-breaking buf-test buf-clean buf-quick-dev \
        sdk-generate sdk-test \
        docker-build docker-push docker-save-server docker-build-arch docker-push-arch \
        release-snapshot release-snapshot-core \
        release-snapshot-sdk-go release-snapshot-sdk-py release-snapshot-sdk-ts \
        docs-publish release
.NOTPARALLEL: kave-local-install kave-local-uninstall kave-local-reinstall

DASHBOARD_DIR := dashboard
SERVER_DIR    := server
BUF_COMPOSE_FILE := compose.buf.yaml
BUF_CONTAINER := buf
SDK_ROOT ?= ../sdk
DIST_DIR ?= dist

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
	@echo "Release:"
	@echo "  make release-snapshot  Local dry-run release plus SDK package artifacts"
	@echo "  make docs-publish      Rebuild external docs for KAVE_VERSION=<tag>"
	@echo "  make docker-build      Build multi-arch Docker images locally (server + cli)"
	@echo "  make docker-push       Push Docker images to Docker Hub (requires docker login)"
	@echo "  make docker-save-server Build server image and save a local Docker tar archive"
	@echo ""
	@echo "Quality:"
	@echo "  make test              Run unit + integration + contracts"
	@echo "  make test-unit         Unit tests only (no external deps)"
	@echo "  make test-integration  Tests tagged 'integration' (local services)"
	@echo "  make test-e2e          Tests tagged 'e2e' (boots full stack)"
	@echo "  make test-contracts    Golden-file contract tests"
	@echo "  make test-fast         Cached run across all modules"
	@echo "  make bench             Run all Go benchmarks"
	@echo "  make fuzz              Run every Fuzz* corpus (FUZZTIME=10s)"
	@echo "  make loadgen           Run loadgen scenarios"
	@echo "  make lint              staticcheck on all Go modules"
	@echo "  make lint-arch         Run architecture linter (B*-* rules)"
	@echo "  make fmt               gofmt all Go files"
	@echo "  make vet               go vet all modules"
	@echo "  make cli-docs          Regenerate Cobra CLI docs"
	@echo ""
	@echo "SDK:"
	@echo "  make sdk-generate      buf generate → Go/Python/TS stubs in sdk/*/gen"
	@echo "  make sdk-test          Start fixture server + run Go contract tests"
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
# Build-tag convention:
#   default       — unit tests (fast, no external deps)
#   //go:build integration — tests that hit local services (duckdb, sqlite, postgres-in-docker)
#   //go:build e2e         — full-stack tests booting the server binary
#   //go:build contracts   — golden-file contract tests for HTTP/gRPC surfaces
# Run `make test` for the umbrella; CI runs the splits separately.

GO_MODULES := core server cli proto/gen cmd/lint-architecture
TEST_FLAGS ?=

test: test-unit test-integration test-contracts

test-unit:
	@for m in $(GO_MODULES); do \
	  echo ">> unit: $$m"; \
	  (cd $$m && go test -count=1 $(TEST_FLAGS) ./...) || exit 1; \
	done

test-integration:
	@for m in $(GO_MODULES); do \
	  echo ">> integration: $$m"; \
	  (cd $$m && go test -count=1 -tags=integration $(TEST_FLAGS) ./...) || exit 1; \
	done

test-e2e:
	@for m in $(GO_MODULES); do \
	  echo ">> e2e: $$m"; \
	  (cd $$m && go test -count=1 -tags=e2e -timeout=10m $(TEST_FLAGS) ./...) || exit 1; \
	done

test-contracts:
	@for m in $(GO_MODULES); do \
	  echo ">> contracts: $$m"; \
	  (cd $$m && go test -count=1 -tags=contracts $(TEST_FLAGS) ./...) || exit 1; \
	done

test-fast:
	@for m in $(GO_MODULES); do \
	  (cd $$m && go test ./...) || exit 1; \
	done

bench:
	@for m in $(GO_MODULES); do \
	  echo ">> bench: $$m"; \
	  (cd $$m && go test -run=^$$ -bench=. -benchmem -benchtime=1s ./...) || exit 1; \
	done

# Run every Fuzz* corpus briefly. Use FUZZTIME=30s make fuzz for longer runs.
FUZZTIME ?= 10s
fuzz:
	@for m in $(GO_MODULES); do \
	  echo ">> fuzz: $$m (FUZZTIME=$(FUZZTIME))"; \
	  for pkg in $$(cd $$m && go list ./...); do \
	    for fn in $$(cd $$m && go test -list '^Fuzz.*' $$pkg 2>/dev/null | grep '^Fuzz' || true); do \
	      echo "   - $$pkg :: $$fn"; \
	      (cd $$m && go test -run=^$$ -fuzz=^$$fn$$ -fuzztime=$(FUZZTIME) $$pkg) || exit 1; \
	    done; \
	  done; \
	done

# Loadgen: long-running perf scenarios under ./benchmarks/loadgen (placeholder until scenarios land).
loadgen:
	@if [ -d benchmarks/loadgen ]; then \
	  go run ./benchmarks/loadgen/... ; \
	else \
	  echo "no benchmarks/loadgen scenarios yet — see benchmarks/BASELINE.md"; \
	fi

lint-arch:
	go run ./cmd/lint-architecture

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
	@mkdir -p sdk/ts/src/gen sdk/py/kave/gen
	@find sdk/ts/src/gen -mindepth 1 -delete 2>/dev/null || true
	@find sdk/py -name '*_pb2*.py' -o -name '*_pb2*.pyi' | xargs rm -f 2>/dev/null || true
	@docker exec $(BUF_CONTAINER) buf generate
	@git checkout -- proto/gen/go.mod proto/gen/go.sum 2>/dev/null || true

sdk-generate: buf-generate

sdk-test: buf-up
	@echo "Starting fixture server..."
	@docker compose -f ../sdk/contract-tests/server/docker-compose.yml up -d
	@echo "Running Go contract tests..."
	@cd ../sdk/go && go test -tags contracts ./... -v -count=1 -timeout 120s \
		-run TestContract || (docker compose -f ../sdk/contract-tests/server/docker-compose.yml down; exit 1)
	@docker compose -f ../sdk/contract-tests/server/docker-compose.yml down

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

docs-publish: cli-docs
	@test -n "$(KAVE_VERSION)" || (echo "error: KAVE_VERSION not set, e.g. KAVE_VERSION=v0.2.0 make docs-publish" && exit 1)
	@mkdir -p "$(KAVE_DOCS_DIR)/src/content/docs/sdk/go" "$(KAVE_DOCS_DIR)/src/content/docs/sdk/python" "$(KAVE_DOCS_DIR)/src/content/docs/sdk/typescript"
	cp "$(SDK_ROOT)/go/README.md" "$(KAVE_DOCS_DIR)/src/content/docs/sdk/go/quickstart.md"
	cp "$(SDK_ROOT)/py/README.md" "$(KAVE_DOCS_DIR)/src/content/docs/sdk/python/quickstart.md"
	cp "$(SDK_ROOT)/ts/README.md" "$(KAVE_DOCS_DIR)/src/content/docs/sdk/typescript/quickstart.md"
	cd "$(KAVE_DOCS_DIR)" && KAVE_VERSION="$(KAVE_VERSION)" bun install --frozen-lockfile
	cd "$(KAVE_DOCS_DIR)" && KAVE_VERSION="$(KAVE_VERSION)" bun run build
	@if command -v gh >/dev/null 2>&1; then \
	  gh api repos/kave-io/kave-docs/dispatches \
	    -f event_type=kave-release \
	    -f client_payload[version]="$(KAVE_VERSION)"; \
	else \
	  echo "gh not installed; docs built locally but dispatch was skipped"; \
	fi

# ── Release ───────────────────────────────────────────────────────────────────

GORELEASER ?= goreleaser
DOCKER_REGISTRY ?= alijulaeerad
VERSION ?= 1.4.0
LOCAL_PLATFORM ?= linux/amd64
SERVER_IMAGE ?= $(DOCKER_REGISTRY)/kave-server:$(VERSION)
CLI_IMAGE ?= $(DOCKER_REGISTRY)/kave-cli:$(VERSION)
SERVER_ARCH_IMAGE ?= $(DOCKER_REGISTRY)/kave-server:$(VERSION)-arch
SERVER_IMAGE_TAR ?= $(DIST_DIR)/images/kave-server-$(VERSION)-$(subst /,-,$(LOCAL_PLATFORM)).tar

release-snapshot: release-snapshot-core release-snapshot-sdk-go release-snapshot-sdk-py release-snapshot-sdk-ts

release-snapshot-core:
	@command -v $(GORELEASER) >/dev/null 2>&1 || (echo "goreleaser not installed: https://goreleaser.com/install/" && exit 1)
	$(GORELEASER) release --snapshot --clean

release-snapshot-sdk-go:
	@mkdir -p "$(DIST_DIR)/sdk/go"
	cd "$(SDK_ROOT)/go" && go build -o "../../core/$(DIST_DIR)/sdk/go/bootstrap" ./examples/bootstrap
	cd "$(SDK_ROOT)/go" && go build -o "../../core/$(DIST_DIR)/sdk/go/runtime" ./examples/runtime
	cd "$(SDK_ROOT)/go" && go build -o "../../core/$(DIST_DIR)/sdk/go/streaming" ./examples/streaming
	cd "$(DIST_DIR)" && tar -czf "kave-sdk-go_snapshot.tar.gz" sdk/go

release-snapshot-sdk-py:
	@command -v uv >/dev/null 2>&1 || (echo "uv not installed: https://docs.astral.sh/uv/" && exit 1)
	@mkdir -p "$(DIST_DIR)/sdk/py"
	cd "$(SDK_ROOT)/py" && uv build --out-dir "../../core/$(DIST_DIR)/sdk/py"

release-snapshot-sdk-ts:
	@command -v bun >/dev/null 2>&1 || (echo "bun not installed: https://bun.sh" && exit 1)
	@mkdir -p "$(DIST_DIR)/sdk/ts"
	cd "$(SDK_ROOT)/ts" && bun run build
	cd "$(SDK_ROOT)/ts" && npm pack --pack-destination "../../core/$(DIST_DIR)/sdk/ts"

release:
	@command -v $(GORELEASER) >/dev/null 2>&1 || (echo "goreleaser not installed: https://goreleaser.com/install/" && exit 1)
	$(GORELEASER) release --clean

docker-build: dashboard-build
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target server \
	  --build-arg VERSION=$(VERSION) \
	  -t $(SERVER_IMAGE) \
	  --load=false \
	  .
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target cli \
	  --build-arg VERSION=$(VERSION) \
	  -t $(CLI_IMAGE) \
	  --load=false \
	  .

docker-push: dashboard-build
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target server \
	  --build-arg VERSION=$(VERSION) \
	  -t $(SERVER_IMAGE) \
	  --push \
	  .
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target cli \
	  --build-arg VERSION=$(VERSION) \
	  -t $(CLI_IMAGE) \
	  --push \
	  .

docker-save-server: dashboard-build
	@mkdir -p "$(dir $(SERVER_IMAGE_TAR))"
	docker buildx build \
	  --platform $(LOCAL_PLATFORM) \
	  --target server \
	  --build-arg VERSION=$(VERSION) \
	  -t $(SERVER_IMAGE) \
	  --load \
	  .
	docker save $(SERVER_IMAGE) -o "$(SERVER_IMAGE_TAR)"
	@echo "Saved server image: $(SERVER_IMAGE_TAR)"

docker-build-arch: dashboard-build
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target server-arch \
	  --build-arg VERSION=$(VERSION) \
	  -t $(SERVER_ARCH_IMAGE) \
	  --load=false \
	  .

docker-push-arch: dashboard-build
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --target server-arch \
	  --build-arg VERSION=$(VERSION) \
	  -t $(SERVER_ARCH_IMAGE) \
	  --push \
	  .

# ── Local Install ─────────────────────────────────────────────────────────────

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
