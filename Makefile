# ──────────────────────────────────────────────────────────────────────────────
# Fennec — Intelligent Image Compression Library
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: all build test test-unit test-integration test-race test-cover \
        bench lint vet fmt fixtures clean help install

# Default target
all: fmt vet test build

# ─── Build ────────────────────────────────────────────────────────────────────

## Build the CLI binary
build:
	go build -o bin/fennec ./cmd/fennec

## Install the CLI to $GOPATH/bin
install:
	go install ./cmd/fennec

# ─── Testing ──────────────────────────────────────────────────────────────────

## Generate test fixture images (run once, or after cleaning testdata/)
fixtures:
	go test -run TestGenerateTestData -v

## Run ALL tests (unit + integration) with race detector
test: fixtures
	go test -count=1 -race -v ./...

## Run only unit tests (no file I/O, fast)
test-unit:
	go test -count=1 -race -v -run "^Test[^I]" ./...

## Run only integration tests (requires fixtures)
test-integration: fixtures
	go test -count=1 -race -v -run "^TestIntegration" ./...

## Run tests with race detector and verbose output
test-race: fixtures
	go test -count=1 -race -v ./...

## Run tests with coverage report
test-cover: fixtures
	go test -count=1 -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Run benchmarks
bench: fixtures
	go test -bench=. -benchmem -run=^$$ ./...

# ─── Code Quality ─────────────────────────────────────────────────────────────

## Format all Go source files
fmt:
	gofmt -s -w .

## Run go vet on all packages
vet:
	go vet ./...

## Run staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint:
	@command -v staticcheck >/dev/null 2>&1 || { echo "Install: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

# ─── Cleanup ──────────────────────────────────────────────────────────────────

## Remove build artifacts and generated test data
clean:
	rm -rf bin/ coverage.out coverage.html
	rm -rf testdata/

# ─── Help ─────────────────────────────────────────────────────────────────────

## Show this help
help:
	@echo "Fennec — Development Commands"
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