# Contributing to Fennec

## Prerequisites

- Go 1.25.5 or later
- No external dependencies — everything is pure stdlib

## Quick Start

```bash
# Clone and enter the project
cd fennec

# Run everything (format → vet → test → build)
make

# Or step by step:
make fixtures   # Generate test images (one-time)
make test       # Run all tests with race detector
make build      # Build CLI → bin/fennec
```

## Running Tests

### Generate Test Fixtures (required once)

Tests use real image files in `testdata/`. Generate them first:

```bash
make fixtures
# or manually:
go test -run TestGenerateTestData -v
```

These are synthetic images (gradients, circles, solid blocks) created by
`testdata_generate_test.go`. They're deterministic — re-running produces
identical files. You can delete `testdata/` and regenerate any time.

### Test Commands

| Command                 | What it does                            |
|-------------------------|-----------------------------------------|
| `make test`             | All tests + race detector (recommended) |
| `make test-unit`        | Unit tests only (fast, no file I/O)     |
| `make test-integration` | Integration tests only (uses fixtures)  |
| `make test-race`        | All tests with `-race` flag             |
| `make test-cover`       | Tests + HTML coverage report            |
| `make bench`            | Benchmarks with memory allocation stats |

### Running specific tests

```bash
# Single test
go test -run TestSSIMIdentical -v

# All SSIM tests
go test -run TestSSIM -v

# All integration tests
go test -run TestIntegration -v

# Benchmarks only (skip regular tests)
go test -bench=. -benchmem -run=^$ -v
```

### Test Organization

Tests are split into two categories:

**Unit tests** (`fennec_test.go`):

- Create images programmatically in memory (no disk I/O)
- Fast — run in a few seconds
- Cover: SSIM math, compression logic, resize, analysis, effects, edge cases
- Pattern: `TestSSIM*`, `TestCompress*`, `TestLanczos*`, `TestAnalyze*`, etc.

**Integration tests** (`integration_test.go`):

- Read/write real files from `testdata/`
- Test the full pipeline: `Open → Compress → Write → Verify`
- Cover: real JPEG/PNG round-trips, resize+compress, target size, analysis
- Pattern: `TestIntegration*`
- Auto-skip with a helpful message if `testdata/` is missing

## Debugging and Fixing Code

### Common issues

**"multiple packages in directory"**: The library is `package fennec` and
the CLI is `package main` in `cmd/fennec/`. This is standard Go layout.
Always use `go build ./...` (not `go build *.go`).

**Tests skip with "testdata missing"**: Run `make fixtures` first.

**SSIM returns unexpected values**: Check that images are the same dimensions.
SSIM requires identical sizes — resize first if comparing different-size images.

**Compression makes file larger**: Expected for small or already-optimized
images. The binary search finds the best quality but can't beat the original
if it's already well-compressed. Check `result.SavingsPercent`.

### Debugging workflow

```bash
# 1. Check if the code compiles
go build ./...

# 2. Run go vet for common mistakes
go vet ./...

# 3. Run the specific failing test with verbose output
go test -run TestNameHere -v -count=1

# 4. Run with race detector to catch concurrency bugs
go test -run TestNameHere -v -race

# 5. Add debug prints in the test or code, then re-run
#    (tests print with t.Logf which only shows on -v or failure)

# 6. Check coverage for the file you changed
go test -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out | grep yourfile.go
```

### Adding a new test

Unit tests go in `fennec_test.go`:

```go
func TestYourNewFeature(t *testing.T) {
// Create a test image programmatically
img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
// ... fill pixels ...

// Test your function
result := YourFunction(img)
if result != expected {
t.Fatalf("got %v, want %v", result, expected)
}
}
```

Integration tests go in `integration_test.go`:

```go
func TestIntegrationYourFeature(t *testing.T) {
ensureTestdata(t) // Skips if fixtures missing
tmpDir := t.TempDir() // Auto-cleaned temp directory

result, err := CompressFile("testdata/gradient.jpg",
filepath.Join(tmpDir, "output.jpg"), DefaultOptions())
if err != nil {
t.Fatalf("CompressFile: %v", err)
}
// ... assert on result ...
}
```

### Adding a new test fixture

Add generation logic in `testdata_generate_test.go` inside
`TestGenerateTestData`, following the `genIfMissing` pattern.
Then run `make fixtures` to create it.

## Code Quality

```bash
make fmt        # Format code (gofmt -s -w)
make vet        # Run go vet
make lint       # Run staticcheck (install separately)
```

Install staticcheck:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
```

## Architecture Notes

### How SSIM-guided compression works

1. User sets a quality preset (e.g., `Balanced` → target SSIM 0.94)
2. Binary search over JPEG quality Q1–Q100:
    - Encode at quality `mid`
    - Decode the compressed bytes back to an image
    - Compute SSIM between original and decoded
    - If SSIM ≥ target: try lower quality (smaller file)
    - If SSIM < target: try higher quality (better fidelity)
3. Converges in ~7 iterations to the minimum quality that meets the target

### How format auto-selection works

`analyze.go` examines the image and recommends a format:

- Has transparency → PNG (JPEG can't store alpha)
- ≤256 colors → PNG with palette (much smaller than JPEG for diagrams)
- High entropy + many colors → JPEG (photographs)
- Low edge density → JPEG (smooth gradients)
- High edge density + few colors → PNG (text, screenshots)

### How Lanczos resize handles alpha

Standard resize causes dark fringing at transparency edges because
interpolation mixes RGB values with zero-alpha (black transparent) pixels.
Fennec fixes this by:

1. Converting to pre-multiplied alpha (R×A, G×A, B×A)
2. Performing the Lanczos-3 interpolation
3. Converting back to straight alpha (R/A, G/A, B/A)

This ensures only visible colors participate in the interpolation.

## Performance Tips

- `SSIMFast()` downsamples large images before computing — use it for
  quick quality checks (1000×1000 in ~30ms vs ~90ms for full `SSIM()`)
- The parallel resize and SSIM use `runtime.NumCPU()` goroutines
- PNG compression is single-pass lossless, no binary search needed
- For batch processing, reuse decoded images rather than re-opening files