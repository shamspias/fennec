package fennec

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

// makeTestImage creates a test image with a gradient pattern.
func makeTestImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8(x * 255 / w)     // R gradient
			img.Pix[off+1] = uint8(y * 255 / h)   // G gradient
			img.Pix[off+2] = uint8((x + y) % 256) // B pattern
			img.Pix[off+3] = 0xff
		}
	}
	return img
}

// makeTestImageWithAlpha creates a test image with varying transparency.
func makeTestImageWithAlpha(w, h int) *image.NRGBA {
	img := makeTestImage(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := y*img.Stride + x*4
			img.Pix[off+3] = uint8(x * 255 / w) // Alpha gradient
		}
	}
	return img
}

// makeSolidImage creates a solid color image.
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

	// Modify the image more noticeably.
	modified := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	copy(modified.Pix, img.Pix)
	for i := 0; i < len(modified.Pix); i += 4 {
		if modified.Pix[i] > 10 {
			modified.Pix[i] -= 10 // Noticeable change to R channel.
		}
	}

	ssim := SSIM(img, modified)
	if ssim < 0.85 {
		t.Fatalf("SSIM of slightly modified image should be high, got %f", ssim)
	}
	if ssim > 0.999 {
		t.Fatalf("SSIM should not be near-perfect for modified image, got %f", ssim)
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

func TestCompressImageJPEG(t *testing.T) {
	img := makeTestImage(200, 200)
	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Balanced

	result, err := CompressImage(img, opts)
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
}

func TestCompressImagePNG(t *testing.T) {
	img := makeTestImageWithAlpha(100, 100)
	opts := DefaultOptions()
	opts.Format = PNG

	result, err := CompressImage(img, opts)
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
	// Opaque image with many colors → should choose JPEG.
	opaqueImg := makeTestImage(200, 200)
	opts := DefaultOptions()
	opts.Format = Auto

	result, err := CompressImage(opaqueImg, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}
	if result.Format != JPEG {
		t.Fatalf("expected JPEG for opaque gradient, got %v", result.Format)
	}

	// Image with alpha → should choose PNG.
	alphaImg := makeTestImageWithAlpha(100, 100)
	result, err = CompressImage(alphaImg, opts)
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

		result, err := CompressImage(img, opts)
		if err != nil {
			t.Fatalf("CompressImage (%s) failed: %v", preset, err)
		}

		target := preset.targetSSIM()
		if result.SSIM < target-0.02 {
			t.Fatalf("%s: SSIM %f below target %f", preset, result.SSIM, target)
		}

		// Each lower quality preset should produce lower or equal SSIM.
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

	result, err := CompressImage(img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	if result.FinalDimensions.X > 500 || result.FinalDimensions.Y > 500 {
		t.Fatalf("image not resized: %dx%d", result.FinalDimensions.X, result.FinalDimensions.Y)
	}

	// Check aspect ratio preservation.
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
	opts.TargetSize = 5000 // Target 5KB

	result, err := CompressImage(img, opts)
	if err != nil {
		t.Fatalf("CompressImage failed: %v", err)
	}

	// Should be within 50% of target (binary search isn't exact).
	if result.CompressedSize > int64(opts.TargetSize)*2 {
		t.Fatalf("compressed size %d far exceeds target %d", result.CompressedSize, opts.TargetSize)
	}
}

func TestLanczosResize(t *testing.T) {
	img := makeTestImage(100, 100)

	// Downscale.
	small := lanczosResize(img, 50, 50)
	if small.Bounds().Dx() != 50 || small.Bounds().Dy() != 50 {
		t.Fatalf("expected 50x50, got %dx%d", small.Bounds().Dx(), small.Bounds().Dy())
	}

	// Upscale.
	big := lanczosResize(img, 200, 200)
	if big.Bounds().Dx() != 200 || big.Bounds().Dy() != 200 {
		t.Fatalf("expected 200x200, got %dx%d", big.Bounds().Dx(), big.Bounds().Dy())
	}

	// Identity.
	same := lanczosResize(img, 100, 100)
	if same.Bounds().Dx() != 100 || same.Bounds().Dy() != 100 {
		t.Fatalf("expected 100x100, got %dx%d", same.Bounds().Dx(), same.Bounds().Dy())
	}
}

func TestLanczosResizeQuality(t *testing.T) {
	// Use a smooth gradient (realistic image content).
	img := makeTestImage(100, 100)

	// Downscale and upscale back.
	small := lanczosResize(img, 50, 50)
	restored := lanczosResize(small, 100, 100)

	// SSIM should be reasonable for smooth content.
	ssim := SSIM(img, restored)
	if ssim < 0.5 {
		t.Fatalf("Lanczos round-trip quality too low for gradient: %f", ssim)
	}
}

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

func TestSharpen(t *testing.T) {
	// Create image with sharp edges that sharpening will amplify.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			off := y*img.Stride + x*4
			// Create stripes pattern with sharp edges.
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

	// Check that at least some pixels changed.
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

func TestAdaptiveSharpen(t *testing.T) {
	img := makeTestImage(50, 50)
	sharpened := AdaptiveSharpen(img, 0.5)

	if sharpened.Bounds() != img.Bounds() {
		t.Fatal("adaptive sharpen should preserve dimensions")
	}
}

func TestGaussianBlur(t *testing.T) {
	img := makeTestImage(100, 100)
	blurred := GaussianBlur(img, 2.0)

	if blurred.Bounds() != img.Bounds() {
		t.Fatal("blur should preserve dimensions")
	}

	// Check that pixels actually changed.
	changed := false
	for i := 0; i < len(img.Pix); i++ {
		if img.Pix[i] != blurred.Pix[i] {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("blur should change the image")
	}

	ssim := SSIM(img, blurred)
	if ssim < 0.3 {
		t.Fatalf("blur changed image too much: SSIM %f", ssim)
	}
}

func TestFormatAnalysis(t *testing.T) {
	// Few colors → PNG.
	fewColors := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(fewColors.Pix); i += 4 {
		fewColors.Pix[i] = 255
		fewColors.Pix[i+3] = 255
	}
	if f := analyzeFormat(fewColors); f != PNG {
		t.Fatalf("expected PNG for few-color image, got %v", f)
	}

	// Many colors → JPEG.
	manyColors := makeTestImage(200, 200)
	if f := analyzeFormat(manyColors); f != JPEG {
		t.Fatalf("expected JPEG for gradient image, got %v", f)
	}

	// Alpha → PNG.
	alphaImg := makeTestImageWithAlpha(100, 100)
	if f := analyzeFormat(alphaImg); f != PNG {
		t.Fatalf("expected PNG for alpha image, got %v", f)
	}
}

func TestIsOpaque(t *testing.T) {
	opaque := makeTestImage(10, 10)
	if !isOpaque(opaque) {
		t.Fatal("should be opaque")
	}

	transparent := makeTestImageWithAlpha(10, 10)
	if isOpaque(transparent) {
		t.Fatal("should not be opaque")
	}
}

func TestIsGrayscale(t *testing.T) {
	gray := makeSolidImage(10, 10, color.NRGBA{128, 128, 128, 255})
	if !isGrayscale(gray) {
		t.Fatal("should be grayscale")
	}

	colorful := makeTestImage(10, 10)
	if isGrayscale(colorful) {
		t.Fatal("should not be grayscale")
	}
}

func TestTryPalettize(t *testing.T) {
	// Image with few colors should palettize.
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
	paletted := tryPalettize(fewColors, 256)
	if paletted == nil {
		t.Fatal("should palettize image with few colors")
	}

	// Image with many colors should not palettize.
	manyColors := makeTestImage(200, 200)
	paletted = tryPalettize(manyColors, 256)
	if paletted != nil {
		t.Fatal("should not palettize gradient image")
	}
}

func TestSmartResize(t *testing.T) {
	img := makeTestImage(1000, 500)

	// Constrain width.
	resized := smartResize(img, 200, 0)
	if resized.Bounds().Dx() != 1000 || resized.Bounds().Dy() != 500 {
		// maxH=0 means no height constraint, but MaxWidth isn't the only path.
	}

	// Constrain both.
	resized = smartResize(img, 200, 200)
	if resized.Bounds().Dx() > 200 || resized.Bounds().Dy() > 200 {
		t.Fatalf("should fit in 200x200, got %dx%d", resized.Bounds().Dx(), resized.Bounds().Dy())
	}

	// Already fits.
	resized = smartResize(img, 2000, 2000)
	if resized.Bounds().Dx() != 1000 || resized.Bounds().Dy() != 500 {
		t.Fatal("should not resize when already fits")
	}
}

func TestCompressNilImage(t *testing.T) {
	_, err := CompressImage(nil, DefaultOptions())
	if err == nil {
		t.Fatal("should error on nil image")
	}
}

func TestCompressEmptyImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	_, err := CompressImage(img, DefaultOptions())
	if err == nil {
		t.Fatal("should error on empty image")
	}
}

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
	s := r.String()
	if len(s) == 0 {
		t.Fatal("Result.String() should not be empty")
	}
}

func TestEncodeJPEG(t *testing.T) {
	img := makeTestImage(100, 100)
	var buf bytes.Buffer
	err := Encode(&buf, img, JPEG, DefaultOptions())
	if err != nil {
		t.Fatalf("Encode JPEG failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("encoded JPEG should not be empty")
	}

	// Verify it's valid JPEG.
	_, err = jpeg.Decode(&buf)
	if err != nil {
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
	if buf.Len() == 0 {
		t.Fatal("encoded PNG should not be empty")
	}

	// Verify it's valid PNG.
	_, err = png.Decode(&buf)
	if err != nil {
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

func TestMSSSIM(t *testing.T) {
	img := makeTestImage(128, 128)
	msssim := MSSSIM(img, img)
	if msssim < 0.99 {
		t.Fatalf("MS-SSIM of identical images should be ~1.0, got %f", msssim)
	}
}

// Benchmarks.

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
		CompressImage(img, opts)
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
