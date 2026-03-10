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
//   - EXIF orientation handling: auto-rotates images from camera sensors
//   - Batch processing: concurrent compression with worker pools
package fennec

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
)

// CompressFile compresses an image file and writes the result to dst.
// It reads EXIF orientation data and auto-rotates if opts.AutoOrient is true.
// The context can be used to cancel long-running operations.
func CompressFile(ctx context.Context, src, dst string, opts Options) (*Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	if err := opts.reportProgress(ctx, StageAnalyzing, 0); err != nil {
		return nil, err
	}

	img, orient, fileSize, err := openWithOrientation(src)
	if err != nil {
		return nil, err
	}

	result, err := compressImageInternal(ctx, img, orient, opts)
	if err != nil {
		return nil, err
	}
	result.OriginalSize = fileSize
	result.computeStats()

	if err := opts.reportProgress(ctx, StageWriting, 0.9); err != nil {
		return nil, err
	}

	// Write the pre-computed compressed bytes directly.
	data := result.CompressedData
	if len(data) == 0 {
		data, err = encodeToBytes(result.Image, result.Format, result.JPEGQuality)
		if err != nil {
			return nil, err
		}
		result.CompressedData = data
		result.CompressedSize = int64(len(data))
		result.computeStats()
	}

	if err := os.WriteFile(dst, data, 0644); err != nil {
		return nil, fmt.Errorf("fennec: write %q: %w", dst, err)
	}

	if err := opts.reportProgress(ctx, StageWriting, 1.0); err != nil {
		return nil, err
	}

	return result, nil
}

// CompressImage compresses an already-decoded image.
// The context can be used to cancel long-running operations.
func CompressImage(ctx context.Context, img image.Image, opts Options) (*Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return compressImageInternal(ctx, img, OrientNormal, opts)
}

// Compress reads an image from r and returns the optimally compressed version.
// The context can be used to cancel long-running operations.
func Compress(ctx context.Context, r io.Reader, opts Options) (*Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("fennec: decode: %w", err)
	}
	return compressImageInternal(ctx, img, OrientNormal, opts)
}

// CompressBytes compresses image data from a byte slice and returns the result.
// This is the most common API for server-side use: receive bytes → compress → return bytes.
func CompressBytes(ctx context.Context, data []byte, opts Options) (*Result, error) {
	return Compress(ctx, bytes.NewReader(data), opts)
}

// compressImageInternal is the shared compression pipeline.
func compressImageInternal(ctx context.Context, img image.Image, orient Orientation, opts Options) (*Result, error) {
	if img == nil {
		return nil, ErrNilImage
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, ErrEmptyImage
	}

	result := &Result{OriginalDimensions: image.Pt(bounds.Dx(), bounds.Dy())}
	src := toNRGBA(img)

	if opts.AutoOrient && orient > OrientNormal {
		src = ApplyOrientation(src, orient)
		result.OriginalDimensions = image.Pt(src.Bounds().Dx(), src.Bounds().Dy())
	}
	if err := opts.reportProgress(ctx, StageResizing, 0.1); err != nil {
		return nil, err
	}

	if opts.MaxWidth > 0 || opts.MaxHeight > 0 {
		src = smartResize(src, opts.MaxWidth, opts.MaxHeight)
	}
	result.Image = src
	result.FinalDimensions = image.Pt(src.Bounds().Dx(), src.Bounds().Dy())

	if err := opts.reportProgress(ctx, StageCompressing, 0.2); err != nil {
		return nil, err
	}

	if opts.TargetSize > 0 {
		return handleTargetSizeMode(ctx, src, opts, result)
	}
	return handleStandardMode(ctx, src, opts, result)
}

func handleTargetSizeMode(ctx context.Context, src *image.NRGBA, opts Options, result *Result) (*Result, error) {
	sr, err := hitTargetSize(ctx, src, opts.TargetSize, opts)
	if err != nil {
		return nil, fmt.Errorf("fennec: target-size compression: %w", err)
	}

	result.CompressedData = sr.data
	result.Format = sr.format
	result.JPEGQuality = sr.quality
	result.SSIM = sr.ssim
	result.FinalDimensions = image.Pt(sr.finalW, sr.finalH)
	if sr.img != nil {
		result.Image = sr.img
	}
	result.CompressedSize = int64(len(sr.data))
	result.computeStats()
	return result, nil
}

func handleStandardMode(ctx context.Context, src *image.NRGBA, opts Options, result *Result) (*Result, error) {
	if opts.Format == Auto {
		opts.Format = analyzeFormat(src)
	}
	result.Format = opts.Format

	if err := opts.reportProgress(ctx, StageOptimizing, 0.3); err != nil {
		return nil, err
	}

	var compressed encodingBuffer
	switch opts.Format {
	case PNG:
		if err := compressPNG(src, &compressed, opts); err != nil {
			return nil, fmt.Errorf("fennec: PNG compression: %w", err)
		}
		result.SSIM = 1.0
	case JPEG:
		target := opts.Quality.targetSSIM()
		if opts.TargetSSIM > 0 && opts.TargetSSIM <= 1.0 {
			target = opts.TargetSSIM
		}

		q, ssim, cachedData, err := compressJPEGOptimal(src, &compressed, target, opts)
		if err != nil {
			return nil, fmt.Errorf("fennec: JPEG compression: %w", err)
		}
		result.JPEGQuality, result.SSIM = q, ssim
		if cachedData != nil {
			compressed.Reset()
			compressed.Write(cachedData)
		}
	default:
		return nil, ErrUnsupportedFormat
	}

	if err := opts.reportProgress(ctx, StageEncoding, 0.9); err != nil {
		return nil, err
	}
	result.CompressedData = compressed.Bytes()
	result.CompressedSize = int64(compressed.Len())
	result.computeStats()
	return result, nil
}

// encodingBuffer is a bytes.Buffer wrapper that satisfies io.Writer.
// Named to reflect its purpose: buffering encoded image data during compression.
// It is NOT safe for concurrent use.
type encodingBuffer struct {
	bytes.Buffer
}
