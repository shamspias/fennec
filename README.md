# 🦊 Fennec

**Tiny Fox. Giant Ears. Hears what matters, drops what doesn't.**

Fennec is a zero-dependency Go library for intelligent image compression. It uses SSIM (Structural Similarity Index) to
find the sweet spot between file size and perceptual quality — so you compress as much as possible without humans
noticing.

[![Go Reference](https://pkg.go.dev/badge/github.com/shamspias/fennec.svg)](https://pkg.go.dev/github.com/shamspias/fennec)
[![Go Report Card](https://goreportcard.com/badge/github.com/shamspias/fennec)](https://goreportcard.com/report/github.com/shamspias/fennec)

---

## Why Fennec?

Most image libraries make you pick a quality number and hope for the best. Fennec measures actual perceptual quality and
optimizes precisely:

| Feature                 | imaging | bild | gift | **Fennec** |
|-------------------------|---------|------|------|------------|
| SSIM-guided compression | ❌       | ❌    | ❌    | ✅          |
| Target file size        | ❌       | ❌    | ❌    | ✅          |
| Auto format selection   | ❌       | ❌    | ❌    | ✅          |
| EXIF auto-orient        | ✅       | ❌    | ❌    | ✅          |
| Batch processing        | ❌       | ❌    | ❌    | ✅          |
| context.Context         | ❌       | ❌    | ❌    | ✅          |
| Progress callbacks      | ❌       | ❌    | ❌    | ✅          |
| Zero dependencies       | ❌       | ✅    | ❌    | ✅          |
| Lanczos-3 resize        | ✅       | ❌    | ✅    | ✅          |
| MS-SSIM                 | ❌       | ❌    | ❌    | ✅          |

---

## Install

```bash
go get github.com/shamspias/fennec@latest
```

CLI tool:

```bash
go install github.com/shamspias/fennec/cmd/fennec@latest
```

**Requirements:** Go 1.22+. Zero external dependencies.

---

## Quick Start

### One-liner compression

```go
result, err := fennec.CompressFile(ctx, "photo.jpg", "optimized.jpg", fennec.DefaultOptions())
// → JPEG Q=42 | 4032x3024 → 4032x3024 | 3.2 MB → 412 KB | SSIM: 0.9456 | Saved: 87.1%
```

### Server-side bytes → bytes

```go
// Receive upload, compress, return — the most common server pattern.
result, err := fennec.CompressBytes(ctx, uploadData, fennec.Options{
Quality:  fennec.High, // SSIM ≥ 0.97
MaxWidth: 1920,
})
optimized := result.Bytes() // Ready for S3, CDN, HTTP response
```

### Target a specific file size

```go
result, err := fennec.CompressFile(ctx, "hero.jpg", "hero_web.jpg", fennec.Options{
TargetSize: 100 * 1024, // Hit 100 KB — Fennec tries JPEG quality, scaling, quantization
})
```

### Analyze before compressing

```go
stats := fennec.Analyze(img)
fmt.Printf("Recommended: %s at %s (entropy: %.1f, edges: %.0f%%)\n",
stats.RecommendedFormat, stats.RecommendedQuality,
stats.Entropy, stats.EdgeDensity*100)
```

### Batch processing with worker pool

```go
items := []fennec.BatchItem{
{Src: "photos/001.jpg", Dst: "out/001.jpg"},
{Src: "photos/002.jpg", Dst: "out/002.jpg"},
{Src: "photos/003.png", Dst: "out/003.png"},
// ... hundreds more
}

results := fennec.CompressBatch(ctx, items, fennec.BatchOptions{
Workers:     8,
DefaultOpts: fennec.DefaultOptions(),
OnItem: func (done, total int) {
fmt.Printf("\r%d/%d", done, total)
},
})

summary := fennec.Summarize(results)
fmt.Println(summary)
// → Batch: 312/312 succeeded | 89.4 MB saved | Avg SSIM: 0.9523
```

### Progress callbacks & cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

opts := fennec.DefaultOptions()
opts.OnProgress = func (stage fennec.ProgressStage, pct float64) error {
fmt.Printf("\r%s %.0f%%", stage, pct*100)
return nil // Return error to abort
}

result, err := fennec.CompressFile(ctx, src, dst, opts)
```

### EXIF auto-orientation

```go
// Automatically reads EXIF orientation and rotates.
// Enabled by default — no third-party EXIF library needed.
img, err := fennec.OpenAndOrient("camera_photo.jpg")

// Or disable it:
opts := fennec.DefaultOptions()
opts.AutoOrient = false
```

### SSIM comparison

```go
ssim := fennec.SSIM(original, compressed) // Full precision
fast := fennec.SSIMFast(nrgba1, nrgba2)         // ~20ms for 4K
msssim := fennec.MSSSIM(original, compressed) // Multi-scale (best correlation with human perception)
```

### Effects

```go
sharp := fennec.Sharpen(img, 0.5) // Unsharp mask
adaptive := fennec.AdaptiveSharpen(img, 0.3)    // Edge-aware (preserves smooth areas)
blurred := fennec.GaussianBlur(img, 2.0) // Separable Gaussian
```

---

## Quality Presets

| Preset       | SSIM Target | Use Case                                    |
|--------------|-------------|---------------------------------------------|
| `Lossless`   | 1.00        | Archival, medical imaging, pixel art        |
| `Ultra`      | ≥ 0.99      | Professional photography, print             |
| `High`       | ≥ 0.97      | Portfolio, e-commerce product shots         |
| `Balanced`   | ≥ 0.94      | **Default.** Web images, social media       |
| `Aggressive` | ≥ 0.90      | Thumbnails, previews, bandwidth-constrained |
| `Maximum`    | ≥ 0.85      | Extreme compression, low-bandwidth mobile   |

The zero value of `Options{}` uses `Balanced` — you get great results without configuring anything.

---

## CLI

```
fennec [flags] <input> [output]

Flags:
  -quality string     lossless|ultra|high|balanced|aggressive|maximum (default "balanced")
  -format string      auto|jpeg|png (default "auto")
  -max-width int      Maximum width (0 = no limit)
  -max-height int     Maximum height (0 = no limit)
  -target-size string Target file size (e.g. 100KB, 2MB)
  -ssim float         Custom SSIM target (0.0-1.0, overrides quality)
  -no-orient          Don't auto-rotate based on EXIF orientation
  -analyze            Analyze image without compressing
```

Examples:

```bash
# Basic compression with defaults (Balanced, SSIM ≥ 0.94)
fennec photo.jpg compressed.jpg

# High quality, capped at 1920px wide
fennec -quality high -max-width 1920 photo.jpg web.jpg

# Hit a target file size
fennec -target-size 200KB hero.jpg hero_web.jpg

# Analyze without compressing
fennec -analyze photo.jpg
# → Dimensions: 4032 x 3024
# → Entropy: 7.12 bits | Edge density: 23.1%
# → Recommended: JPEG at Balanced
```

---

### How SSIM-Guided Compression Works

1. **Binary search** over JPEG quality (1–100)
2. At each step: encode → decode → compute SSIM against original
3. Find the **lowest quality** that still meets the target SSIM
4. Cache the winning encoded buffer (no double-encode)

This typically achieves 60–90% size reduction at SSIM ≥ 0.94, meaning the compressed version is visually
indistinguishable from the original.

### Target Size Engine

When you specify a `TargetSize`, Fennec tries four strategies and picks the best:

1. **JPEG quality search** — binary search for quality that fits
2. **Color quantization** — median-cut to indexed PNG (great for illustrations)
3. **Quality + scale** — combined quality reduction and downscaling
4. **Scale search** — progressive downscaling (last resort)

---

## API Reference

### Core Functions

| Function                               | Description                        |
|----------------------------------------|------------------------------------|
| `CompressFile(ctx, src, dst, opts)`    | File → file compression            |
| `CompressImage(ctx, img, opts)`        | `image.Image` → `Result`           |
| `Compress(ctx, reader, opts)`          | `io.Reader` → `Result`             |
| `CompressBytes(ctx, data, opts)`       | `[]byte` → `Result`                |
| `CompressBatch(ctx, items, batchOpts)` | Concurrent batch compression       |
| `Analyze(img)`                         | Image analysis without compression |

### SSIM Functions

| Function         | Description                                  |
|------------------|----------------------------------------------|
| `SSIM(a, b)`     | Full-precision windowed SSIM                 |
| `SSIMFast(a, b)` | Fast SSIM at 512px resolution (~20ms for 4K) |
| `MSSSIM(a, b)`   | Multi-Scale SSIM                             |

### I/O Functions

| Function                       | Description                     |
|--------------------------------|---------------------------------|
| `Open(path)`                   | Decode image from file          |
| `OpenAndOrient(path)`          | Decode + apply EXIF orientation |
| `Save(img, path, opts)`        | Save with auto-detected format  |
| `Encode(w, img, format, opts)` | Encode to writer                |

### Effects

| Function                         | Description             |
|----------------------------------|-------------------------|
| `Sharpen(img, strength)`         | Unsharp mask sharpening |
| `AdaptiveSharpen(img, strength)` | Edge-aware sharpening   |
| `GaussianBlur(img, sigma)`       | Separable Gaussian blur |

### Result

```go
type Result struct {
Image          *image.NRGBA // Processed image
CompressedData []byte        // Encoded bytes — use Bytes() or WriteTo()
Format         Format
OriginalSize   int64
CompressedSize int64
SSIM           float64
JPEGQuality    int
Ratio          float64
SavingsPercent float64
OriginalDimensions  image.Point
FinalDimensions     image.Point
}

// Write compressed bytes to any writer (http.ResponseWriter, file, S3, etc.)
result.WriteTo(w)

// Get raw bytes
data := result.Bytes()
```

---

## Development

```bash
make              # fmt + vet + test + build
make test         # Run all tests
make test-unit    # Unit tests only (fast)
make bench        # Benchmarks
make test-cover   # Generate HTML coverage report
make lint         # Run staticcheck
make fixtures     # Generate test images
make clean        # Remove artifacts
```

---

## Benchmarks

Run `make bench` to see results on your hardware. Typical numbers on Apple M2:

| Operation                | Image Size | Time  | Allocs |
|--------------------------|------------|-------|--------|
| SSIMFast                 | 1000×1000  | ~8ms  | 4      |
| Lanczos resize 50%       | 1000×1000  | ~12ms | 3      |
| CompressImage (Balanced) | 500×500    | ~45ms | 18     |
| Analyze                  | 1000×1000  | ~5ms  | 2      |
| GaussianBlur σ=2         | 500×500    | ~3ms  | 3      |

---

## License

MIT — see [LICENSE](LICENSE).