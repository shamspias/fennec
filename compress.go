package fennec

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
)

// compressJPEGOptimal uses binary search to find the lowest JPEG quality
// that still meets the target SSIM. This is the core innovation of Fennec.
//
// Traditional approach: pick a quality number and hope for the best.
// Fennec approach: measure actual perceptual quality and optimize precisely.
func compressJPEGOptimal(src *image.NRGBA, w io.Writer, targetSSIM float64, opts Options) (int, float64, error) {
	// Binary search bounds.
	lo, hi := 1, 100
	bestQuality := hi
	bestSSIM := 1.0

	// Fast path: if target is very high, start from higher quality.
	if targetSSIM >= 0.99 {
		lo = 75
	} else if targetSSIM >= 0.97 {
		lo = 50
	} else if targetSSIM >= 0.94 {
		lo = 30
	} else if targetSSIM >= 0.90 {
		lo = 15
	}

	// Prepare source for SSIM comparison.
	// For large images, use a downsampled version for faster SSIM.
	for lo <= hi {
		mid := (lo + hi) / 2

		// Encode at this quality.
		var buf bytes.Buffer
		if err := encodeJPEG(&buf, src, mid, opts.Subsample); err != nil {
			return 0, 0, err
		}

		// Decode back to measure actual quality.
		decoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return 0, 0, err
		}
		decodedNRGBA := toNRGBA(decoded)

		// Compute SSIM between original and compressed.
		ssim := SSIMFast(src, decodedNRGBA)

		if ssim >= targetSSIM {
			// Quality is sufficient — try lower quality to save more space.
			bestQuality = mid
			bestSSIM = ssim
			hi = mid - 1
		} else {
			// Quality too low — increase quality.
			lo = mid + 1
		}
	}

	// Encode final result with the optimal quality.
	return bestQuality, bestSSIM, encodeJPEG(w, src, bestQuality, opts.Subsample)
}

// compressJPEGToSize finds the JPEG quality that produces output closest to targetSize.
func compressJPEGToSize(src *image.NRGBA, w io.Writer, targetSize int) (int, float64, error) {
	lo, hi := 1, 100
	bestQuality := 50
	bestDiff := int64(1<<63 - 1)
	bestSSIM := 0.0

	for lo <= hi {
		mid := (lo + hi) / 2

		var buf bytes.Buffer
		if err := encodeJPEG(&buf, src, mid, true); err != nil {
			return 0, 0, err
		}

		size := int64(buf.Len())
		diff := abs64(size - int64(targetSize))

		if diff < bestDiff {
			bestDiff = diff
			bestQuality = mid

			// Compute SSIM for the best candidate.
			decoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
			if err != nil {
				return 0, 0, err
			}
			bestSSIM = SSIMFast(src, toNRGBA(decoded))
		}

		if size > int64(targetSize) {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}

	return bestQuality, bestSSIM, encodeJPEG(w, src, bestQuality, true)
}

// encodeJPEG handles JPEG encoding with optional optimizations.
func encodeJPEG(w io.Writer, img *image.NRGBA, quality int, subsample bool) error {
	// Go's standard jpeg encoder doesn't expose chroma subsampling control,
	// but converting NRGBA to RGBA for opaque images avoids an extra copy.
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

// compressPNG applies PNG-specific optimizations.
func compressPNG(img *image.NRGBA, w io.Writer, opts Options) error {
	// Check if we can reduce to a palette (indexed color).
	// PNG with palette is dramatically smaller for images with few colors.
	paletted := tryPalettize(img, 256)
	if paletted != nil {
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		return encoder.Encode(w, paletted)
	}

	// Check if image is grayscale — use Gray format for 3x savings.
	if isGrayscale(img) {
		gray := toGray(img)
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		return encoder.Encode(w, gray)
	}

	// Full NRGBA with best compression.
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	return encoder.Encode(w, img)
}

// tryPalettize attempts to convert the image to an indexed palette.
// Returns nil if the image has too many colors.
func tryPalettize(img *image.NRGBA, maxColors int) *image.Paletted {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	// Collect unique colors.
	type colorCount struct {
		c     [4]uint8
		count int
	}
	colorMap := make(map[[4]uint8]int)

	for y := 0; y < h; y++ {
		off := y * img.Stride
		for x := 0; x < w; x++ {
			i := off + x*4
			key := [4]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
			colorMap[key]++
			if len(colorMap) > maxColors {
				return nil // Too many colors.
			}
		}
	}

	// Build palette.
	palette := make([]color.Color, 0, len(colorMap))
	colorIndex := make(map[[4]uint8]uint8, len(colorMap))

	for c := range colorMap {
		idx := uint8(len(palette))
		colorIndex[c] = idx
		palette = append(palette, color.NRGBA{c[0], c[1], c[2], c[3]})
	}

	// Create paletted image.
	paletted := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	for y := 0; y < h; y++ {
		srcOff := y * img.Stride
		dstOff := y * paletted.Stride
		for x := 0; x < w; x++ {
			i := srcOff + x*4
			key := [4]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
			paletted.Pix[dstOff+x] = colorIndex[key]
		}
	}

	return paletted
}

// nrgbaColor implements color.Color for palette construction.
type nrgbaColor struct {
	R, G, B, A uint8
}

func (c nrgbaColor) RGBA() (r, g, b, a uint32) {
	a = uint32(c.A) * 0x101
	r = uint32(c.R) * 0x101 * a / 0xffff
	g = uint32(c.G) * 0x101 * a / 0xffff
	b = uint32(c.B) * 0x101 * a / 0xffff
	return
}

// isOpaque checks if all pixels have full alpha.
func isOpaque(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}

// isGrayscale checks if all pixels have R == G == B.
func isGrayscale(img *image.NRGBA) bool {
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != img.Pix[i+1] || img.Pix[i+1] != img.Pix[i+2] {
			return false
		}
	}
	return true
}

// toGray converts to grayscale image (1 byte per pixel instead of 4).
func toGray(img *image.NRGBA) *image.Gray {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	gray := image.NewGray(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		srcOff := y * img.Stride
		dstOff := y * gray.Stride
		for x := 0; x < w; x++ {
			gray.Pix[dstOff+x] = img.Pix[srcOff+x*4]
		}
	}
	return gray
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
