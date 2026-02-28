package fennec

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Open loads an image from a file path.
// If the file is a JPEG, the EXIF orientation is read (but not applied).
// Use OpenAndOrient to automatically correct orientation.
func Open(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("fennec: open %q: %w", filename, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("fennec: decode %q: %w", filename, err)
	}
	return img, nil
}

// OpenAndOrient loads an image and corrects its orientation using EXIF data.
// For JPEG files with orientation metadata, the returned image will be
// rotated/flipped so that it displays correctly regardless of camera orientation.
func OpenAndOrient(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("fennec: open %q: %w", filename, err)
	}
	defer f.Close()

	// Read EXIF orientation first.
	orient := ReadOrientation(f)

	// Seek back to start for image decode.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("fennec: seek %q: %w", filename, err)
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("fennec: decode %q: %w", filename, err)
	}

	if orient <= OrientNormal {
		return img, nil
	}

	// Apply orientation correction.
	nrgba := toNRGBA(img)
	return ApplyOrientation(nrgba, orient), nil
}

// openWithOrientation opens a file and returns the image, its EXIF orientation,
// and the file size. Used internally by CompressFile.
func openWithOrientation(filename string) (image.Image, Orientation, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, OrientNormal, 0, fmt.Errorf("fennec: open %q: %w", filename, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, OrientNormal, 0, fmt.Errorf("fennec: stat %q: %w", filename, err)
	}

	orient := ReadOrientation(f)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, OrientNormal, 0, fmt.Errorf("fennec: seek %q: %w", filename, err)
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, OrientNormal, 0, fmt.Errorf("fennec: decode %q: %w", filename, err)
	}

	return img, orient, stat.Size(), nil
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
	default:
		return fmt.Errorf("fennec: unsupported extension %q (use .jpg or .png)", ext)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("fennec: create %q: %w", filename, err)
	}
	defer f.Close()

	return Encode(f, img, format, opts)
}

// Encode writes the image to w in the specified format with Fennec optimization.
func Encode(w io.Writer, img image.Image, format Format, opts Options) error {
	src := toNRGBARef(img)

	switch format {
	case JPEG:
		targetSSIM := opts.Quality.targetSSIM()
		if opts.TargetSSIM > 0 {
			targetSSIM = opts.TargetSSIM
		}
		_, _, _, err := compressJPEGOptimal(src, w, targetSSIM, opts)
		return err
	case PNG:
		return compressPNG(src, w, opts)
	default:
		return fmt.Errorf("fennec: unsupported format for Encode (use JPEG or PNG)")
	}
}

// encodeToBytes encodes an image to bytes in the specified format.
// Used internally when CompressedData is missing.
func encodeToBytes(img *image.NRGBA, format Format, quality int) ([]byte, error) {
	var buf safeBuffer
	switch format {
	case JPEG:
		if err := encodeJPEG(&buf, img, quality, false); err != nil {
			return nil, fmt.Errorf("fennec: JPEG encode: %w", err)
		}
	case PNG:
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("fennec: PNG encode: %w", err)
		}
	default:
		return nil, fmt.Errorf("fennec: unsupported format")
	}
	return buf.Bytes(), nil
}

// encodeJPEG handles JPEG encoding, using RGBA for opaque images (faster path).
func encodeJPEG(w io.Writer, img *image.NRGBA, quality int, subsample bool) error {
	if isOpaque(img) {
		rgba := &image.RGBA{
			Pix:    img.Pix,
			Stride: img.Stride,
			Rect:   img.Rect,
		}
		return jpeg.Encode(w, rgba, &jpeg.Options{Quality: quality})
	}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}
