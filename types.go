package fennec

import (
	"context"
	"fmt"
	"image"
	"io"
)

// Version is the library version.
const Version = "2.0.0"

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

func (f Format) String() string {
	switch f {
	case JPEG:
		return "JPEG"
	case PNG:
		return "PNG"
	default:
		return "Auto"
	}
}

// Quality presets define compression aggressiveness.
// The zero value is Balanced, which is the recommended default.
type Quality int

const (
	// Balanced targets SSIM >= 0.94 — great quality, strong compression (default).
	Balanced Quality = iota
	// Lossless preserves every pixel (PNG only, no quality loss).
	Lossless
	// Ultra targets SSIM >= 0.99 — visually identical to original.
	Ultra
	// High targets SSIM >= 0.97 — excellent quality, good compression.
	High
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

// String returns the human-readable name of the quality preset.
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

// ProgressStage describes what the compressor is currently doing.
type ProgressStage string

const (
	StageAnalyzing   ProgressStage = "analyzing"
	StageResizing    ProgressStage = "resizing"
	StageCompressing ProgressStage = "compressing"
	StageOptimizing  ProgressStage = "optimizing"
	StageEncoding    ProgressStage = "encoding"
	StageWriting     ProgressStage = "writing"
)

// ProgressFunc is called during compression to report progress.
// stage describes the current operation, percent is 0.0–1.0.
// Return a non-nil error to abort the operation.
type ProgressFunc func(stage ProgressStage, percent float64) error

// Options configures the compression behavior.
type Options struct {
	// Quality preset (default: Balanced, the zero value).
	Quality Quality

	// Format specifies the output format. Auto will analyze the image.
	Format Format

	// MaxWidth constrains the output width. 0 means no constraint.
	// Aspect ratio is always preserved.
	MaxWidth int

	// MaxHeight constrains the output height. 0 means no constraint.
	// Aspect ratio is always preserved.
	MaxHeight int

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

	// AutoOrient reads EXIF orientation data and auto-rotates the image.
	// Default: true. Set to false to preserve original pixel orientation.
	AutoOrient bool

	// OnProgress is called during compression to report progress.
	// Optional. Returning a non-nil error aborts the operation.
	OnProgress ProgressFunc
}

// DefaultOptions returns sensible defaults for general use.
func DefaultOptions() Options {
	return Options{
		Quality:    Balanced,
		Format:     Auto,
		Subsample:  true,
		AutoOrient: true,
	}
}

// reportProgress safely invokes the progress callback if set.
// Returns context error or progress callback error.
func (o *Options) reportProgress(ctx context.Context, stage ProgressStage, percent float64) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if o.OnProgress != nil {
		return o.OnProgress(stage, percent)
	}
	return nil
}

// Result contains compression results and statistics.
type Result struct {
	// Image is the final processed image (resized, oriented).
	Image *image.NRGBA

	// CompressedData holds the actual encoded bytes (JPEG or PNG).
	// Use WriteTo to write this data to any io.Writer.
	CompressedData []byte

	// Format is the chosen output format.
	Format Format

	// OriginalSize is the original image size in bytes (if known from file).
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

// WriteTo writes the compressed image data to w.
// This writes the exact bytes that were produced by the compression engine,
// preserving target-size precision.
func (r *Result) WriteTo(w io.Writer) (int64, error) {
	if len(r.CompressedData) == 0 {
		return 0, fmt.Errorf("fennec: no compressed data available")
	}
	n, err := w.Write(r.CompressedData)
	return int64(n), err
}

// Bytes returns the compressed image data as a byte slice.
func (r *Result) Bytes() []byte {
	return r.CompressedData
}

// String returns a human-readable summary of the compression result.
func (r *Result) String() string {
	format := r.Format.String()
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

// computeStats fills in the computed fields (Ratio, SavingsPercent) from sizes.
func (r *Result) computeStats() {
	if r.OriginalSize > 0 && r.CompressedSize > 0 {
		r.Ratio = float64(r.OriginalSize) / float64(r.CompressedSize)
		r.SavingsPercent = (1 - float64(r.CompressedSize)/float64(r.OriginalSize)) * 100
	}
}
