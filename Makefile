# ──────────────────────────────────────────────────────────────────────────────
# Fennec — Intelligent Image Compression Library  v2.0
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: all build test test-unit test-integration test-race test-cover \
        bench lint vet fmt fixtures clean help install

# Default target
all: fmt vet test build

# ─── Build ────────────────────────────────────────────────────────────────────

build:
	go build -o bin/fennec ./cmd/fennec

install:
	go install ./cmd/fennec

# ─── Testing ──────────────────────────────────────────────────────────────────

fixtures:
	go test -run TestGenerateTestData -v

test: fixtures
	go test -count=1 -race -v ./...

test-unit:
	go test -count=1 -race -v -run "^Test[^I]" ./...

test-integration: fixtures
	go test -count=1 -race -v -run "^TestIntegration" ./...

test-race: fixtures
	go test -count=1 -race -v ./...

test-cover: fixtures
	go test -count=1 -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: fixtures
	go test -bench=. -benchmem -run=^$$ ./...

# ─── Code Quality ─────────────────────────────────────────────────────────────

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	@command -v staticcheck >/dev/null 2>&1 || { echo "Install: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

# ─── Cleanup ──────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ coverage.out coverage.html
	rm -rf testdata/

# ─── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo "Fennec v2.0 — Development Commands"
	@echo ""
	@echo "  make              Run fmt + vet + test + build (default)"
	@echo "  make build        Build the CLI → bin/fennec"
	@echo "  make install      Install CLI to \$$GOPATH/bin"
	@echo ""
	@echo "  make test         Run ALL tests (generates fixtures first)"
	@echo "  make test-unit    Run unit tests only (fast, no I/O)"
	@echo "  make test-integration  Run integration tests (with fixtures)"
	@echo "  make test-race    Run all tests with race detector"
	@echo "  make test-cover   Run tests + generate HTML coverage report"
	@echo "  make bench        Run benchmarks"
	@echo ""
	@echo "  make fmt          Format code with gofmt"
	@echo "  make vet          Run go vet"
	@echo "  make lint         Run staticcheck (install separately)"
	@echo "  make fixtures     Generate test images in testdata/"
	@echo "  make clean        Remove build artifacts + testdata"
	@echo "  make help         Show this help"