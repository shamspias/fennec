package fennec

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// smartResize resizes the image to fit within maxW x maxH while preserving
// aspect ratio. Uses Lanczos-3 interpolation for superior quality.
func smartResize(img *image.NRGBA, maxW, maxH int) *image.NRGBA {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()

	if maxW <= 0 {
		maxW = srcW
	}
	if maxH <= 0 {
		maxH = srcH
	}

	if srcW <= maxW && srcH <= maxH {
		return img
	}

	ratio := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	dstW := int(math.Max(1, math.Round(float64(srcW)*ratio)))
	dstH := int(math.Max(1, math.Round(float64(srcH)*ratio)))

	return lanczosResize(img, dstW, dstH)
}

// lanczosResize performs high-quality Lanczos-3 interpolation.
// Two-pass separable filter: horizontal then vertical.
// Uses pre-multiplied alpha to prevent color fringing at transparency edges.
func lanczosResize(img *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()

	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}

	if srcW == dstW && srcH == dstH {
		dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
		copy(dst.Pix, img.Pix)
		return dst
	}

	tmp := resizeH(img, dstW, srcH)
	return resizeV(tmp, dstW, dstH)
}

const lanczosA = 3.0

func lanczosKernel(x float64) float64 {
	if x == 0 {
		return 1.0
	}
	if x < 0 {
		x = -x
	}
	if x >= lanczosA {
		return 0.0
	}
	xpi := x * math.Pi
	return (lanczosA * math.Sin(xpi) * math.Sin(xpi/lanczosA)) / (xpi * xpi)
}

type weightEntry struct {
	index  int
	weight float64
}

// resizeH performs horizontal Lanczos resize with pre-multiplied alpha.
func resizeH(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcW := src.Bounds().Dx()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	ratio := float64(srcW) / float64(dstW)
	support := lanczosA
	if ratio > 1 {
		support = lanczosA * ratio
	}

	weights := precomputeWeights(dstW, srcW, ratio, support)

	parallelDo(0, dstH, func(y int) {
		for dx := 0; dx < dstW; dx++ {
			var r, g, b, a float64

			for _, we := range weights[dx] {
				off := y*src.Stride + we.index*4
				sa := float64(src.Pix[off+3])
				w := we.weight

				// Pre-multiply alpha for correct interpolation.
				aw := sa * w
				r += float64(src.Pix[off]) * aw
				g += float64(src.Pix[off+1]) * aw
				b += float64(src.Pix[off+2]) * aw
				a += aw
			}

			dstOff := y*dst.Stride + dx*4
			// Phase 1 fix: guard against zero alpha from floating-point rounding.
			if a > 0.5 {
				inv := 1.0 / a
				dst.Pix[dstOff] = clampF(r * inv)
				dst.Pix[dstOff+1] = clampF(g * inv)
				dst.Pix[dstOff+2] = clampF(b * inv)
				dst.Pix[dstOff+3] = clampF(a)
			}
			// else: leave as zero (transparent black) — correct for truly transparent regions.
		}
	})

	return dst
}

// resizeV performs vertical Lanczos resize with pre-multiplied alpha.
func resizeV(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcH := src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	ratio := float64(srcH) / float64(dstH)
	support := lanczosA
	if ratio > 1 {
		support = lanczosA * ratio
	}

	weights := precomputeWeights(dstH, srcH, ratio, support)

	parallelDo(0, dstW, func(x int) {
		for dy := 0; dy < dstH; dy++ {
			var r, g, b, a float64

			for _, we := range weights[dy] {
				off := we.index*src.Stride + x*4
				sa := float64(src.Pix[off+3])
				w := we.weight

				aw := sa * w
				r += float64(src.Pix[off]) * aw
				g += float64(src.Pix[off+1]) * aw
				b += float64(src.Pix[off+2]) * aw
				a += aw
			}

			dstOff := dy*dst.Stride + x*4
			if a > 0.5 {
				inv := 1.0 / a
				dst.Pix[dstOff] = clampF(r * inv)
				dst.Pix[dstOff+1] = clampF(g * inv)
				dst.Pix[dstOff+2] = clampF(b * inv)
				dst.Pix[dstOff+3] = clampF(a)
			}
		}
	})

	return dst
}

// precomputeWeights builds filter weight tables for a single dimension.
func precomputeWeights(dstSize, srcSize int, ratio, support float64) [][]weightEntry {
	weights := make([][]weightEntry, dstSize)
	filterScale := math.Max(ratio, 1.0)

	for d := 0; d < dstSize; d++ {
		center := (float64(d)+0.5)*ratio - 0.5
		left := int(math.Ceil(center - support))
		right := int(math.Floor(center + support))

		if left < 0 {
			left = 0
		}
		if right >= srcSize {
			right = srcSize - 1
		}

		var wsum float64
		entries := make([]weightEntry, 0, right-left+1)
		for s := left; s <= right; s++ {
			w := lanczosKernel((float64(s) - center) / filterScale)
			if w != 0 {
				wsum += w
				entries = append(entries, weightEntry{s, w})
			}
		}
		if wsum != 0 {
			for i := range entries {
				entries[i].weight /= wsum
			}
		}
		weights[d] = entries
	}
	return weights
}

// parallelDo executes fn(i) for i in [start, stop) across multiple goroutines.
func parallelDo(start, stop int, fn func(i int)) {
	count := stop - start
	if count <= 0 {
		return
	}

	procs := runtime.GOMAXPROCS(0)
	if procs > count {
		procs = count
	}
	if procs <= 1 {
		for i := start; i < stop; i++ {
			fn(i)
		}
		return
	}

	var wg sync.WaitGroup
	batchSize := (count + procs - 1) / procs

	for p := 0; p < procs; p++ {
		batchStart := start + p*batchSize
		batchEnd := batchStart + batchSize
		if batchEnd > stop {
			batchEnd = stop
		}
		if batchStart >= batchEnd {
			continue
		}

		wg.Add(1)
		go func(from, to int) {
			defer wg.Done()
			for i := from; i < to; i++ {
				fn(i)
			}
		}(batchStart, batchEnd)
	}
	wg.Wait()
}
