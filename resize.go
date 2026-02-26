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

	// Already fits.
	if srcW <= maxW && srcH <= maxH {
		return img
	}

	// Compute target dimensions preserving aspect ratio.
	ratio := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	dstW := int(math.Max(1, math.Round(float64(srcW)*ratio)))
	dstH := int(math.Max(1, math.Round(float64(srcH)*ratio)))

	return lanczosResize(img, dstW, dstH)
}

// lanczosResize performs high-quality Lanczos-3 interpolation.
// This is a two-pass separable filter: horizontal then vertical.
//
// Improvements over imaging library:
// - Pre-multiplied alpha handling prevents color fringing at transparency edges
// - Better weight normalization for edge pixels
// - Optimized memory access patterns for cache locality
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

	// Two-pass: horizontal then vertical.
	tmp := resizeH(img, dstW, srcH)
	return resizeV(tmp, dstW, dstH)
}

const lanczosA = 3.0 // Lanczos-3 kernel support

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

// resizeH performs horizontal Lanczos resize with pre-multiplied alpha.
func resizeH(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcW := src.Bounds().Dx()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	ratio := float64(srcW) / float64(dstW)
	support := lanczosA
	if ratio > 1 {
		support = lanczosA * ratio
	}

	// Precompute filter weights for each destination column.
	type weightEntry struct {
		index  int
		weight float64
	}
	weights := make([][]weightEntry, dstW)

	for dx := 0; dx < dstW; dx++ {
		center := (float64(dx)+0.5)*ratio - 0.5
		left := int(math.Ceil(center - support))
		right := int(math.Floor(center + support))

		if left < 0 {
			left = 0
		}
		if right >= srcW {
			right = srcW - 1
		}

		var wsum float64
		entries := make([]weightEntry, 0, right-left+1)
		for sx := left; sx <= right; sx++ {
			w := lanczosKernel((float64(sx) - center) / math.Max(ratio, 1.0))
			if w != 0 {
				wsum += w
				entries = append(entries, weightEntry{sx, w})
			}
		}
		// Normalize.
		if wsum != 0 {
			for i := range entries {
				entries[i].weight /= wsum
			}
		}
		weights[dx] = entries
	}

	// Process rows in parallel.
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
			if a != 0 {
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

// resizeV performs vertical Lanczos resize with pre-multiplied alpha.
func resizeV(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	srcH := src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	ratio := float64(srcH) / float64(dstH)
	support := lanczosA
	if ratio > 1 {
		support = lanczosA * ratio
	}

	// Precompute weights for each destination row.
	type weightEntry struct {
		index  int
		weight float64
	}
	weights := make([][]weightEntry, dstH)

	for dy := 0; dy < dstH; dy++ {
		center := (float64(dy)+0.5)*ratio - 0.5
		top := int(math.Ceil(center - support))
		bottom := int(math.Floor(center + support))

		if top < 0 {
			top = 0
		}
		if bottom >= srcH {
			bottom = srcH - 1
		}

		var wsum float64
		entries := make([]weightEntry, 0, bottom-top+1)
		for sy := top; sy <= bottom; sy++ {
			w := lanczosKernel((float64(sy) - center) / math.Max(ratio, 1.0))
			if w != 0 {
				wsum += w
				entries = append(entries, weightEntry{sy, w})
			}
		}
		if wsum != 0 {
			for i := range entries {
				entries[i].weight /= wsum
			}
		}
		weights[dy] = entries
	}

	// Process columns in parallel for better cache behavior.
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
			if a != 0 {
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
