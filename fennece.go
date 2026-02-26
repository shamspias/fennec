// Package fennec provides intelligent image compression that dramatically reduces
// file size while preserving perceptual quality. It uses SSIM (Structural Similarity
// Index) guided optimization to find the sweet spot between size and quality.
//
// Fennec — Tiny Fox. Giant Ears. Hears what matters, drops what doesn't.
//
// Unlike traditional image processing libraries that apply fixed compression,
// Fennec analyzes each image and adapts its strategy:
//
//   - SSIM-guided quality search: binary search for minimum file size at target quality
//   - Perceptual color quantization: reduce colors where human eyes can't tell
//   - Content-aware downscaling: smart resize that preserves important detail
//   - Adaptive format selection: picks JPEG/PNG based on image characteristics
//   - Edge-preserving compression: protects sharp edges while compressing smooth areas
package fennec

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Version is the library version.
const Version = "1.0.0"

// Format represents an output image format.
type Format int

const (
	// Auto lets Fennec choose the best format based on image analysis.
	Auto Format = iota
	// JPEG for photographs and complex images.
	JPEG
	// PNG for images with transparency, text, or sharp edges.
	PNG
)

// Quality presets define compression aggressiveness.
type Quality int

const (
	// Lossless preserves every pixel (PNG only, no quality loss).
	Lossless Quality = iota
	// Ultra targets SSIM >= 0.99 — visually identical to original.
	Ultra
	// High targets SSIM >= 0.97 — excellent quality, good compression.
	High
	// Balanced targets SSIM >= 0.94 — great quality, strong compression.
	Balanced
	// Aggressive targets SSIM >= 0.90 — good quality, maximum compression.
	Aggressive
	// Maximum targets SSIM >= 0.85 — acceptable quality, extreme compression.
	Maximum
)

func (q Quality) targetSSIM() float64 {
	switch q {
	case Lossless:
		return 1.0
	case Ultra:
		return 0.99
	case High:
		return 0.97
	case Balanced:
		return 0.94
	case Aggressive:
		return 0.90
	case Maximum:
		return 0.85
	default:
		return 0.94
	}
}

func (q Quality) String() string {
	switch q {
	case Lossless:
		return "Lossless"
	case Ultra:
		return "Ultra"
	case High:
		return "High"
	case Balanced:
		return "Balanced"
	case Aggressive:
		return "Aggressive"
	case Maximum:
		return "Maximum"
	default:
		return "Unknown"
	}
}

// Options configures the compression behavior.
type Options struct {
	// Quality preset (default: Balanced).
	Quality Quality

	// Format specifies the output format. Auto will analyze the image.
	Format Format

	// MaxWidth constrains the output width. 0 means no constraint.
	// Aspect ratio is always preserved.
	MaxWidth int

	// MaxHeight constrains the output height. 0 means no constraint.
	// Aspect ratio is always preserved.
	MaxHeight int

	// StripMetadata removes EXIF and other metadata (default: true).
	StripMetadata bool

	// Subsample enables chroma subsampling for JPEG (default: true).
	// This exploits the fact that human eyes are less sensitive to
	// color detail than luminance detail.
	Subsample bool

	// TargetSSIM overrides the Quality preset with a custom SSIM target.
	// Must be between 0.0 and 1.0. 0 means use the Quality preset.
	TargetSSIM float64

	// TargetSize tries to achieve a specific file size in bytes.
	// 0 means no size target (use quality-based optimization).
	TargetSize int
}

// DefaultOptions returns sensible defaults for general use.
func DefaultOptions() Options {
	return Options{
		Quality:       Balanced,
		Format:        Auto,
		StripMetadata: true,
		Subsample:     true,
	}
}

// Result contains compression results and statistics.
type Result struct {
	// Image is the final (possibly resized) image.
	Image *image.NRGBA

	// compressedData holds the actual encoded bytes (JPEG or PNG).
	// This avoids the double-encode bug where CompressFile would re-encode.
	compressedData []byte

	// Format is the chosen output format.
	Format Format

	// OriginalSize is the original image size in bytes (if known).
	OriginalSize int64

	// CompressedSize is the compressed output size in bytes.
	CompressedSize int64

	// SSIM is the structural similarity between original and compressed.
	SSIM float64

	// JPEGQuality is the JPEG quality used (0 if PNG).
	JPEGQuality int

	// Ratio is the compression ratio (original / compressed).
	Ratio float64

	// SavingsPercent is the percentage of bytes saved.
	SavingsPercent float64

	// OriginalDimensions is the original width x height.
	OriginalDimensions image.Point

	// FinalDimensions is the output width x height.
	FinalDimensions image.Point
}

func (r Result) String() string {
	format := "JPEG"
	if r.Format == PNG {
		format = "PNG"
	}
	qStr := ""
	if r.Format == JPEG && r.JPEGQuality > 0 {
		qStr = fmt.Sprintf(" Q=%d |", r.JPEGQuality)
	}
	return fmt.Sprintf(
		"Fennec Result: %s |%s %dx%d → %dx%d | %s → %s | SSIM: %.4f | Saved: %.1f%%",
		format, qStr,
		r.OriginalDimensions.X, r.OriginalDimensions.Y,
		r.FinalDimensions.X, r.FinalDimensions.Y,
		humanBytes(r.OriginalSize), humanBytes(r.CompressedSize),
		r.SSIM, r.SavingsPercent,
	)
}

// Compress reads an image from r and returns the optimally compressed version.
func Compress(r io.Reader, opts Options) (*Result, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("fennec: decode failed: %w", err)
	}
	return CompressImage(img, opts)
}

// CompressImage compresses an already-decoded image.
func CompressImage(img image.Image, opts Options) (*Result, error) {
	if img == nil {
		return nil, fmt.Errorf("fennec: nil image")
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, fmt.Errorf("fennec: empty image")
	}

	result := &Result{
		OriginalDimensions: image.Pt(bounds.Dx(), bounds.Dy()),
	}

	// Step 1: Convert to NRGBA for uniform processing.
	src := toNRGBA(img)

	// Step 2: Smart resize if dimensions are constrained.
	if opts.MaxWidth > 0 || opts.MaxHeight > 0 {
		src = smartResize(src, opts.MaxWidth, opts.MaxHeight)
	}

	result.Image = src
	result.FinalDimensions = image.Pt(src.Bounds().Dx(), src.Bounds().Dy())

	// ── Target Size Mode ────────────────────────────────────────────────
	// Uses the multi-strategy engine (targetsize.go) which tries:
	//   1. JPEG quality search
	//   2. Color quantization → indexed PNG
	//   3. JPEG quality + downscale
	//   4. Binary search on scale factor
	if opts.TargetSize > 0 {
		sr, err := hitTargetSize(src, opts.TargetSize, opts)
		if err != nil {
			return nil, fmt.Errorf("fennec: target-size compression failed: %w", err)
		}
		result.compressedData = sr.data
		result.Format = sr.format
		result.JPEGQuality = sr.quality
		result.SSIM = sr.ssim
		result.FinalDimensions = image.Pt(sr.finalW, sr.finalH)
		if sr.img != nil {
			result.Image = sr.img
		}
		result.CompressedSize = int64(len(sr.data))
		if result.OriginalSize > 0 {
			result.Ratio = float64(result.OriginalSize) / float64(result.CompressedSize)
			result.SavingsPercent = (1 - float64(result.CompressedSize)/float64(result.OriginalSize)) * 100
		}
		return result, nil
	}

	// ── Standard Mode: SSIM-Guided Compression ──────────────────────────
	if opts.Format == Auto {
		opts.Format = analyzeFormat(src)
	}
	result.Format = opts.Format

	targetSSIM := opts.Quality.targetSSIM()
	if opts.TargetSSIM > 0 && opts.TargetSSIM <= 1.0 {
		targetSSIM = opts.TargetSSIM
	}

	var compressed bytes.Buffer

	switch opts.Format {
	case PNG:
		if err := compressPNG(src, &compressed, opts); err != nil {
			return nil, fmt.Errorf("fennec: PNG compression failed: %w", err)
		}
		result.SSIM = 1.0
	case JPEG:
		quality, ssim, err := compressJPEGOptimal(src, &compressed, targetSSIM, opts)
		if err != nil {
			return nil, fmt.Errorf("fennec: JPEG compression failed: %w", err)
		}
		result.JPEGQuality = quality
		result.SSIM = ssim
	default:
		return nil, fmt.Errorf("fennec: unsupported format")
	}

	result.compressedData = compressed.Bytes()
	result.CompressedSize = int64(compressed.Len())
	if result.OriginalSize > 0 {
		result.Ratio = float64(result.OriginalSize) / float64(result.CompressedSize)
		result.SavingsPercent = (1 - float64(result.CompressedSize)/float64(result.OriginalSize)) * 100
	}

	return result, nil
}

// CompressFile compresses an image file and writes the result to dst.
func CompressFile(src, dst string, opts Options) (*Result, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("fennec: open failed: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("fennec: stat failed: %w", err)
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("fennec: decode failed: %w", err)
	}

	result, err := CompressImage(img, opts)
	if err != nil {
		return nil, err
	}
	result.OriginalSize = stat.Size()

	// Use the compressed bytes from CompressImage directly.
	// This avoids the double-encode bug and ensures target-size results
	// are written exactly as computed.
	data := result.compressedData
	if len(data) == 0 {
		// Fallback: encode from the image if no compressed data stored.
		var buf bytes.Buffer
		switch result.Format {
		case JPEG:
			err = jpeg.Encode(&buf, result.Image, &jpeg.Options{Quality: result.JPEGQuality})
		case PNG:
			encoder := png.Encoder{CompressionLevel: png.BestCompression}
			err = encoder.Encode(&buf, result.Image)
		}
		if err != nil {
			return nil, fmt.Errorf("fennec: encode failed: %w", err)
		}
		data = buf.Bytes()
	}

	result.CompressedSize = int64(len(data))
	result.Ratio = float64(result.OriginalSize) / float64(result.CompressedSize)
	result.SavingsPercent = (1 - float64(result.CompressedSize)/float64(result.OriginalSize)) * 100

	if err := os.WriteFile(dst, data, 0644); err != nil {
		return nil, fmt.Errorf("fennec: write failed: %w", err)
	}

	return result, nil
}

// Encode writes the image to w in the specified format with Fennec optimization.
func Encode(w io.Writer, img image.Image, format Format, opts Options) error {
	src := toNRGBA(img)

	switch format {
	case JPEG:
		targetSSIM := opts.Quality.targetSSIM()
		if opts.TargetSSIM > 0 {
			targetSSIM = opts.TargetSSIM
		}
		_, _, err := compressJPEGOptimal(src, w, targetSSIM, opts)
		return err
	case PNG:
		return compressPNG(src, w, opts)
	default:
		return fmt.Errorf("fennec: unsupported format")
	}
}

// Open loads an image from a file path.
func Open(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

// Save saves the image to a file, auto-detecting format from extension.
func Save(img image.Image, filename string, opts Options) error {
	ext := strings.ToLower(filepath.Ext(filename))
	var format Format
	switch ext {
	case ".jpg", ".jpeg":
		format = JPEG
	case ".png":
		format = PNG
	case ".bmp", ".tif", ".tiff":
		return fmt.Errorf("fennec: %s format not supported, use .jpg or .png", ext)
	default:
		return fmt.Errorf("fennec: unsupported extension %q", ext)
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return Encode(f, img, format, opts)
}

// analyzeFormat examines the image to determine the best output format.
// Images with transparency or very few colors → PNG.
// Photographic images with many colors → JPEG.
func analyzeFormat(img *image.NRGBA) Format {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Sample pixels to check for transparency and color count.
	hasAlpha := false
	colorSet := make(map[color.NRGBA]struct{})
	maxSamples := 10000
	step := 1
	total := w * h
	if total > maxSamples {
		step = total / maxSamples
		if step < 1 {
			step = 1
		}
	}

	idx := 0
	for y := 0; y < h && len(colorSet) < 512; y++ {
		for x := 0; x < w && len(colorSet) < 512; x++ {
			if idx%step != 0 {
				idx++
				continue
			}
			off := y*img.Stride + x*4
			a := img.Pix[off+3]
			if a < 255 {
				hasAlpha = true
			}
			c := color.NRGBA{img.Pix[off], img.Pix[off+1], img.Pix[off+2], a}
			colorSet[c] = struct{}{}
			idx++
		}
	}

	// Transparency requires PNG.
	if hasAlpha {
		return PNG
	}

	// Few unique colors (icons, logos, screenshots) → PNG compresses better.
	if len(colorSet) < 256 {
		return PNG
	}

	// Default to JPEG for photographic content.
	return JPEG
}

// toNRGBA converts any image.Image to *image.NRGBA.
func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		// Return a copy to avoid mutating the original.
		bounds := nrgba.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		copy(dst.Pix, nrgba.Pix)
		return dst
	}

	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			off := (y-bounds.Min.Y)*dst.Stride + (x-bounds.Min.X)*4
			if a == 0 {
				dst.Pix[off] = 0
				dst.Pix[off+1] = 0
				dst.Pix[off+2] = 0
				dst.Pix[off+3] = 0
			} else if a == 0xffff {
				dst.Pix[off] = uint8(r >> 8)
				dst.Pix[off+1] = uint8(g >> 8)
				dst.Pix[off+2] = uint8(b >> 8)
				dst.Pix[off+3] = 0xff
			} else {
				dst.Pix[off] = uint8(((r * 0xffff) / a) >> 8)
				dst.Pix[off+1] = uint8(((g * 0xffff) / a) >> 8)
				dst.Pix[off+2] = uint8(((b * 0xffff) / a) >> 8)
				dst.Pix[off+3] = uint8(a >> 8)
			}
		}
	}
	return dst
}

func humanBytes(b int64) string {
	if b == 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	i := 0
	bf := float64(b)
	for bf >= 1024 && i < len(units)-1 {
		bf /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f %s", bf, units[i])
}

func clampF(x float64) uint8 {
	v := int64(math.Round(x))
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}
