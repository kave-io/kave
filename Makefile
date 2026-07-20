SHELL := /bin/sh

.DEFAULT_GOAL := help

GO_MODULES := core proto/gen server
DIST_DIR ?= dist
SERVER_BINARY := $(DIST_DIR)/kave-server
PROTO_PATH := proto/kave/kernel/v2/kernel.proto

.PHONY: help tools build build-server dashboard-build dashboard-build-check clean \
	test test-unit test-race test-e2e test-postgres fmt fmt-check vet lint check \
	generate generate-check surface-check security-check docker-build dev-up dev-down migrate bootstrap

help:
	@echo "Kave"
	@echo ""
	@echo "  make tools            install the pinned Go build and security helpers"
	@echo "  make build            build the embedded console and kave-server"
	@echo "  make test             run unit tests and race tests"
	@echo "  make test-e2e         run the focused Chromium console acceptance test"
	@echo "  make check            run formatting, vet, lint, and tests"
	@echo "  make generate         regenerate Go and dashboard API contracts"
	@echo "  make generate-check   regenerate contracts and reject drift"
	@echo "  make dashboard-build-check  rebuild the embedded console and reject drift"
	@echo "  make surface-check    reject retired product/API surfaces"
	@echo "  make security-check   scan the current commit and dependencies"
	@echo "  make docker-build     build the production image"
	@echo "  make dev-up           start the loopback-only Postgres/server stack"
	@echo "  make dev-down         stop the developer stack"

build: dashboard-build build-server

dashboard-build:
	cd dashboard && bun run build

dashboard-build-check: dashboard-build
	@status="$$(git status --porcelain --untracked-files=all -- server/ui/dist)"; \
		test -z "$$status" || (echo "embedded console is stale:" >&2; echo "$$status" >&2; exit 1)

build-server:
	mkdir -p $(DIST_DIR)
	cd server && CGO_ENABLED=0 go build -trimpath \
		-ldflags='-s -w -X main.buildVersion=dev' \
		-o ../$(SERVER_BINARY) .

test: test-unit test-race

test-e2e:
	@if [ "$${KAVE_PLAYWRIGHT_USE_BUNDLED:-}" = "1" ]; then \
		cd dashboard && bun run test:e2e -- --project=chromium; \
	else \
		browser="$${PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH:-}"; \
		if [ -z "$$browser" ]; then \
			browser="$$(command -v google-chrome-stable 2>/dev/null || command -v google-chrome 2>/dev/null || command -v chromium 2>/dev/null || true)"; \
		fi; \
		if [ -n "$$browser" ]; then export PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="$$browser"; fi; \
		cd dashboard && bun run test:e2e -- --project=chromium; \
	fi

test-unit:
	@set -eu; for module in $(GO_MODULES); do \
		echo ">> test $$module"; \
		(cd "$$module" && go test -count=1 ./...); \
	done
	cd dashboard && bun run test

test-race:
	cd core && go test -race -count=1 ./v2/...
	cd server && go test -race -count=1 . ./internal/v2/...

# The database tests skip unless their explicit DSNs are present. CI supplies
# isolated databases and both variables; local callers must do the same.
test-postgres:
	@test -n "$(KAVE_TEST_V2_POSTGRES_DSN)" || (echo "KAVE_TEST_V2_POSTGRES_DSN is required" >&2; exit 1)
	@test -n "$(KAVE_TEST_V2_POSTGRES_ADMIN_DSN)" || (echo "KAVE_TEST_V2_POSTGRES_ADMIN_DSN is required" >&2; exit 1)
	cd server && go test -race -count=1 -timeout=15m ./internal/v2/postgres ./internal/v2

fmt:
	@set -eu; for module in $(GO_MODULES); do \
		find "$$module" -name '*.go' -type f -print0 | xargs -0 gofmt -w; \
	done
	buf format -w
	cd dashboard && bun run format

fmt-check:
	@files="$$(find $(GO_MODULES) -name '*.go' -type f -print0 | xargs -0 gofmt -l)"; \
	test -z "$$files" || (echo "gofmt required:" >&2; echo "$$files" >&2; exit 1)
	buf format --diff --exit-code
	cd dashboard && bun run format:check

vet:
	@set -eu; for module in $(GO_MODULES); do \
		echo ">> vet $$module"; \
		(cd "$$module" && go vet ./...); \
	done

lint:
	@command -v staticcheck >/dev/null 2>&1 || (echo "staticcheck is required" >&2; exit 1)
	@set -eu; for module in $(GO_MODULES); do \
		echo ">> staticcheck $$module"; \
		(cd "$$module" && staticcheck ./...); \
	done
	cd dashboard && bun run lint

check: surface-check fmt-check generate-check vet lint test test-e2e dashboard-build-check

tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.69.0
	go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1
	go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
	go install github.com/zricethezav/gitleaks/v8@v8.30.1

security-check:
	@command -v gitleaks >/dev/null 2>&1 || (echo "gitleaks is required; run make tools" >&2; exit 1)
	@command -v govulncheck >/dev/null 2>&1 || (echo "govulncheck is required; run make tools" >&2; exit 1)
	gitleaks git --redact --no-banner --log-opts=-1 .
	@set -eu; for module in $(GO_MODULES); do \
		echo ">> govulncheck $$module"; \
		(cd "$$module" && govulncheck ./...); \
	done
	cd dashboard && bun audit

generate:
	rm -rf proto/gen/kave dashboard/src/gen/kave
	buf generate --template buf.gen.yaml --path $(PROTO_PATH)
	buf generate --template buf.gen.dashboard.yaml --path $(PROTO_PATH)
	@generated=dashboard/src/gen/kave/kernel/v2/kernel_pb.ts; \
		tmp="$${generated}.tmp"; \
		awk '{ lines[NR] = $$0; if ($$0 !~ /^[[:space:]]*$$/) last = NR } \
			END { for (i = 1; i <= last; i++) print lines[i] }' \
			"$$generated" > "$$tmp" && mv "$$tmp" "$$generated"

generate-check: generate
	@status="$$(git status --porcelain --untracked-files=all -- proto/gen dashboard/src/gen)"; \
		test -z "$$status" || (echo "generated contracts are stale:" >&2; echo "$$status" >&2; exit 1)

surface-check:
	@command -v rg >/dev/null 2>&1 || (echo "ripgrep is required" >&2; exit 1)
	@unexpected="$$(find core -mindepth 2 -type f \
		! -path 'core/.*/*' ! -path 'core/v2/*' ! -path 'core/pkg/ids/*' -print)"; \
	test -z "$$unexpected" || (echo "unexpected core surface:" >&2; echo "$$unexpected" >&2; exit 1)
	@unexpected="$$(find server/internal -type f ! -path 'server/internal/v2/*' -print)"; \
	test -z "$$unexpected" || (echo "unexpected server surface:" >&2; echo "$$unexpected" >&2; exit 1)
	@unexpected="$$(find proto/kave -type f ! -path 'proto/kave/kernel/v2/*' -print)"; \
	test -z "$$unexpected" || (echo "unexpected source API surface:" >&2; echo "$$unexpected" >&2; exit 1)
	@unexpected="$$(find proto/gen -type f \
		! -path 'proto/gen/kave/kernel/v2/*' \
		! -name go.mod ! -name go.sum -print)"; \
	test -z "$$unexpected" || (echo "unexpected generated API surface:" >&2; echo "$$unexpected" >&2; exit 1)
	@test ! -e proto/gen/kave/kernel/v2/kernel_grpc.pb.go || \
		(echo "unexpected gRPC transport output" >&2; exit 1)
	@if rg -n 'kave\.(audit|common|control|runtime)\.v1|/api/v1/' core server proto dashboard; then \
		echo "retired API reference found" >&2; exit 1; \
	fi

docker-build:
	docker build --build-arg VERSION=dev -t kave:dev .

# The compose stack requires fresh keys in the caller's environment. See the
# operator guide for the two openssl commands.
dev-up:
	docker compose up --build --wait

dev-down:
	docker compose down

migrate: build-server
	$(SERVER_BINARY) migrate

bootstrap: build-server
	$(SERVER_BINARY) bootstrap

clean:
	rm -rf "$(DIST_DIR)"
