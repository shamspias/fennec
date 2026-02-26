# 🦊 Fennec

**Tiny Fox. Giant Ears.** Hears what matters, drops what doesn't.

Fennec is a pure Go image compression library that dramatically reduces file sizes while preserving perceptual quality.
It uses **SSIM-guided optimization** — binary searching for the minimum file size that maintains your target visual
quality — instead of guessing at compression parameters.

## Why Fennec?

Traditional image compression asks: *"What quality number should I use?"*

Fennec asks: *"What's the smallest file that still looks great to human eyes?"*

| Feature          | Traditional         | Fennec                              |
|------------------|---------------------|-------------------------------------|
| Quality control  | Fixed number (Q=75) | Perceptual target (SSIM ≥ 0.94)     |
| Format selection | Manual              | Auto-analyzed per image             |
| Resize quality   | Basic bilinear      | Lanczos-3 with pre-multiplied alpha |
| PNG optimization | Generic             | Auto-palettize, grayscale detection |
| Transparency     | Often breaks        | Pre-multiplied alpha compositing    |

## Installation

```bash
go get github.com/shamspias/fennec
```

## Quick Start

```go
package main

import (
	"fmt"
	"github.com/shamspias/fennec"
)

func main() {
	// One-line compression with smart defaults.
	result, err := fennec.CompressFile("photo.jpg", "optimized.jpg", fennec.DefaultOptions())
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
	// Fennec Result: JPEG | 4000x3000 → 4000x3000 | 3.2 MB → 420.5 KB | SSIM: 0.9512 | Saved: 87.2%
}
```

## Usage

### Quality Presets

```go
opts := fennec.DefaultOptions()

opts.Quality = fennec.Ultra // SSIM ≥ 0.99 — pixel-perfect
opts.Quality = fennec.High // SSIM ≥ 0.97 — excellent quality
opts.Quality = fennec.Balanced    // SSIM ≥ 0.94 — great balance (default)
opts.Quality = fennec.Aggressive  // SSIM ≥ 0.90 — strong compression
opts.Quality = fennec.Maximum // SSIM ≥ 0.85 — extreme compression
opts.Quality = fennec.Lossless // PNG only, zero loss
```

### Compress with Resize

```go
opts := fennec.DefaultOptions()
opts.MaxWidth = 1920
opts.MaxHeight = 1080
opts.Quality = fennec.Balanced

result, err := fennec.CompressFile("huge_photo.jpg", "web_ready.jpg", opts)
```

### Target a Specific File Size

```go
opts := fennec.DefaultOptions()
opts.Format = fennec.JPEG
opts.TargetSize = 100 * 1024 // 100 KB

result, err := fennec.CompressFile("photo.jpg", "exactly_100kb.jpg", opts)
```

### Auto Format Selection

```go
opts := fennec.DefaultOptions()
opts.Format = fennec.Auto // Fennec analyzes and picks JPEG or PNG

// Photographs → JPEG (better compression for complex images)
// Screenshots, logos, transparency → PNG (lossless, palette optimization)
```

### Image Analysis

```go
img, _ := fennec.Open("photo.jpg")
stats := fennec.Analyze(img)

fmt.Printf("Entropy: %.2f bits\n", stats.Entropy)
fmt.Printf("Edge density: %.1f%%\n", stats.EdgeDensity*100)
fmt.Printf("Recommended: %v @ %v\n", stats.RecommendedFormat, stats.RecommendedQuality)
fmt.Printf("Est. compression: %.1fx\n", stats.EstimatedCompression)
```

### Measure Quality (SSIM)

```go
original, _ := fennec.Open("original.jpg")
compressed, _ := fennec.Open("compressed.jpg")

ssim := fennec.SSIM(original, compressed)
fmt.Printf("SSIM: %.4f\n", ssim) // 1.0 = identical, >0.95 = excellent

// Multi-Scale SSIM (better correlates with human perception)
msssim := fennec.MSSSIM(original, compressed)
fmt.Printf("MS-SSIM: %.4f\n", msssim)
```

### Programmatic Compression

```go
img, _ := fennec.Open("input.png")

opts := fennec.DefaultOptions()
opts.Format = fennec.JPEG
opts.Quality = fennec.High

result, err := fennec.CompressImage(img, opts)
if err != nil {
panic(err)
}

fmt.Printf("JPEG Q%d, SSIM: %.4f, Size: %d bytes\n",
result.JPEGQuality, result.SSIM, result.CompressedSize)
```

### Edge-Preserving Sharpening

```go
img, _ := fennec.Open("photo.jpg")
nrgba := fennec.ToNRGBA(img) // not exported, use within pipeline

// Adaptive sharpen: only sharpens edges, leaves smooth areas alone
sharpened := fennec.AdaptiveSharpen(nrgba, 0.5)
```

## CLI Tool

```bash
go install github.com/xero/fennec/cmd/fennec@latest
```

```bash
# Balanced compression (default)
fennec photo.jpg compressed.jpg

# Ultra quality
fennec -quality ultra photo.png out.png

# Aggressive compression with max dimensions
fennec -quality aggressive -max-width 1920 photo.jpg out.jpg

# Target specific file size
fennec -target-size 100KB photo.jpg out.jpg

# Analyze without compressing
fennec -analyze photo.jpg
```

## How It Works

### SSIM-Guided Binary Search

Instead of guessing a JPEG quality number, Fennec:

1. Sets bounds: Q1 (lowest) to Q100 (highest)
2. Tries the midpoint quality
3. Decodes the result and computes SSIM against the original
4. If SSIM ≥ target → tries lower quality (smaller file)
5. If SSIM < target → tries higher quality (better quality)
6. Converges on the optimal quality in ~7 iterations

This means every image gets exactly the compression it can handle.

### Smart PNG Optimization

Fennec automatically:

- **Palettizes** images with ≤256 colors (indexed PNG = ~75% smaller)
- **Detects grayscale** content (1 channel vs 3 = ~66% smaller)
- Uses **maximum compression** level

### Content-Aware Format Selection

Fennec analyzes:

- **Color count**: Few colors → PNG, many → JPEG
- **Transparency**: Any alpha → PNG
- **Edge density**: High edges + few colors → PNG (screenshots, text)
- **Entropy**: High information density → JPEG

### Pre-Multiplied Alpha Resize

Unlike most Go image libraries, Fennec's Lanczos resize uses pre-multiplied alpha interpolation. This prevents the dark
fringing artifacts that occur at transparency boundaries.

## Architecture

```
fennec.go      → Public API, format selection, orchestration
ssim.go        → SSIM & MS-SSIM quality metrics  
compress.go    → JPEG/PNG compression with SSIM optimization
resize.go      → Lanczos-3 resize with pre-multiplied alpha
analyze.go     → Image analysis (entropy, edges, color stats)
effects.go     → Sharpening, blur, edge-preserving filters
cmd/fennec/    → CLI tool
```

## Benchmarks

On Intel Xeon Platinum (2 cores):

| Operation                | Image Size | Time  | Allocations |
|--------------------------|------------|-------|-------------|
| SSIM                     | 500×500    | 86ms  | 6 MB        |
| SSIMFast                 | 1000×1000  | 30ms  | 1.6 MB      |
| Lanczos Resize           | 1000→500   | 47ms  | 3.2 MB      |
| JPEG Compress (Balanced) | 500×500    | 272ms | 24 MB       |
| Analyze                  | 1000×1000  | 10ms  | 4 MB        |
| Gaussian Blur σ=2        | 500×500    | 36ms  | 2 MB        |

## License

MIT

---

*Named after the Fennec fox — the smallest canid with the largest ears relative to body size. It hears everything, but
only keeps what matters.*