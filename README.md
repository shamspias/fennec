# 🦊 Fennec

**Tiny Fox. Giant Ears. Hears what matters, drops what doesn't.**

Fennec is a pure-Go image compression library that uses **SSIM-guided optimization** to find the smallest file size that
preserves visual quality. Instead of guessing a JPEG quality number, you tell Fennec how good the output should *look*,
and it figures out the rest.

## Why Fennec?

Standard image libraries make you pick a JPEG quality (0–100). But Q=75 is overkill for a smooth gradient and terrible
for a detailed photograph. You're either wasting bytes or destroying detail.

Fennec solves this by **binary-searching for the minimum JPEG quality that meets a perceptual quality target** (measured
by SSIM). It also handles format selection, resizing, color quantization, and target file size — all automatically.

**6.8 MB PNG → 100 KB JPEG at Q=37 with SSIM 0.98.** One line of code.

## Install

```bash
go get github.com/shamspias/fennec@latest
```

## Quick Start

```go
import "github.com/shamspias/fennec"

// Compress with sensible defaults (Balanced preset, SSIM ≥ 0.94)
result, _ := fennec.CompressFile("photo.jpg", "optimized.jpg", fennec.DefaultOptions())
fmt.Println(result)
// → JPEG | Q=42 | 4000×3000 → 4000×3000 | 3.2 MB → 320 KB | SSIM: 0.9512 | Saved: 90%
```

```go
// Hit a specific file size target
opts := fennec.DefaultOptions()
opts.TargetSize = 100 * 1024 // 100 KB
result, _ := fennec.CompressFile("photo.png", "small.jpg", opts)
// Fennec auto-selects format, dimensions, and quality to hit the target
```

```go
// Resize + compress in one call
opts := fennec.DefaultOptions()
opts.MaxWidth = 1920
opts.Quality = fennec.High  // SSIM ≥ 0.97
result, _ := fennec.CompressFile("huge.jpg", "web.jpg", opts)
```

## Features

**SSIM-Guided Compression** — Binary search over JPEG Q=1–100, comparing each output against the original using
Structural Similarity Index. Converges in ~7 iterations to the minimum quality that meets your target.

**Multi-Strategy Target Size** — When you specify a target file size, Fennec tries four strategies and picks the best
result: JPEG quality search with a Q≥20 floor, median-cut color quantization to indexed PNG, combined quality + scale
binary search, and progressive scale search. It never produces blocking artifacts by capping JPEG quality at full
resolution.

**Content-Aware Format Selection** — Analyzes entropy, edge density, unique color count, and transparency to pick JPEG
or PNG automatically. Screenshots and diagrams get PNG; photographs get JPEG.

**Lanczos-3 Resize** — High-quality downscaling with pre-multiplied alpha. Standard resize algorithms cause dark
fringing at transparency edges because they mix visible colors with zero-alpha black pixels. Fennec converts to
pre-multiplied alpha before interpolation, then converts back.

**Smart PNG Optimization** — Auto-palettizes images with ≤256 colors (using a median-cut quantizer), and detects
grayscale images for smaller output.

**Zero Dependencies** — Pure Go standard library. No cgo, no external packages, builds anywhere Go builds.

## Quality Presets

| Preset       | SSIM Target | Best For                        |
|--------------|-------------|---------------------------------|
| `Lossless`   | 1.00        | Archival, medical imaging       |
| `Ultra`      | ≥ 0.99      | Print, professional photography |
| `High`       | ≥ 0.97      | Web galleries, portfolios       |
| `Balanced`   | ≥ 0.94      | General web use **(default)**   |
| `Aggressive` | ≥ 0.90      | Thumbnails, social media        |
| `Maximum`    | ≥ 0.85      | Extreme compression, previews   |

## API Reference

### Compression

```go
// File-to-file compression (reads, compresses, writes, returns stats)
result, err := fennec.CompressFile(src, dst string, opts Options) (*Result, error)

// Compress an already-decoded image
result, err := fennec.CompressImage(img image.Image, opts Options) (*Result, error)

// Stream-based compression
result, err := fennec.Compress(r io.Reader, opts Options) (*Result, error)
```

### Quality Measurement

```go
// Structural Similarity Index (1.0 = identical, >0.95 = excellent)
ssim := fennec.SSIM(original, compressed image.Image) float64

// Multi-scale SSIM (better correlation with human perception)
msssim := fennec.MSSSIM(original, compressed image.Image) float64

// Fast SSIM using downsampling (good for quick checks)
ssim := fennec.SSIMFast(a, b *image.NRGBA) float64
```

### Image Analysis

```go
stats := fennec.Analyze(img image.Image) ImageStats

// Returns: Width, Height, HasAlpha, IsGrayscale, UniqueColors,
//          Entropy, EdgeDensity, MeanBrightness, Contrast,
//          RecommendedFormat, RecommendedQuality, EstimatedCompression
```

### Effects

```go
sharpened := fennec.Sharpen(img *image.NRGBA, strength float64) *image.NRGBA
sharpened := fennec.AdaptiveSharpen(img *image.NRGBA, strength float64) *image.NRGBA
blurred   := fennec.GaussianBlur(img *image.NRGBA, sigma float64) *image.NRGBA
```

### Options

```go
type Options struct {
Quality       Quality // Preset: Lossless, Ultra, High, Balanced, Aggressive, Maximum
Format        Format  // Auto, JPEG, or PNG
MaxWidth      int     // Maximum output width (0 = no limit)
MaxHeight     int     // Maximum output height (0 = no limit)
TargetSSIM    float64 // Custom SSIM target (overrides Quality preset)
TargetSize    int     // Target file size in bytes (enables multi-strategy engine)
StripMetadata bool     // Remove EXIF data
Subsample     bool     // Chroma subsampling for JPEG
}
```

### Result

```go
type Result struct {
Image              *image.NRGBA
Format             Format
OriginalSize       int64
CompressedSize     int64
SSIM               float64
JPEGQuality        int
Ratio              float64
SavingsPercent     float64
OriginalDimensions image.Point
FinalDimensions    image.Point
}
```

## CLI

```bash
# Build
make build

# Compress with defaults (Balanced, auto format)
bin/fennec photo.jpg compressed.jpg

# High quality
bin/fennec -quality ultra photo.png out.png

# Resize and compress
bin/fennec -quality aggressive -max-width 1920 photo.jpg out.jpg

# Hit a target file size
bin/fennec -target-size 100KB photo.png output.jpg

# Analyze without compressing
bin/fennec -analyze photo.png
```

### CLI Flags

| Flag           | Default    | Description                                                  |
|----------------|------------|--------------------------------------------------------------|
| `-quality`     | `balanced` | Preset: lossless, ultra, high, balanced, aggressive, maximum |
| `-format`      | `auto`     | Output format: auto, jpeg, png                               |
| `-max-width`   | `0`        | Maximum width (0 = no limit)                                 |
| `-max-height`  | `0`        | Maximum height (0 = no limit)                                |
| `-target-size` | —          | Target file size (e.g., `100KB`, `2MB`)                      |
| `-ssim`        | `0`        | Custom SSIM target (0.0–1.0, overrides quality)              |
| `-analyze`     | `false`    | Analyze image without compressing                            |

## How It Works

### SSIM-Guided Binary Search

```
Input: image, target SSIM = 0.94

  Q=50 → encode → decode → SSIM=0.98 → too high, try lower
  Q=25 → encode → decode → SSIM=0.91 → too low, try higher
  Q=37 → encode → decode → SSIM=0.95 → above target, try lower
  Q=31 → encode → decode → SSIM=0.93 → below target, try higher
  Q=34 → encode → decode → SSIM=0.94 → ✓ done

Output: JPEG at Q=34, smallest file that meets SSIM ≥ 0.94
```

### Target Size Engine

When you specify `-target-size`, Fennec runs four strategies and picks the best:

**Strategy 1: JPEG Quality Search** — Binary search Q=1–100 at full resolution. Rejected if the optimal quality falls
below Q=20 (would cause blocking artifacts).

**Strategy 2: Color Quantization** — Median-cut algorithm reduces the image to ≤256 colors, then encodes as indexed PNG.
Produces perfect quality for screenshots and diagrams.

**Strategy 3: Quality + Scale Search** — Binary search on scale factor (using fast box downsampling during exploration,
Lanczos-3 for the final output) to find the largest dimensions where Q≥20 JPEG fits under the target. This is the
workhorse for aggressive targets on photographs.

**Strategy 4: Scale Search** — Fallback binary search on scale factor alone.

The engine picks whichever strategy produces the result that fits under the target with the highest SSIM.

### Format Auto-Selection

| Image characteristic      | Chosen format | Why                                   |
|---------------------------|---------------|---------------------------------------|
| Has transparency (alpha)  | PNG           | JPEG can't store alpha                |
| ≤ 256 unique colors       | PNG (indexed) | Palette PNG is dramatically smaller   |
| High entropy, many colors | JPEG          | Photographs compress well with DCT    |
| High edge density         | PNG           | Text and screenshots need sharp edges |
| Smooth gradients          | JPEG          | Low edge density, DCT excels here     |

## Benchmarks

Measured on Apple M2 Pro (12 cores):

| Operation                | Input     | Time    | Memory  | Allocs    |
|--------------------------|-----------|---------|---------|-----------|
| SSIM                     | 500×500   | 6.1 ms  | 6.0 MB  | 34        |
| SSIMFast                 | 500×500   | 5.2 ms  | 1.6 MB  | 33        |
| Lanczos Resize           | 1000→500  | 3.3 ms  | 3.2 MB  | 1,058     |
| Compress JPEG (Balanced) | 500×500   | 70.6 ms | 23.9 MB | 1,500,338 |
| Analyze                  | 1000×1000 | 3.9 ms  | 4.0 MB  | 22        |
| Gaussian Blur σ=2        | 500×500   | 2.2 ms  | 2.0 MB  | 57        |
| Full Pipeline (JPEG)     | 1920×1080 | 752 ms  | 141 MB  | 14.5M     |
| SSIM (real images)       | 400×300   | 2.2 ms  | 1.2 MB  | 33        |

## Development

```bash
make              # fmt + vet + test + build
make test         # All tests with race detector
make test-unit    # Unit tests only (fast)
make test-cover   # Tests + HTML coverage report
make bench        # Benchmarks with memory stats
make help         # Show all commands
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full developer guide: test organization, debugging workflow, how to add
tests, and architecture documentation.

## Test Coverage

50+ tests across four test files, covering SSIM correctness, compression quality presets, format auto-selection, resize
with aspect ratio, target size strategies, median-cut quantization, PNG optimization, edge cases (nil images, empty
images, dimension mismatches), and CLI integration.

```bash
$ make test-cover
ok   github.com/shamspias/fennec   coverage: 88.3% of statements
```

## License

MIT