package fennec

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// toNRGBA converts any image.Image to *image.NRGBA, always returning a new copy.
// Use this when the caller intends to mutate the result (resize, compress, etc.).
func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		bounds := nrgba.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		copy(dst.Pix, nrgba.Pix)
		return dst
	}
	return convertToNRGBA(img)
}

// toNRGBARef converts any image.Image to *image.NRGBA without copying if
// the input is already NRGBA. Use this for read-only paths (SSIM, Analyze)
// where no mutation occurs. The caller must NOT modify the returned image.
func toNRGBARef(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	return convertToNRGBA(img)
}

// convertToNRGBA does the actual pixel-by-pixel conversion from any image
// format to NRGBA. Handles pre-multiplied alpha correctly.
func convertToNRGBA(img image.Image) *image.NRGBA {
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			off := (y-bounds.Min.Y)*dst.Stride + (x-bounds.Min.X)*4
			if a == 0 {
				// Fully transparent — zero everything.
				dst.Pix[off] = 0
				dst.Pix[off+1] = 0
				dst.Pix[off+2] = 0
				dst.Pix[off+3] = 0
			} else if a == 0xffff {
				// Fully opaque — simple shift.
				dst.Pix[off] = uint8(r >> 8)
				dst.Pix[off+1] = uint8(g >> 8)
				dst.Pix[off+2] = uint8(b >> 8)
				dst.Pix[off+3] = 0xff
			} else {
				// Semi-transparent — un-premultiply alpha.
				dst.Pix[off] = uint8(((r * 0xffff) / a) >> 8)
				dst.Pix[off+1] = uint8(((g * 0xffff) / a) >> 8)
				dst.Pix[off+2] = uint8(((b * 0xffff) / a) >> 8)
				dst.Pix[off+3] = uint8(a >> 8)
			}
		}
	}
	return dst
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

// analyzeFormat examines the image to determine the best output format.
// Images with transparency or very few colors → PNG.
// Photographic images with many colors → JPEG.
func analyzeFormat(img *image.NRGBA) Format {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

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

	if hasAlpha {
		return PNG
	}
	if len(colorSet) < 256 {
		return PNG
	}
	return JPEG
}

// clampF clamps a float64 to uint8 range [0, 255].
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

// humanBytes formats a byte count for human reading.
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

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// rotateNRGBA90CW rotates an NRGBA image 90° clockwise.
func rotateNRGBA90CW(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := y*img.Stride + x*4
			dstOff := x*dst.Stride + (h-1-y)*4
			copy(dst.Pix[dstOff:dstOff+4], img.Pix[srcOff:srcOff+4])
		}
	}
	return dst
}

// rotateNRGBA180 rotates an NRGBA image 180°.
func rotateNRGBA180(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := y*img.Stride + x*4
			dstOff := (h-1-y)*dst.Stride + (w-1-x)*4
			copy(dst.Pix[dstOff:dstOff+4], img.Pix[srcOff:srcOff+4])
		}
	}
	return dst
}

// rotateNRGBA270CW rotates an NRGBA image 270° clockwise (90° counter-clockwise).
func rotateNRGBA270CW(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := y*img.Stride + x*4
			dstOff := (w-1-x)*dst.Stride + y*4
			copy(dst.Pix[dstOff:dstOff+4], img.Pix[srcOff:srcOff+4])
		}
	}
	return dst
}

// flipNRGBAHorizontal mirrors an NRGBA image horizontally.
func flipNRGBAHorizontal(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := y*img.Stride + x*4
			dstOff := y*dst.Stride + (w-1-x)*4
			copy(dst.Pix[dstOff:dstOff+4], img.Pix[srcOff:srcOff+4])
		}
	}
	return dst
}

// flipNRGBAVertical mirrors an NRGBA image vertically.
func flipNRGBAVertical(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcRow := y * img.Stride
		dstRow := (h - 1 - y) * dst.Stride
		copy(dst.Pix[dstRow:dstRow+w*4], img.Pix[srcRow:srcRow+w*4])
	}
	return dst
}
