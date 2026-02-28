package fennec

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// SSIM constants based on the original Wang et al. paper.
const (
	ssimK1 = 0.01
	ssimK2 = 0.03
	ssimL  = 255.0
	ssimC1 = (ssimK1 * ssimL) * (ssimK1 * ssimL)
	ssimC2 = (ssimK2 * ssimL) * (ssimK2 * ssimL)
)

// SSIM computes the Structural Similarity Index between two images.
// Returns a value between 0.0 (completely different) and 1.0 (identical).
//
// Uses a sliding window approach with a Gaussian-weighted kernel on the
// luminance channel (BT.601). Operates on read-only references to avoid copies.
func SSIM(img1, img2 image.Image) float64 {
	a := toNRGBARef(img1)
	b := toNRGBARef(img2)

	w := a.Bounds().Dx()
	h := a.Bounds().Dy()

	if w != b.Bounds().Dx() || h != b.Bounds().Dy() {
		b = lanczosResize(b, w, h)
	}

	if w < 8 || h < 8 {
		return pixelSSIM(a, b)
	}

	lumA := toLuminance(a)
	lumB := toLuminance(b)

	return windowedSSIM(lumA, lumB, w, h)
}

// SSIMFast computes a faster approximation of SSIM using downsampled images.
// Phase 2: increased max dimension from 256 to 512 for better artifact detection.
// 512px catches subtle blocking artifacts that 256px misses, while staying fast (~20ms).
func SSIMFast(img1, img2 *image.NRGBA) float64 {
	w := img1.Bounds().Dx()
	h := img1.Bounds().Dy()

	maxDim := 512 // Phase 2: was 256, increased for accuracy.
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / math.Max(float64(w), float64(h))
		newW := int(math.Max(8, math.Round(float64(w)*scale)))
		newH := int(math.Max(8, math.Round(float64(h)*scale)))
		img1 = boxDownsample(img1, newW, newH)
		img2 = boxDownsample(img2, newW, newH)
		w, h = newW, newH
	}

	if w < 8 || h < 8 {
		return pixelSSIM(img1, img2)
	}

	lumA := toLuminance(img1)
	lumB := toLuminance(img2)

	return windowedSSIM(lumA, lumB, w, h)
}

// windowedSSIM computes SSIM using an 8x8 sliding window with Gaussian weighting.
func windowedSSIM(lumA, lumB []float64, w, h int) float64 {
	const windowSize = 8
	half := windowSize / 2

	kernel := gaussianKernel(windowSize, 1.5)

	type ssimResult struct {
		sum   float64
		count int
	}

	procs := runtime.GOMAXPROCS(0)
	rows := h - windowSize + 1
	if procs > rows {
		procs = rows
	}
	if procs < 1 {
		procs = 1
	}

	results := make([]ssimResult, procs)
	rowsPerProc := (rows + procs - 1) / procs

	var wg sync.WaitGroup
	for p := 0; p < procs; p++ {
		wg.Add(1)
		go func(proc int) {
			defer wg.Done()
			startY := half + proc*rowsPerProc
			endY := startY + rowsPerProc
			if endY > h-half {
				endY = h - half
			}

			var localSum float64
			var localCount int

			for y := startY; y < endY; y++ {
				for x := half; x < w-half; x++ {
					var muA, muB float64
					var sigAA, sigBB, sigAB float64

					ki := 0
					for wy := -half; wy < half; wy++ {
						for wx := -half; wx < half; wx++ {
							idx := (y+wy)*w + (x + wx)
							weight := kernel[ki]
							va := lumA[idx]
							vb := lumB[idx]
							muA += va * weight
							muB += vb * weight
							ki++
						}
					}

					ki = 0
					for wy := -half; wy < half; wy++ {
						for wx := -half; wx < half; wx++ {
							idx := (y+wy)*w + (x + wx)
							weight := kernel[ki]
							da := lumA[idx] - muA
							db := lumB[idx] - muB
							sigAA += da * da * weight
							sigBB += db * db * weight
							sigAB += da * db * weight
							ki++
						}
					}

					num := (2*muA*muB + ssimC1) * (2*sigAB + ssimC2)
					den := (muA*muA + muB*muB + ssimC1) * (sigAA + sigBB + ssimC2)

					localSum += num / den
					localCount++
				}
			}

			results[proc] = ssimResult{localSum, localCount}
		}(p)
	}
	wg.Wait()

	var totalSum float64
	var totalCount int
	for _, r := range results {
		totalSum += r.sum
		totalCount += r.count
	}

	if totalCount == 0 {
		return 1.0
	}
	return totalSum / float64(totalCount)
}

// pixelSSIM computes a simple pixel-level SSIM for very small images.
func pixelSSIM(a, b *image.NRGBA) float64 {
	w := a.Bounds().Dx()
	h := a.Bounds().Dy()
	n := float64(w * h)
	if n == 0 {
		return 1.0
	}

	var muA, muB float64
	for i := 0; i < len(a.Pix); i += 4 {
		la := 0.299*float64(a.Pix[i]) + 0.587*float64(a.Pix[i+1]) + 0.114*float64(a.Pix[i+2])
		lb := 0.299*float64(b.Pix[i]) + 0.587*float64(b.Pix[i+1]) + 0.114*float64(b.Pix[i+2])
		muA += la
		muB += lb
	}
	muA /= n
	muB /= n

	var sigAA, sigBB, sigAB float64
	for i := 0; i < len(a.Pix); i += 4 {
		la := 0.299*float64(a.Pix[i]) + 0.587*float64(a.Pix[i+1]) + 0.114*float64(a.Pix[i+2])
		lb := 0.299*float64(b.Pix[i]) + 0.587*float64(b.Pix[i+1]) + 0.114*float64(b.Pix[i+2])
		da := la - muA
		db := lb - muB
		sigAA += da * da
		sigBB += db * db
		sigAB += da * db
	}
	sigAA /= n
	sigBB /= n
	sigAB /= n

	num := (2*muA*muB + ssimC1) * (2*sigAB + ssimC2)
	den := (muA*muA + muB*muB + ssimC1) * (sigAA + sigBB + ssimC2)
	return num / den
}

// toLuminance converts an NRGBA image to a float64 luminance array.
func toLuminance(img *image.NRGBA) []float64 {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	lum := make([]float64, w*h)

	for y := 0; y < h; y++ {
		off := y * img.Stride
		for x := 0; x < w; x++ {
			i := off + x*4
			lum[y*w+x] = 0.299*float64(img.Pix[i]) + 0.587*float64(img.Pix[i+1]) + 0.114*float64(img.Pix[i+2])
		}
	}
	return lum
}

// gaussianKernel creates a normalized 2D Gaussian kernel.
func gaussianKernel(size int, sigma float64) []float64 {
	kernel := make([]float64, size*size)
	half := size / 2
	var sum float64

	idx := 0
	for y := -half; y < half; y++ {
		for x := -half; x < half; x++ {
			val := math.Exp(-float64(x*x+y*y) / (2 * sigma * sigma))
			kernel[idx] = val
			sum += val
			idx++
		}
	}
	for i := range kernel {
		kernel[i] /= sum
	}
	return kernel
}

// boxDownsample performs fast box-filter downsampling.
func boxDownsample(img *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()

	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	for dy := 0; dy < dstH; dy++ {
		sy0 := int(float64(dy) * yRatio)
		sy1 := int(float64(dy+1) * yRatio)
		if sy1 > srcH {
			sy1 = srcH
		}
		if sy0 >= sy1 {
			sy0 = sy1 - 1
		}
		if sy0 < 0 {
			sy0 = 0
		}

		for dx := 0; dx < dstW; dx++ {
			sx0 := int(float64(dx) * xRatio)
			sx1 := int(float64(dx+1) * xRatio)
			if sx1 > srcW {
				sx1 = srcW
			}
			if sx0 >= sx1 {
				sx0 = sx1 - 1
			}
			if sx0 < 0 {
				sx0 = 0
			}

			var rSum, gSum, bSum, aSum float64
			var count float64

			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					off := sy*img.Stride + sx*4
					rSum += float64(img.Pix[off])
					gSum += float64(img.Pix[off+1])
					bSum += float64(img.Pix[off+2])
					aSum += float64(img.Pix[off+3])
					count++
				}
			}

			if count > 0 {
				inv := 1.0 / count
				off := dy*dst.Stride + dx*4
				dst.Pix[off] = clampF(rSum * inv)
				dst.Pix[off+1] = clampF(gSum * inv)
				dst.Pix[off+2] = clampF(bSum * inv)
				dst.Pix[off+3] = clampF(aSum * inv)
			}
		}
	}
	return dst
}

// MSSSIM computes Multi-Scale SSIM, which better correlates with
// human perception than single-scale SSIM.
func MSSSIM(img1, img2 image.Image) float64 {
	a := toNRGBARef(img1)
	b := toNRGBARef(img2)

	w := a.Bounds().Dx()
	h := a.Bounds().Dy()

	if w != b.Bounds().Dx() || h != b.Bounds().Dy() {
		b = lanczosResize(b, w, h)
	}

	weights := []float64{0.0448, 0.2856, 0.3001, 0.2363, 0.1333}
	levels := len(weights)

	for i := 0; i < levels-1; i++ {
		minDim := int(math.Min(float64(w), float64(h)))
		if minDim < 8 {
			weights = weights[:i+1]
			var sum float64
			for _, wt := range weights {
				sum += wt
			}
			for j := range weights {
				weights[j] /= sum
			}
			break
		}
		w /= 2
		h /= 2
	}

	// We need mutable copies for the multi-scale downsampling.
	aCopy := toNRGBA(a)
	bCopy := toNRGBA(b)

	var result float64
	for i, wt := range weights {
		ssim := SSIMFast(aCopy, bCopy)
		result += wt * math.Log(math.Max(ssim, 1e-10))

		if i < len(weights)-1 {
			nw := aCopy.Bounds().Dx() / 2
			nh := aCopy.Bounds().Dy() / 2
			if nw < 8 || nh < 8 {
				break
			}
			aCopy = boxDownsample(aCopy, nw, nh)
			bCopy = boxDownsample(bCopy, nw, nh)
		}
	}

	return math.Exp(result)
}
