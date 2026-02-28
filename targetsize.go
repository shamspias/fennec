package fennec

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

const minJPEGQuality = 20

type sizeResult struct {
	data    []byte
	format  Format
	quality int
	ssim    float64
	finalW  int
	finalH  int
	img     *image.NRGBA
}

func hitTargetSize(ctx context.Context, original *image.NRGBA, targetBytes int, opts Options) (*sizeResult, error) {
	w := original.Bounds().Dx()
	h := original.Bounds().Dy()

	wantPNG := opts.Format == PNG
	wantJPEG := opts.Format == JPEG
	canUseJPEG := !wantPNG && isOpaque(original)

	var candidates []*sizeResult

	// Check context before each strategy.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Strategy 1: JPEG quality binary search.
	if canUseJPEG || wantJPEG {
		if r, err := jpegQualitySearch(original, targetBytes); err == nil && r != nil {
			if r.quality >= minJPEGQuality {
				candidates = append(candidates, r)
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Strategy 2: Color quantization → indexed PNG.
	if !wantJPEG {
		if r, err := quantizeStrategy(original, targetBytes); err == nil && r != nil {
			candidates = append(candidates, r)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Strategy 3: JPEG quality + scale.
	if canUseJPEG || wantJPEG {
		if r, err := jpegQualityScaleSearch(ctx, original, targetBytes); err == nil && r != nil {
			candidates = append(candidates, r)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Strategy 4: Scale search (last resort).
	if len(candidates) == 0 {
		format := opts.Format
		if format == Auto {
			if canUseJPEG {
				format = JPEG
			} else {
				format = PNG
			}
		}
		if r, err := scaleSearch(ctx, original, targetBytes, format); err == nil && r != nil {
			candidates = append(candidates, r)
		}
	}

	if len(candidates) == 0 {
		var buf bytes.Buffer
		if canUseJPEG || wantJPEG {
			encodeJPEG(&buf, original, 1, false)
			return &sizeResult{
				data: buf.Bytes(), format: JPEG, quality: 1,
				ssim: computeSSIMNRGBA(original, original), finalW: w, finalH: h, img: original,
			}, nil
		}
		compressPNG(original, &buf, opts)
		return &sizeResult{
			data: buf.Bytes(), format: PNG,
			ssim: 1.0, finalW: w, finalH: h, img: original,
		}, nil
	}

	var best *sizeResult
	for _, c := range candidates {
		if best == nil || betterFit(c, best, targetBytes) {
			best = c
		}
	}

	return best, nil
}

func betterFit(candidate, current *sizeResult, target int) bool {
	cSize := int64(len(candidate.data))
	bSize := int64(len(current.data))
	t := int64(target)

	cUnder := cSize <= t
	bUnder := bSize <= t

	if cUnder && !bUnder {
		return true
	}
	if !cUnder && bUnder {
		return false
	}
	if cUnder && bUnder {
		if candidate.ssim != current.ssim {
			return candidate.ssim > current.ssim
		}
		return candidate.quality > current.quality
	}
	return cSize < bSize
}

// ── Strategy 1 ──────────────────────────────────────────────────────────────

func jpegQualitySearch(src *image.NRGBA, targetBytes int) (*sizeResult, error) {
	return jpegQualitySearchOpt(src, targetBytes, false)
}

func jpegQualitySearchFast(src *image.NRGBA, targetBytes int) (*sizeResult, error) {
	return jpegQualitySearchOpt(src, targetBytes, true)
}

func jpegQualitySearchOpt(src *image.NRGBA, targetBytes int, skipSSIM bool) (*sizeResult, error) {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()
	pixels := w * h

	targetBPP := float64(targetBytes*8) / float64(pixels)
	lo, hi := 1, 100
	if targetBPP < 0.5 {
		hi = 40
	} else if targetBPP < 1.0 {
		lo, hi = 10, 70
	} else if targetBPP < 2.0 {
		lo, hi = 30, 90
	} else if targetBPP > 4.0 {
		lo = 60
	}

	var bestBuf []byte
	bestQ := 0
	bestSSIM := 0.0

	for lo <= hi {
		mid := (lo + hi) / 2
		var buf bytes.Buffer
		if err := encodeJPEG(&buf, src, mid, false); err != nil {
			return nil, err
		}

		if int64(buf.Len()) <= int64(targetBytes) {
			bestBuf = copyBytes(buf.Bytes())
			bestQ = mid
			if !skipSSIM {
				decoded := decodeJPEGFromBytes(bestBuf)
				if decoded != nil {
					bestSSIM = computeSSIMNRGBA(src, decoded)
				}
			}
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if bestBuf == nil {
		return nil, nil
	}

	return &sizeResult{
		data: bestBuf, format: JPEG, quality: bestQ,
		ssim: bestSSIM, finalW: w, finalH: h, img: src,
	}, nil
}

// ── Strategy 2 ──────────────────────────────────────────────────────────────

func quantizeStrategy(src *image.NRGBA, targetBytes int) (*sizeResult, error) {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()

	for _, maxColors := range []int{256, 128, 64, 32, 16} {
		palette := medianCut(src, maxColors)
		indexed := applyPalette(src, palette)

		var buf bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&buf, indexed); err != nil {
			continue
		}

		if int64(buf.Len()) <= int64(targetBytes) {
			quantizedNRGBA := palettedToNRGBA(indexed)
			ssim := computeSSIMNRGBA(src, quantizedNRGBA)

			return &sizeResult{
				data: buf.Bytes(), format: PNG, quality: 0,
				ssim: ssim, finalW: w, finalH: h, img: quantizedNRGBA,
			}, nil
		}
	}

	return nil, nil
}

// ── Strategy 3 ──────────────────────────────────────────────────────────────

func jpegQualityScaleSearch(ctx context.Context, src *image.NRGBA, targetBytes int) (*sizeResult, error) {
	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()

	type candidate struct {
		scale   float64
		quality int
		size    int
	}
	var bestCand *candidate

	loScale, hiScale := 0.05, 1.0

	for i := 0; i < 10; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		midScale := (loScale + hiScale) / 2
		newW := int(float64(origW) * midScale)
		newH := int(float64(origH) * midScale)
		if newW < 8 || newH < 8 {
			loScale = midScale
			continue
		}

		scaled := boxDownsample(src, newW, newH)
		r, err := jpegQualitySearchFast(scaled, targetBytes)
		if err != nil || r == nil {
			hiScale = midScale
			continue
		}

		if int64(len(r.data)) <= int64(targetBytes) && r.quality >= minJPEGQuality {
			bestCand = &candidate{scale: midScale, quality: r.quality, size: len(r.data)}
			loScale = midScale
		} else {
			hiScale = midScale
		}
	}

	for _, scale := range []float64{0.75, 0.50, 0.375, 0.25} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		newW := int(float64(origW) * scale)
		newH := int(float64(origH) * scale)
		if newW < 8 || newH < 8 {
			continue
		}
		scaled := boxDownsample(src, newW, newH)
		r, err := jpegQualitySearchFast(scaled, targetBytes)
		if err != nil || r == nil || int64(len(r.data)) > int64(targetBytes) {
			continue
		}
		if r.quality < minJPEGQuality {
			continue
		}
		if bestCand == nil || scale > bestCand.scale {
			bestCand = &candidate{scale: scale, quality: r.quality, size: len(r.data)}
		}
	}

	if bestCand == nil {
		return nil, nil
	}

	finalW := int(float64(origW) * bestCand.scale)
	finalH := int(float64(origH) * bestCand.scale)
	finalScaled := lanczosResize(src, finalW, finalH)

	r, err := jpegQualitySearch(finalScaled, targetBytes)
	if err != nil || r == nil {
		return nil, nil
	}
	if r.quality < minJPEGQuality {
		return nil, nil
	}

	r.ssim = computeSSIMNRGBA(src, finalScaled)
	r.finalW = finalW
	r.finalH = finalH
	r.img = finalScaled

	return r, nil
}

// ── Strategy 4 ──────────────────────────────────────────────────────────────

func scaleSearch(ctx context.Context, src *image.NRGBA, targetBytes int, format Format) (*sizeResult, error) {
	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()

	lo, hi := 0.05, 1.0
	bestScale := 0.0
	bestQ := 0

	for i := 0; i < 12; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		mid := (lo + hi) / 2
		newW := int(float64(origW) * mid)
		newH := int(float64(origH) * mid)
		if newW < 1 || newH < 1 {
			lo = mid
			continue
		}

		scaled := boxDownsample(src, newW, newH)

		var fits bool
		var q int
		switch format {
		case JPEG:
			r, err := jpegQualitySearchFast(scaled, targetBytes)
			if err == nil && r != nil && int64(len(r.data)) <= int64(targetBytes) && r.quality >= minJPEGQuality {
				fits = true
				q = r.quality
			}
		case PNG:
			var buf bytes.Buffer
			encoder := png.Encoder{CompressionLevel: png.BestCompression}
			encoder.Encode(&buf, scaled)
			fits = int64(buf.Len()) <= int64(targetBytes)
		}

		if fits {
			bestScale = mid
			bestQ = q
			lo = mid
		} else {
			hi = mid
		}
	}

	if bestScale == 0 {
		return nil, nil
	}

	finalW := int(float64(origW) * bestScale)
	finalH := int(float64(origH) * bestScale)
	scaled := lanczosResize(src, finalW, finalH)

	var buf bytes.Buffer
	switch format {
	case JPEG:
		r, err := jpegQualitySearchFast(scaled, targetBytes)
		if err != nil || r == nil {
			encodeJPEG(&buf, scaled, bestQ, false)
		} else {
			return &sizeResult{
				data: r.data, format: JPEG, quality: r.quality,
				ssim:   computeSSIMNRGBA(src, scaled),
				finalW: finalW, finalH: finalH, img: scaled,
			}, nil
		}
	case PNG:
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		encoder.Encode(&buf, scaled)
	}

	return &sizeResult{
		data: buf.Bytes(), format: format, quality: bestQ,
		ssim:   computeSSIMNRGBA(src, scaled),
		finalW: finalW, finalH: finalH, img: scaled,
	}, nil
}

// ── Median-Cut Color Quantizer ──────────────────────────────────────────────

type colorBox struct {
	pixels     [][3]uint8
	rMin, rMax uint8
	gMin, gMax uint8
	bMin, bMax uint8
}

func newColorBox(pixels [][3]uint8) *colorBox {
	box := &colorBox{
		pixels: pixels,
		rMin:   255, gMin: 255, bMin: 255,
	}
	for _, p := range pixels {
		if p[0] < box.rMin {
			box.rMin = p[0]
		}
		if p[0] > box.rMax {
			box.rMax = p[0]
		}
		if p[1] < box.gMin {
			box.gMin = p[1]
		}
		if p[1] > box.gMax {
			box.gMax = p[1]
		}
		if p[2] < box.bMin {
			box.bMin = p[2]
		}
		if p[2] > box.bMax {
			box.bMax = p[2]
		}
	}
	return box
}

func (b *colorBox) longestAxis() int {
	rRange := int(b.rMax) - int(b.rMin)
	gRange := int(b.gMax) - int(b.gMin)
	bRange := int(b.bMax) - int(b.bMin)
	if rRange >= gRange && rRange >= bRange {
		return 0
	}
	if gRange >= bRange {
		return 1
	}
	return 2
}

func (b *colorBox) average() color.NRGBA {
	if len(b.pixels) == 0 {
		return color.NRGBA{0, 0, 0, 255}
	}
	var rSum, gSum, bSum int64
	for _, p := range b.pixels {
		rSum += int64(p[0])
		gSum += int64(p[1])
		bSum += int64(p[2])
	}
	n := int64(len(b.pixels))
	return color.NRGBA{
		R: uint8(rSum / n), G: uint8(gSum / n), B: uint8(bSum / n), A: 255,
	}
}

func (b *colorBox) volume() int {
	return (int(b.rMax) - int(b.rMin) + 1) *
		(int(b.gMax) - int(b.gMin) + 1) *
		(int(b.bMax) - int(b.bMin) + 1)
}

func medianCut(img *image.NRGBA, maxColors int) color.Palette {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	maxSamples := 100000
	step := 1
	if w*h > maxSamples {
		step = (w * h) / maxSamples
		if step < 1 {
			step = 1
		}
	}

	pixels := make([][3]uint8, 0, w*h/step)
	for i := 0; i < w*h; i += step {
		off := i * 4
		if off+3 < len(img.Pix) {
			pixels = append(pixels, [3]uint8{img.Pix[off], img.Pix[off+1], img.Pix[off+2]})
		}
	}

	if len(pixels) == 0 {
		return color.Palette{color.NRGBA{0, 0, 0, 255}}
	}

	boxes := []*colorBox{newColorBox(pixels)}

	for len(boxes) < maxColors {
		bestIdx := -1
		bestScore := -1
		for i, box := range boxes {
			if len(box.pixels) < 2 {
				continue
			}
			score := box.volume() * len(box.pixels)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			break
		}

		box := boxes[bestIdx]
		axis := box.longestAxis()

		sort.Slice(box.pixels, func(i, j int) bool {
			return box.pixels[i][axis] < box.pixels[j][axis]
		})

		mid := len(box.pixels) / 2
		left := newColorBox(box.pixels[:mid])
		right := newColorBox(box.pixels[mid:])

		boxes[bestIdx] = left
		boxes = append(boxes, right)
	}

	palette := make(color.Palette, len(boxes))
	for i, box := range boxes {
		palette[i] = box.average()
	}
	return palette
}

func applyPalette(src *image.NRGBA, palette color.Palette) *image.Paletted {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	indexed := image.NewPaletted(bounds, palette)

	type cacheKey struct{ r, g, b uint8 }
	cache := make(map[cacheKey]uint8, 256)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*src.Stride + x*4
			r, g, b := src.Pix[off], src.Pix[off+1], src.Pix[off+2]

			key := cacheKey{r, g, b}
			if idx, ok := cache[key]; ok {
				indexed.Pix[y*indexed.Stride+x] = idx
				continue
			}

			bestIdx := 0
			bestDist := math.MaxInt32
			for i, c := range palette {
				pr, pg, pb, _ := c.RGBA()
				dr := int(r) - int(pr>>8)
				dg := int(g) - int(pg>>8)
				db := int(b) - int(pb>>8)
				dist := dr*dr + dg*dg + db*db
				if dist < bestDist {
					bestDist = dist
					bestIdx = i
				}
			}

			cache[key] = uint8(bestIdx)
			indexed.Pix[y*indexed.Stride+x] = uint8(bestIdx)
		}
	}
	return indexed
}

func palettedToNRGBA(p *image.Paletted) *image.NRGBA {
	bounds := p.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(bounds)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := p.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			off := y*dst.Stride + x*4
			dst.Pix[off] = uint8(r >> 8)
			dst.Pix[off+1] = uint8(g >> 8)
			dst.Pix[off+2] = uint8(b >> 8)
			dst.Pix[off+3] = uint8(a >> 8)
		}
	}
	return dst
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func copyBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func decodeJPEGFromBytes(data []byte) *image.NRGBA {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return toNRGBARef(img)
}

func computeSSIMNRGBA(a, b *image.NRGBA) float64 {
	if a.Bounds().Dx() != b.Bounds().Dx() || a.Bounds().Dy() != b.Bounds().Dy() {
		b = lanczosResize(b, a.Bounds().Dx(), a.Bounds().Dy())
	}
	return SSIMFast(a, b)
}
