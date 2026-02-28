package fennec

import (
	"image"
	"math"
)

// Sharpen applies adaptive unsharp mask sharpening.
// The strength parameter should be 0.0–1.0 (0.3 is a good default).
func Sharpen(img *image.NRGBA, strength float64) *image.NRGBA {
	if strength <= 0 {
		return img
	}
	if strength > 1 {
		strength = 1
	}

	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	if w < 3 || h < 3 {
		return img
	}

	blurred := gaussianBlur3x3(img)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	amount := 1.0 + strength*1.5

	parallelDo(0, h, func(y int) {
		for x := 0; x < w; x++ {
			srcOff := y*img.Stride + x*4
			blurOff := y*blurred.Stride + x*4
			dstOff := y*dst.Stride + x*4

			for c := 0; c < 3; c++ {
				orig := float64(img.Pix[srcOff+c])
				blur := float64(blurred.Pix[blurOff+c])
				val := orig + amount*(orig-blur)
				dst.Pix[dstOff+c] = clampF(val)
			}
			dst.Pix[dstOff+3] = img.Pix[srcOff+3]
		}
	})

	return dst
}

// AdaptiveSharpen applies sharpening only to edge regions, leaving smooth
// areas untouched to prevent noise amplification.
func AdaptiveSharpen(img *image.NRGBA, strength float64) *image.NRGBA {
	if strength <= 0 {
		return img
	}
	if strength > 1 {
		strength = 1
	}

	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	if w < 3 || h < 3 {
		return img
	}

	blurred := gaussianBlur3x3(img)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	amount := 1.0 + strength*2.0

	// Copy the entire source first (handles borders).
	copy(dst.Pix, img.Pix)

	parallelDo(1, h-1, func(y int) {
		for x := 1; x < w-1; x++ {
			srcOff := y*img.Stride + x*4
			edgeStr := localEdgeStrength(img, x, y)
			localAmount := amount * edgeStr

			blurOff := y*blurred.Stride + x*4
			dstOff := y*dst.Stride + x*4

			for c := 0; c < 3; c++ {
				orig := float64(img.Pix[srcOff+c])
				blur := float64(blurred.Pix[blurOff+c])
				val := orig + localAmount*(orig-blur)
				dst.Pix[dstOff+c] = clampF(val)
			}
			dst.Pix[dstOff+3] = img.Pix[srcOff+3]
		}
	})

	return dst
}

// localEdgeStrength computes edge strength at a pixel using Sobel gradients.
func localEdgeStrength(img *image.NRGBA, x, y int) float64 {
	getLum := func(px, py int) float64 {
		off := py*img.Stride + px*4
		return 0.299*float64(img.Pix[off]) + 0.587*float64(img.Pix[off+1]) + 0.114*float64(img.Pix[off+2])
	}

	gx := -getLum(x-1, y-1) + getLum(x+1, y-1) -
		2*getLum(x-1, y) + 2*getLum(x+1, y) -
		getLum(x-1, y+1) + getLum(x+1, y+1)

	gy := -getLum(x-1, y-1) - 2*getLum(x, y-1) - getLum(x+1, y-1) +
		getLum(x-1, y+1) + 2*getLum(x, y+1) + getLum(x+1, y+1)

	mag := math.Sqrt(gx*gx + gy*gy)
	normalized := mag / 400.0
	if normalized > 1 {
		normalized = 1
	}
	return normalized
}

// gaussianBlur3x3 applies a fast 3x3 Gaussian blur.
func gaussianBlur3x3(img *image.NRGBA) *image.NRGBA {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(dst.Pix, img.Pix)

	parallelDo(1, h-1, func(y int) {
		for x := 1; x < w-1; x++ {
			for c := 0; c < 4; c++ {
				var sum float64
				sum += float64(img.Pix[(y-1)*img.Stride+(x-1)*4+c]) * 1
				sum += float64(img.Pix[(y-1)*img.Stride+(x)*4+c]) * 2
				sum += float64(img.Pix[(y-1)*img.Stride+(x+1)*4+c]) * 1
				sum += float64(img.Pix[(y)*img.Stride+(x-1)*4+c]) * 2
				sum += float64(img.Pix[(y)*img.Stride+(x)*4+c]) * 4
				sum += float64(img.Pix[(y)*img.Stride+(x+1)*4+c]) * 2
				sum += float64(img.Pix[(y+1)*img.Stride+(x-1)*4+c]) * 1
				sum += float64(img.Pix[(y+1)*img.Stride+(x)*4+c]) * 2
				sum += float64(img.Pix[(y+1)*img.Stride+(x+1)*4+c]) * 1
				dst.Pix[y*dst.Stride+x*4+c] = clampF(sum / 16.0)
			}
		}
	})
	return dst
}

// GaussianBlur applies Gaussian blur with the specified sigma.
// Uses separable convolution for O(n*r) instead of O(n*r²) complexity.
func GaussianBlur(img *image.NRGBA, sigma float64) *image.NRGBA {
	if sigma <= 0 {
		return img
	}

	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	radius := int(math.Ceil(sigma * 3))

	kernelSize := radius*2 + 1
	kernel := make([]float64, kernelSize)
	var sum float64
	for i := 0; i < kernelSize; i++ {
		x := float64(i - radius)
		kernel[i] = math.Exp(-(x * x) / (2 * sigma * sigma))
		sum += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	// Horizontal pass.
	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
	parallelDo(0, h, func(y int) {
		for x := 0; x < w; x++ {
			var r, g, b, a float64
			for k := 0; k < kernelSize; k++ {
				sx := x + k - radius
				if sx < 0 {
					sx = 0
				} else if sx >= w {
					sx = w - 1
				}
				off := y*img.Stride + sx*4
				wt := kernel[k]
				r += float64(img.Pix[off]) * wt
				g += float64(img.Pix[off+1]) * wt
				b += float64(img.Pix[off+2]) * wt
				a += float64(img.Pix[off+3]) * wt
			}
			off := y*tmp.Stride + x*4
			tmp.Pix[off] = clampF(r)
			tmp.Pix[off+1] = clampF(g)
			tmp.Pix[off+2] = clampF(b)
			tmp.Pix[off+3] = clampF(a)
		}
	})

	// Vertical pass.
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	parallelDo(0, w, func(x int) {
		for y := 0; y < h; y++ {
			var r, g, b, a float64
			for k := 0; k < kernelSize; k++ {
				sy := y + k - radius
				if sy < 0 {
					sy = 0
				} else if sy >= h {
					sy = h - 1
				}
				off := sy*tmp.Stride + x*4
				wt := kernel[k]
				r += float64(tmp.Pix[off]) * wt
				g += float64(tmp.Pix[off+1]) * wt
				b += float64(tmp.Pix[off+2]) * wt
				a += float64(tmp.Pix[off+3]) * wt
			}
			off := y*dst.Stride + x*4
			dst.Pix[off] = clampF(r)
			dst.Pix[off+1] = clampF(g)
			dst.Pix[off+2] = clampF(b)
			dst.Pix[off+3] = clampF(a)
		}
	})

	return dst
}
