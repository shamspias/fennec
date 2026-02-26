---
name: fennec-image-compression
description: Use this skill when asked to compress, resize, or analyze images in Go using the Fennec library, or when modifying the Fennec codebase itself.
---

# Fennec Image Compression Skill

Fennec is a pure-Go image compression library that uses SSIM-guided optimization to find the smallest file size that
preserves visual quality.

## Core Architecture & Rules

* **Pure Go:** Fennec has zero external dependencies and does not use cgo. Do not introduce third-party packages.
* **SSIM-Guided:** Fennec binary-searches for the lowest JPEG quality that meets a target SSIM, rather than using
  arbitrary quality numbers.
* **Format Auto-Selection:** Fennec automatically selects between JPEG and PNG based on factors like alpha transparency,
  color count, and edge density.

## Usage Examples

### 1. Basic File Compression (Quality-based)

When a user wants to compress an image with a quality preset (Lossless, Ultra, High, Balanced, Aggressive, Maximum):

```go
import "github.com/shamspias/fennec"

opts := fennec.DefaultOptions()
opts.Quality = fennec.Balanced // Targets SSIM ≥ 0.94
result, err := fennec.CompressFile("input.jpg", "output.jpg", opts)

```

### 2. Target File Size Compression

When hitting a specific file size target in bytes, Fennec uses a multi-strategy engine (JPEG quality search, color
quantization, scaling):

```go
opts := fennec.DefaultOptions()
opts.TargetSize = 100 * 1024 // 100 KB
opts.Format = fennec.Auto
result, err := fennec.CompressFile("input.png", "output.jpg", opts)

```

### 3. Resizing and Compressing

Fennec uses Lanczos-3 interpolation with pre-multiplied alpha to prevent dark fringing on transparent edges:

```go
opts := fennec.DefaultOptions()
opts.MaxWidth = 1920
opts.MaxHeight = 1080
result, err := fennec.CompressFile("huge.jpg", "web.jpg", opts)

```

### 4. Image Analysis

To analyze an image's characteristics without compressing it:

```go
img, err := fennec.Open("photo.jpg")
stats := fennec.Analyze(img)
// Returns stats.RecommendedFormat, stats.Entropy, stats.UniqueColors, etc.

```

## Contributor Guidelines

If you are tasked with modifying the Fennec codebase:

* **Test Fixtures:** Fennec's integration tests rely on real image files in `testdata/`. If tests fail with "testdata
  missing", you must run `make fixtures` first to generate the synthetic deterministic images.
* **Performance:** When modifying files like `compress.go` or `targetsize.go`, run `make bench` to check for memory
  allocation and performance regressions.
* **SSIM Checks:** The `SSIM()` and `SSIMFast()` functions require `*image.NRGBA` inputs. Always ensure images are
  converted using `toNRGBA()` before comparison.
