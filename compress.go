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
// that still meets the target SSIM. Returns the quality, SSIM, cached encoded
// bytes (from the winning iteration), and any error.
//
// Traditional approach: pick a quality number and hope for the best.
// Fennec approach: measure actual perceptual quality and optimize precisely.
//
// The fourth return value is the cached JPEG bytes from the binary search.
// This avoids the double-encode bug where the final output would be re-encoded.
func compressJPEGOptimal(src *image.NRGBA, w io.Writer, targetSSIM float64, opts Options) (int, float64, []byte, error) {
	// Binary search bounds.
	lo, hi := 1, 100
	bestQuality := hi
	bestSSIM := 1.0
	var bestData []byte

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

	for lo <= hi {
		mid := (lo + hi) / 2

		// Encode at this quality.
		var buf bytes.Buffer
		if err := encodeJPEG(&buf, src, mid, opts.Subsample); err != nil {
			return 0, 0, nil, err
		}

		// Decode back to measure actual quality.
		decoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return 0, 0, nil, err
		}
		decodedNRGBA := toNRGBARef(decoded)

		// Compute SSIM between original and compressed.
		ssim := SSIMFast(src, decodedNRGBA)

		if ssim >= targetSSIM {
			// Quality is sufficient — cache this result and try lower quality.
			bestQuality = mid
			bestSSIM = ssim
			bestData = copyBytes(buf.Bytes())
			hi = mid - 1
		} else {
			// Quality too low — increase quality.
			lo = mid + 1
		}
	}

	// Write the cached best result directly instead of re-encoding.
	if bestData != nil {
		_, err := w.Write(bestData)
		return bestQuality, bestSSIM, bestData, err
	}

	// Fallback: encode at best quality found.
	if err := encodeJPEG(w, src, bestQuality, opts.Subsample); err != nil {
		return 0, 0, nil, err
	}
	return bestQuality, bestSSIM, nil, nil
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

			decoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
			if err != nil {
				return 0, 0, err
			}
			bestSSIM = SSIMFast(src, toNRGBARef(decoded))
		}

		if size > int64(targetSize) {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}

	return bestQuality, bestSSIM, encodeJPEG(w, src, bestQuality, true)
}

// compressPNG applies PNG-specific optimizations.
func compressPNG(img *image.NRGBA, w io.Writer, opts Options) error {
	// Check if we can reduce to a palette (indexed color).
	paletted := tryPalettize(img, 256)
	if paletted != nil {
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		return encoder.Encode(w, paletted)
	}

	// Check if image is grayscale — use Gray format for ~3× savings.
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

	colorMap := make(map[[4]uint8]int)

	for y := 0; y < h; y++ {
		off := y * img.Stride
		for x := 0; x < w; x++ {
			i := off + x*4
			key := [4]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
			colorMap[key]++
			if len(colorMap) > maxColors {
				return nil
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
