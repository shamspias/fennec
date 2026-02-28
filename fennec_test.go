package fennec

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

// ── Test Helpers ────────────────────────────────────────────────────────────

func makeTestImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8(x * 255 / w)
			img.Pix[off+1] = uint8(y * 255 / h)
			img.Pix[off+2] = uint8((x + y) % 256)
			img.Pix[off+3] = 0xff
		}
	}
	return img
}

func makeTestImageWithAlpha(w, h int) *image.NRGBA {
	img := makeTestImage(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*img.Stride + x*4
			img.Pix[off+3] = uint8(x * 255 / w)
		}
	}
	return img
}

func makeSolidImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}
	return img
}

func ctx() context.Context { return context.Background() }

// ── SSIM Tests ──────────────────────────────────────────────────────────────

func TestSSIMIdentical(t *testing.T) {
	img := makeTestImage(100, 100)
	ssim := SSIM(img, img)
	if ssim < 0.999 {
		t.Fatalf("SSIM of identical images should be ~1.0, got %f", ssim)
	}
}

func TestSSIMDifferent(t *testing.T) {
	img1 := makeSolidImage(100, 100, color.NRGBA{0, 0, 0, 255})
	img2 := makeSolidImage(100, 100, color.NRGBA{255, 255, 255, 255})
	ssim := SSIM(img1, img2)
	if ssim > 0.1 {
		t.Fatalf("SSIM of black vs white should be very low, got %f", ssim)
	}
}

func TestSSIMSimilar(t *testing.T) {
	img := makeTestImage(100, 100)
	modified := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	copy(modified.Pix, img.Pix)
	for i := 0; i < len(modified.Pix); i += 4 {
		if modified.Pix[i] > 10 {
			modified.Pix[i] -= 10
		}
	}

	ssim := SSIM(img, modified)
	if ssim < 0.85 || ssim > 0.999 {
		t.Fatalf("SSIM of slightly modified image should be in [0.85, 0.999), got %f", ssim)
	}
}

func TestSSIMFast(t *testing.T) {
	img := makeTestImage(500, 500)
	ssim := SSIMFast(img, img)
	if ssim < 0.999 {
		t.Fatalf("SSIMFast of identical images should be ~1.0, got %f", ssim)
	}
}

func TestSSIMSmallImage(t *testing.T) {
	img := makeTestImage(4, 4)
	ssim := SSIM(img, img)
	if ssim < 0.999 {
		t.Fatalf("SSIM of small identical images should be ~1.0, got %f", ssim)
	}
}

func TestMSSSIM(t *testing.T) {
	img := makeTestImage(128, 128)
	msssim := MSSSIM(img, img)
	if msssim < 0.99 {
		t.Fatalf("MS-SSIM of identical images should be ~1.0, got %f", msssim)
	}
}

// ── Compression Tests ───────────────────────────────────────────────────────

func TestCompressImageJPEG(t *testing.T) {
	img := makeTestImage(200, 200)
	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Balanced

	result, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	if result.Format != JPEG {
		t.Fatalf("expected JPEG format, got %v", result.Format)
	}
	if result.SSIM < 0.90 {
		t.Fatalf("SSIM too low: %f", result.SSIM)
	}
	if result.JPEGQuality < 1 || result.JPEGQuality > 100 {
		t.Fatalf("invalid JPEG quality: %d", result.JPEGQuality)
	}
	// Phase 1: verify CompressedData is populated.
	if len(result.CompressedData) == 0 {
		t.Fatal("CompressedData should not be empty")
	}
}

func TestCompressImagePNG(t *testing.T) {
	img := makeTestImageWithAlpha(100, 100)
	opts := DefaultOptions()
	opts.Format = PNG

	result, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	if result.Format != PNG {
		t.Fatalf("expected PNG format, got %v", result.Format)
	}
	if result.SSIM != 1.0 {
		t.Fatalf("PNG should be lossless, SSIM: %f", result.SSIM)
	}
}

func TestCompressAutoFormat(t *testing.T) {
	opaqueImg := makeTestImage(200, 200)
	opts := DefaultOptions()
	opts.Format = Auto

	result, err := CompressImage(ctx(), opaqueImg, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}
	if result.Format != JPEG {
		t.Fatalf("expected JPEG for opaque gradient, got %v", result.Format)
	}

	alphaImg := makeTestImageWithAlpha(100, 100)
	result, err = CompressImage(ctx(), alphaImg, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}
	if result.Format != PNG {
		t.Fatalf("expected PNG for transparent image, got %v", result.Format)
	}
}

func TestCompressQualityPresets(t *testing.T) {
	img := makeTestImage(200, 200)

	presets := []Quality{Ultra, High, Balanced, Aggressive, Maximum}
	var prevSSIM float64 = 2.0

	for _, preset := range presets {
		opts := DefaultOptions()
		opts.Format = JPEG
		opts.Quality = preset

		result, err := CompressImage(ctx(), img, opts)
		if err != nil {
			t.Fatalf("CompressImage (%s) failed: %v", preset, err)
		}

		target := preset.targetSSIM()
		if result.SSIM < target-0.02 {
			t.Fatalf("%s: SSIM %f below target %f", preset, result.SSIM, target)
		}

		if result.SSIM > prevSSIM+0.01 {
			t.Logf("Warning: %s SSIM (%f) higher than previous (%f)", preset, result.SSIM, prevSSIM)
		}
		prevSSIM = result.SSIM
	}
}

func TestCompressWithResize(t *testing.T) {
	img := makeTestImage(1000, 800)
	opts := DefaultOptions()
	opts.Format = JPEG
	opts.MaxWidth = 500
	opts.MaxHeight = 500

	result, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	if result.FinalDimensions.X > 500 || result.FinalDimensions.Y > 500 {
		t.Fatalf("image not resized: %dx%d", result.FinalDimensions.X, result.FinalDimensions.Y)
	}

	originalRatio := 1000.0 / 800.0
	newRatio := float64(result.FinalDimensions.X) / float64(result.FinalDimensions.Y)
	if math.Abs(originalRatio-newRatio) > 0.02 {
		t.Fatalf("aspect ratio not preserved: original %f, new %f", originalRatio, newRatio)
	}
}

func TestCompressTargetSize(t *testing.T) {
	img := makeTestImage(300, 300)
	opts := DefaultOptions()
	opts.Format = JPEG
	opts.TargetSize = 5000

	result, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	if result.CompressedSize > int64(opts.TargetSize)*2 {
		t.Fatalf("compressed size %d far exceeds target %d", result.CompressedSize, opts.TargetSize)
	}
}

func TestCompressNilImage(t *testing.T) {
	_, err := CompressImage(ctx(), nil, DefaultOptions())
	if err == nil {
		t.Fatal("should error on nil image")
	}
}

func TestCompressEmptyImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	_, err := CompressImage(ctx(), img, DefaultOptions())
	if err == nil {
		t.Fatal("should error on empty image")
	}
}

// ── Context Cancellation Tests ──────────────────────────────────────────────

func TestCompressContextCancelled(t *testing.T) {
	img := makeTestImage(200, 200)
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := CompressImage(cancelledCtx, img, DefaultOptions())
	if err == nil {
		t.Fatal("should error on cancelled context")
	}
}

// ── CompressBytes Test ──────────────────────────────────────────────────────

func TestCompressBytes(t *testing.T) {
	img := makeTestImage(100, 100)
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})

	result, err := CompressBytes(ctx(), buf.Bytes(), DefaultOptions())
	if err != nil {
		t.Fatalf("CompressBytes failed: %v", err)
	}
	if len(result.CompressedData) == 0 {
		t.Fatal("CompressedData should not be empty")
	}
}

// ── Result.WriteTo Test ─────────────────────────────────────────────────────

func TestResultWriteTo(t *testing.T) {
	img := makeTestImage(100, 100)
	opts := DefaultOptions()
	opts.Format = JPEG
	result, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	n, err := result.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n != int64(len(result.CompressedData)) {
		t.Fatalf("WriteTo wrote %d bytes, expected %d", n, len(result.CompressedData))
	}
	if buf.Len() == 0 {
		t.Fatal("WriteTo output should not be empty")
	}
}

// ── Progress Callback Test ──────────────────────────────────────────────────

func TestProgressCallback(t *testing.T) {
	img := makeTestImage(100, 100)
	var stages []ProgressStage

	opts := DefaultOptions()
	opts.Format = JPEG
	opts.OnProgress = func(stage ProgressStage, percent float64) error {
		stages = append(stages, stage)
		return nil
	}

	_, err := CompressImage(ctx(), img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}
	if len(stages) == 0 {
		t.Fatal("progress callback was never called")
	}
}

// ── Resize Tests ────────────────────────────────────────────────────────────

func TestLanczosResize(t *testing.T) {
	img := makeTestImage(100, 100)

	small := lanczosResize(img, 50, 50)
	if small.Bounds().Dx() != 50 || small.Bounds().Dy() != 50 {
		t.Fatalf("expected 50x50, got %dx%d", small.Bounds().Dx(), small.Bounds().Dy())
	}

	big := lanczosResize(img, 200, 200)
	if big.Bounds().Dx() != 200 || big.Bounds().Dy() != 200 {
		t.Fatalf("expected 200x200, got %dx%d", big.Bounds().Dx(), big.Bounds().Dy())
	}

	same := lanczosResize(img, 100, 100)
	if same.Bounds().Dx() != 100 || same.Bounds().Dy() != 100 {
		t.Fatalf("expected 100x100, got %dx%d", same.Bounds().Dx(), same.Bounds().Dy())
	}
}

func TestLanczosResizeQuality(t *testing.T) {
	img := makeTestImage(100, 100)
	small := lanczosResize(img, 50, 50)
	restored := lanczosResize(small, 100, 100)

	ssim := SSIM(img, restored)
	if ssim < 0.5 {
		t.Fatalf("Lanczos round-trip quality too low: %f", ssim)
	}
}

func TestSmartResize(t *testing.T) {
	img := makeTestImage(1000, 500)

	resized := smartResize(img, 200, 200)
	if resized.Bounds().Dx() > 200 || resized.Bounds().Dy() > 200 {
		t.Fatalf("should fit in 200x200, got %dx%d", resized.Bounds().Dx(), resized.Bounds().Dy())
	}

	resized = smartResize(img, 2000, 2000)
	if resized.Bounds().Dx() != 1000 || resized.Bounds().Dy() != 500 {
		t.Fatal("should not resize when already fits")
	}
}

// ── Analysis Tests ──────────────────────────────────────────────────────────

func TestAnalyze(t *testing.T) {
	t.Run("gradient", func(t *testing.T) {
		img := makeTestImage(200, 200)
		stats := Analyze(img)
		if stats.Width != 200 || stats.Height != 200 {
			t.Fatalf("wrong dimensions: %dx%d", stats.Width, stats.Height)
		}
		if stats.HasAlpha {
			t.Fatal("should not have alpha")
		}
		if stats.Entropy < 1 {
			t.Fatalf("entropy too low for gradient: %f", stats.Entropy)
		}
	})

	t.Run("solid_color", func(t *testing.T) {
		img := makeSolidImage(100, 100, color.NRGBA{128, 128, 128, 255})
		stats := Analyze(img)
		if !stats.IsGrayscale {
			t.Fatal("should be grayscale")
		}
		if stats.Entropy > 0.01 {
			t.Fatalf("entropy should be ~0 for solid color: %f", stats.Entropy)
		}
	})

	t.Run("with_alpha", func(t *testing.T) {
		img := makeTestImageWithAlpha(100, 100)
		stats := Analyze(img)
		if !stats.HasAlpha {
			t.Fatal("should have alpha")
		}
		if stats.RecommendedFormat != PNG {
			t.Fatal("should recommend PNG for alpha image")
		}
	})
}

// ── Effects Tests ───────────────────────────────────────────────────────────

func TestSharpen(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			off := y*img.Stride + x*4
			if (x/10)%2 == 0 {
				img.Pix[off] = 200
				img.Pix[off+1] = 50
				img.Pix[off+2] = 100
			} else {
				img.Pix[off] = 50
				img.Pix[off+1] = 200
				img.Pix[off+2] = 100
			}
			img.Pix[off+3] = 255
		}
	}

	sharpened := Sharpen(img, 0.8)
	if sharpened.Bounds() != img.Bounds() {
		t.Fatal("sharpen should preserve dimensions")
	}

	changed := false
	for i := 0; i < len(img.Pix); i++ {
		if img.Pix[i] != sharpened.Pix[i] {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("sharpen should change some pixels")
	}
}

func TestGaussianBlur(t *testing.T) {
	img := makeTestImage(100, 100)
	blurred := GaussianBlur(img, 2.0)
	if blurred.Bounds() != img.Bounds() {
		t.Fatal("blur should preserve dimensions")
	}
	ssim := SSIM(img, blurred)
	if ssim < 0.3 {
		t.Fatalf("blur changed image too much: SSIM %f", ssim)
	}
}

// ── Conversion Tests ────────────────────────────────────────────────────────

func TestFormatAnalysis(t *testing.T) {
	fewColors := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(fewColors.Pix); i += 4 {
		fewColors.Pix[i] = 255
		fewColors.Pix[i+3] = 255
	}
	if f := analyzeFormat(fewColors); f != PNG {
		t.Fatalf("expected PNG for few-color image, got %v", f)
	}

	manyColors := makeTestImage(200, 200)
	if f := analyzeFormat(manyColors); f != JPEG {
		t.Fatalf("expected JPEG for gradient image, got %v", f)
	}

	alphaImg := makeTestImageWithAlpha(100, 100)
	if f := analyzeFormat(alphaImg); f != PNG {
		t.Fatalf("expected PNG for alpha image, got %v", f)
	}
}

func TestIsOpaque(t *testing.T) {
	if !isOpaque(makeTestImage(10, 10)) {
		t.Fatal("should be opaque")
	}
	if isOpaque(makeTestImageWithAlpha(10, 10)) {
		t.Fatal("should not be opaque")
	}
}

func TestIsGrayscale(t *testing.T) {
	gray := makeSolidImage(10, 10, color.NRGBA{128, 128, 128, 255})
	if !isGrayscale(gray) {
		t.Fatal("should be grayscale")
	}
	if isGrayscale(makeTestImage(10, 10)) {
		t.Fatal("should not be grayscale")
	}
}

func TestTryPalettize(t *testing.T) {
	fewColors := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			off := y*fewColors.Stride + x*4
			if x < 25 {
				fewColors.Pix[off] = 255
			}
			fewColors.Pix[off+3] = 255
		}
	}
	if tryPalettize(fewColors, 256) == nil {
		t.Fatal("should palettize image with few colors")
	}

	if tryPalettize(makeTestImage(200, 200), 256) != nil {
		t.Fatal("should not palettize gradient image")
	}
}

// ── EXIF Orientation Tests ──────────────────────────────────────────────────

func TestApplyOrientation(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	// Mark top-left red.
	img.Pix[0] = 255
	img.Pix[3] = 255

	// Normal — should be unchanged.
	normal := ApplyOrientation(img, OrientNormal)
	if normal.Bounds().Dx() != 100 || normal.Bounds().Dy() != 50 {
		t.Fatal("normal should be 100x50")
	}

	// Rotate 90 CW — should swap dimensions.
	rotated := ApplyOrientation(img, OrientRotate90CW)
	if rotated.Bounds().Dx() != 50 || rotated.Bounds().Dy() != 100 {
		t.Fatalf("90CW should be 50x100, got %dx%d", rotated.Bounds().Dx(), rotated.Bounds().Dy())
	}

	// Rotate 180 — should keep dimensions.
	rot180 := ApplyOrientation(img, OrientRotate180)
	if rot180.Bounds().Dx() != 100 || rot180.Bounds().Dy() != 50 {
		t.Fatal("180 should be 100x50")
	}
}

// ── Type Tests ──────────────────────────────────────────────────────────────

func TestQualityString(t *testing.T) {
	presets := map[Quality]string{
		Lossless:   "Lossless",
		Ultra:      "Ultra",
		High:       "High",
		Balanced:   "Balanced",
		Aggressive: "Aggressive",
		Maximum:    "Maximum",
	}
	for q, name := range presets {
		if q.String() != name {
			t.Fatalf("expected %q, got %q", name, q.String())
		}
	}
}

func TestDefaultQualityIsBalanced(t *testing.T) {
	// Phase 1 fix: zero-value of Quality should be Balanced.
	var q Quality
	if q != Balanced {
		t.Fatalf("zero-value Quality should be Balanced, got %s", q)
	}
	opts := Options{}
	if opts.Quality != Balanced {
		t.Fatalf("zero-value Options.Quality should be Balanced, got %s", opts.Quality)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1536000, "1.5 MB"},
	}
	for _, tc := range cases {
		got := humanBytes(tc.bytes)
		if got != tc.want {
			t.Fatalf("humanBytes(%d): got %q want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestResultString(t *testing.T) {
	r := Result{
		Format:             JPEG,
		OriginalDimensions: image.Pt(1000, 800),
		FinalDimensions:    image.Pt(500, 400),
		OriginalSize:       500000,
		CompressedSize:     50000,
		SSIM:               0.9650,
		SavingsPercent:     90.0,
	}
	if len(r.String()) == 0 {
		t.Fatal("Result.String() should not be empty")
	}
}

func TestEncodeJPEG(t *testing.T) {
	img := makeTestImage(100, 100)
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = JPEG
	err := Encode(&buf, img, JPEG, opts)
	if err != nil {
		t.Fatalf("Encode JPEG failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("encoded JPEG should not be empty")
	}
	if _, err = jpeg.Decode(&buf); err != nil {
		t.Fatalf("encoded data is not valid JPEG: %v", err)
	}
}

func TestEncodePNG(t *testing.T) {
	img := makeTestImage(100, 100)
	var buf bytes.Buffer
	err := Encode(&buf, img, PNG, DefaultOptions())
	if err != nil {
		t.Fatalf("Encode PNG failed: %v", err)
	}
	if _, err = png.Decode(&buf); err != nil {
		t.Fatalf("encoded data is not valid PNG: %v", err)
	}
}

func TestBoxDownsample(t *testing.T) {
	img := makeTestImage(100, 100)
	small := boxDownsample(img, 10, 10)
	if small.Bounds().Dx() != 10 || small.Bounds().Dy() != 10 {
		t.Fatalf("expected 10x10, got %dx%d", small.Bounds().Dx(), small.Bounds().Dy())
	}
}

// ── Benchmarks ──────────────────────────────────────────────────────────────

func BenchmarkSSIM(b *testing.B) {
	img := makeTestImage(500, 500)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SSIM(img, img)
	}
}

func BenchmarkSSIMFast(b *testing.B) {
	img := makeTestImage(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SSIMFast(img, img)
	}
}

func BenchmarkLanczosResize(b *testing.B) {
	img := makeTestImage(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lanczosResize(img, 500, 500)
	}
}

func BenchmarkCompressJPEG(b *testing.B) {
	img := makeTestImage(500, 500)
	opts := DefaultOptions()
	opts.Format = JPEG
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		CompressImage(ctx(), img, opts)
	}
}

func BenchmarkAnalyze(b *testing.B) {
	img := makeTestImage(1000, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Analyze(img)
	}
}

func BenchmarkGaussianBlur(b *testing.B) {
	img := makeTestImage(500, 500)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GaussianBlur(img, 2.0)
	}
}
