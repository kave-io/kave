.PHONY: help dev build test test-fast lint fmt vet migrate clean

help:
	@echo "Kave — Control Plane for AI Agents"
	@echo ""
	@echo "Commands:"
	@echo "  make dev          Hot reload Go server with Air"
	@echo "  make build        Build kave-server binary"
	@echo "  make test         Run all tests"
	@echo "  make test-fast    Run tests (cached, no DB rebuild)"
	@echo "  make lint         Run staticcheck"
	@echo "  make fmt          Format code with gofmt"
	@echo "  make vet          Run go vet"
	@echo "  make migrate      Run database migrations (requires postgres)"
	@echo "  make clean        Remove binaries and cache"

dev:
	cd server && air || (echo "air not installed; run: go install github.com/cosmtrek/air@latest" && exit 1)

build:
	cd server && go build -o kave-server .

test:
	cd core && go test -v ./... && \
	cd ../server && go test -v ./... && \
	cd ../connectors && go test -v ./...

test-fast:
	cd core && go test -count=1 ./... && \
	cd ../server && go test -count=1 ./... && \
	cd ../connectors && go test -count=1 ./...

lint:
	@command -v staticcheck >/dev/null 2>&1 || (echo "staticcheck not installed; run: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	cd core && staticcheck ./... && \
	cd ../server && staticcheck ./... && \
	cd ../connectors && staticcheck ./... && \
	cd ../cli && staticcheck ./...

fmt:
	cd core && gofmt -w -l . && \
	cd ../server && gofmt -w -l . && \
	cd ../connectors && gofmt -w -l . && \
	cd ../cli && gofmt -w -l .
	@echo "Formatted all Go files"

vet:
	cd core && go vet ./... && \
	cd ../server && go vet ./... && \
	cd ../connectors && go vet ./... && \
	cd ../cli && go vet ./...

migrate:
	@echo "Running migrations (requires postgres)..."
	cd server && go run . migrate || echo "Could not run migrations; ensure postgres is running"

clean:
	rm -f server/kave-server cli/kave-cli
	go clean -cache -testcache
	find . -name "*.test" -type f -delete
	@echo "Cleaned build artifacts"
