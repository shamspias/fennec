package fennec

import (
	"image"
	"image/color"
	"testing"
)

// ── Median-Cut Quantizer Tests ──────────────────────────────────────────────

func TestMedianCutBasic(t *testing.T) {
	// 4-color image should produce exactly 4 colors.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	colors := []color.NRGBA{
		{255, 0, 0, 255},   // red
		{0, 255, 0, 255},   // green
		{0, 0, 255, 255},   // blue
		{255, 255, 0, 255}, // yellow
	}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := colors[(y/50)*2+x/50]
			off := y*img.Stride + x*4
			img.Pix[off] = c.R
			img.Pix[off+1] = c.G
			img.Pix[off+2] = c.B
			img.Pix[off+3] = c.A
		}
	}

	palette := medianCut(img, 4)
	if len(palette) != 4 {
		t.Fatalf("Expected 4 colors, got %d", len(palette))
	}

	// Each palette color should be close to one of the input colors.
	for _, pc := range palette {
		r, g, b, _ := pc.RGBA()
		pr, pg, pb := int(r>>8), int(g>>8), int(b>>8)
		found := false
		for _, c := range colors {
			dr := absInt(pr - int(c.R))
			dg := absInt(pg - int(c.G))
			db := absInt(pb - int(c.B))
			if dr < 10 && dg < 10 && db < 10 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Palette color (%d,%d,%d) doesn't match any input color", pr, pg, pb)
		}
	}
}

func TestMedianCutReduction(t *testing.T) {
	// Gradient image with thousands of colors → reduce to 16.
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8(x * 255 / 200)
			img.Pix[off+1] = uint8(y * 255 / 200)
			img.Pix[off+2] = 128
			img.Pix[off+3] = 255
		}
	}

	palette := medianCut(img, 16)
	if len(palette) > 16 {
		t.Fatalf("Expected ≤16 colors, got %d", len(palette))
	}
	if len(palette) < 8 {
		t.Fatalf("Expected reasonable palette size, got only %d", len(palette))
	}
}

func TestApplyPalette(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8(x * 5)
			img.Pix[off+1] = uint8(y * 5)
			img.Pix[off+2] = 100
			img.Pix[off+3] = 255
		}
	}

	palette := medianCut(img, 32)
	indexed := applyPalette(img, palette)

	if indexed.Bounds().Dx() != 50 || indexed.Bounds().Dy() != 50 {
		t.Fatal("Indexed image has wrong dimensions")
	}

	// Every pixel index should be valid.
	for i, idx := range indexed.Pix[:50*50] {
		if int(idx) >= len(palette) {
			t.Fatalf("Pixel %d has invalid palette index %d (palette size %d)",
				i, idx, len(palette))
		}
	}
}

func TestPalettedToNRGBA(t *testing.T) {
	palette := color.Palette{
		color.NRGBA{255, 0, 0, 255},
		color.NRGBA{0, 255, 0, 255},
	}
	indexed := image.NewPaletted(image.Rect(0, 0, 10, 10), palette)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x < 5 {
				indexed.Pix[y*indexed.Stride+x] = 0
			} else {
				indexed.Pix[y*indexed.Stride+x] = 1
			}
		}
	}

	nrgba := palettedToNRGBA(indexed)
	// Check a red pixel.
	r, g, b, a := nrgba.At(2, 2).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Fatalf("Expected red, got (%d,%d,%d,%d)", r>>8, g>>8, b>>8, a>>8)
	}
	// Check a green pixel.
	r, g, b, a = nrgba.At(7, 7).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 || a>>8 != 255 {
		t.Fatalf("Expected green, got (%d,%d,%d,%d)", r>>8, g>>8, b>>8, a>>8)
	}
}

// ── Target Size Engine Tests ────────────────────────────────────────────────

func TestHitTargetSizeJPEG(t *testing.T) {
	img := makeGradient(400, 300)

	target := 5 * 1024 // 5 KB
	opts := DefaultOptions()
	opts.Format = Auto

	sr, err := hitTargetSize(img, target, opts)
	if err != nil {
		t.Fatalf("hitTargetSize failed: %v", err)
	}

	if sr == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Strategy result: format=%v size=%d target=%d ssim=%.4f quality=%d dims=%dx%d",
		sr.format, len(sr.data), target, sr.ssim, sr.quality, sr.finalW, sr.finalH)

	if int64(len(sr.data)) > int64(target)*2 {
		t.Fatalf("Result %d bytes way over target %d", len(sr.data), target)
	}

	if sr.ssim < 0.5 {
		t.Fatalf("SSIM too low: %.4f", sr.ssim)
	}
}

func TestHitTargetSizePNGQuantize(t *testing.T) {
	// Image with few-ish colors — quantization should be effective.
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	colors := []color.NRGBA{
		{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255},
		{255, 255, 0, 255}, {255, 0, 255, 255}, {0, 255, 255, 255},
		{128, 128, 128, 255}, {255, 255, 255, 255},
	}
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			c := colors[(y/25+x/25)%len(colors)]
			off := y*img.Stride + x*4
			img.Pix[off] = c.R
			img.Pix[off+1] = c.G
			img.Pix[off+2] = c.B
			img.Pix[off+3] = c.A
		}
	}

	target := 2 * 1024 // 2 KB
	opts := DefaultOptions()
	opts.Format = PNG

	sr, err := hitTargetSize(img, target, opts)
	if err != nil {
		t.Fatalf("hitTargetSize failed: %v", err)
	}

	if sr == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("PNG result: size=%d target=%d ssim=%.4f dims=%dx%d",
		len(sr.data), target, sr.ssim, sr.finalW, sr.finalH)
}

func TestHitTargetSizeLargeImage(t *testing.T) {
	// 800×600 gradient → compress to 20 KB.
	img := makeGradient(800, 600)

	target := 20 * 1024 // 20 KB
	opts := DefaultOptions()
	opts.Format = Auto

	sr, err := hitTargetSize(img, target, opts)
	if err != nil {
		t.Fatalf("hitTargetSize failed: %v", err)
	}

	if sr == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Large image result: format=%v size=%d (%.1f KB) target=%d ssim=%.4f q=%d dims=%dx%d",
		sr.format, len(sr.data), float64(len(sr.data))/1024, target,
		sr.ssim, sr.quality, sr.finalW, sr.finalH)

	if int64(len(sr.data)) > int64(target) {
		t.Logf("Warning: %d bytes over target %d (%.1f%% over)",
			len(sr.data)-target, target,
			float64(len(sr.data)-target)/float64(target)*100)
	}
}

func TestBetterFit(t *testing.T) {
	target := 1000

	underSmall := &sizeResult{data: make([]byte, 800), ssim: 0.9}
	underLarge := &sizeResult{data: make([]byte, 950), ssim: 0.95}
	overSmall := &sizeResult{data: make([]byte, 1100), ssim: 0.99}
	overLarge := &sizeResult{data: make([]byte, 2000), ssim: 0.99}

	// Under-target beats over-target.
	if !betterFit(underSmall, overSmall, target) {
		t.Error("Under-target should beat over-target")
	}

	// Higher SSIM wins when both under target.
	if !betterFit(underLarge, underSmall, target) {
		t.Error("Higher SSIM should win when both under target")
	}

	// Smaller size wins when both over target.
	if !betterFit(overSmall, overLarge, target) {
		t.Error("Smaller should win when both over target")
	}
}

func TestJPEGQualitySearch(t *testing.T) {
	img := makeGradient(200, 150)

	sr, err := jpegQualitySearch(img, 3*1024)
	if err != nil {
		t.Fatalf("jpegQualitySearch failed: %v", err)
	}

	if sr == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Q=%d size=%d (%.1f KB) ssim=%.4f",
		sr.quality, len(sr.data), float64(len(sr.data))/1024, sr.ssim)

	if int64(len(sr.data)) > 3*1024 {
		t.Fatalf("Result %d bytes over 3 KB target", len(sr.data))
	}

	if sr.quality < 1 || sr.quality > 100 {
		t.Fatalf("Invalid quality: %d", sr.quality)
	}
}

func TestScaleSearch(t *testing.T) {
	img := makeGradient(400, 300)

	sr, err := scaleSearch(img, 2*1024, JPEG)
	if err != nil {
		t.Fatalf("scaleSearch failed: %v", err)
	}

	if sr == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Scale search: size=%d (%.1f KB) dims=%dx%d ssim=%.4f",
		len(sr.data), float64(len(sr.data))/1024, sr.finalW, sr.finalH, sr.ssim)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func makeGradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8(x * 255 / w)
			img.Pix[off+1] = uint8(y * 255 / h)
			img.Pix[off+2] = uint8((x + y) % 256)
			img.Pix[off+3] = 255
		}
	}
	return img
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
