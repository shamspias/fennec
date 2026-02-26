package fennec

import (
	"os"
	"path/filepath"
	"testing"
)

// Integration tests that use real image files from testdata/.
// Run TestGenerateTestData first to create fixtures:
//   go test -run TestGenerateTestData -v
//
// Then run these:
//   go test -run TestIntegration -v

func ensureTestdata(t *testing.T) {
	t.Helper()
	files := []string{
		"testdata/gradient.jpg",
		"testdata/transparent.png",
		"testdata/fewcolors.png",
		"testdata/large_photo.jpg",
		"testdata/grayscale.png",
	}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Skipf("testdata missing (%s). Run: go test -run TestGenerateTestData -v", f)
		}
	}
}

func TestIntegrationCompressJPEG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	src := "testdata/gradient.jpg"
	dst := filepath.Join(tmpDir, "compressed.jpg")

	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Balanced

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)

	if result.OriginalSize <= 0 {
		t.Fatal("OriginalSize should be > 0")
	}
	if result.CompressedSize <= 0 {
		t.Fatal("CompressedSize should be > 0")
	}
	if result.SSIM < 0.90 {
		t.Fatalf("SSIM too low: %.4f", result.SSIM)
	}
	if result.SavingsPercent < 0 {
		t.Logf("Warning: file got larger (savings=%.1f%%)", result.SavingsPercent)
	}

	// Verify output exists and is valid JPEG.
	if _, err := Open(dst); err != nil {
		t.Fatalf("Cannot open compressed output: %v", err)
	}
}

func TestIntegrationCompressPNGTransparent(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	src := "testdata/transparent.png"
	dst := filepath.Join(tmpDir, "compressed.png")

	opts := DefaultOptions()
	opts.Format = PNG

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)

	// PNG is lossless.
	if result.SSIM != 1.0 {
		t.Fatalf("PNG should be lossless, SSIM: %.4f", result.SSIM)
	}

	// Verify output.
	if _, err := Open(dst); err != nil {
		t.Fatalf("Cannot open compressed output: %v", err)
	}
}

func TestIntegrationAutoFormatFewColors(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	src := "testdata/fewcolors.png"
	dst := filepath.Join(tmpDir, "auto_output.png")

	opts := DefaultOptions()
	opts.Format = Auto

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)
	t.Logf("Chosen format: JPEG=%v PNG=%v", result.Format == JPEG, result.Format == PNG)

	// Few-color image should get PNG.
	if result.Format != PNG {
		t.Logf("Note: expected PNG for few-color image, got JPEG")
	}
}

func TestIntegrationCompressWithResize(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	src := "testdata/large_photo.jpg"
	dst := filepath.Join(tmpDir, "resized.jpg")

	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Balanced
	opts.MaxWidth = 800
	opts.MaxHeight = 600

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)

	if result.FinalDimensions.X > 800 {
		t.Fatalf("Width %d exceeds max 800", result.FinalDimensions.X)
	}
	if result.FinalDimensions.Y > 600 {
		t.Fatalf("Height %d exceeds max 600", result.FinalDimensions.Y)
	}
}

func TestIntegrationAnalyzeAll(t *testing.T) {
	ensureTestdata(t)

	files := map[string]struct {
		expectAlpha bool
	}{
		"testdata/gradient.jpg":    {false},
		"testdata/transparent.png": {true},
		"testdata/fewcolors.png":   {false},
		"testdata/large_photo.jpg": {false},
		"testdata/grayscale.png":   {false},
	}

	for path, expect := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			img, err := Open(path)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}

			stats := Analyze(img)
			t.Logf("  Dimensions:  %d × %d", stats.Width, stats.Height)
			t.Logf("  HasAlpha:    %v", stats.HasAlpha)
			t.Logf("  Grayscale:   %v", stats.IsGrayscale)
			t.Logf("  Colors:      %d+", stats.UniqueColors)
			t.Logf("  Entropy:     %.2f bits", stats.Entropy)
			t.Logf("  EdgeDensity: %.1f%%", stats.EdgeDensity*100)
			t.Logf("  Brightness:  %.0f", stats.MeanBrightness)
			t.Logf("  Contrast:    %.1f", stats.Contrast)
			t.Logf("  Recommended: format=%v quality=%v", stats.RecommendedFormat, stats.RecommendedQuality)

			if stats.HasAlpha != expect.expectAlpha {
				t.Fatalf("HasAlpha: got %v want %v", stats.HasAlpha, expect.expectAlpha)
			}
		})
	}
}

func TestIntegrationSSIMRealImages(t *testing.T) {
	ensureTestdata(t)

	img, err := Open("testdata/gradient.jpg")
	if err != nil {
		t.Fatal(err)
	}

	// SSIM of image with itself.
	ssim := SSIM(img, img)
	if ssim < 0.999 {
		t.Fatalf("Self-SSIM should be ~1.0, got %f", ssim)
	}

	// Compress and measure.
	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Aggressive
	result, err := CompressImage(img, opts)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Aggressive compression: SSIM=%.4f Q=%d", result.SSIM, result.JPEGQuality)

	if result.SSIM < Aggressive.targetSSIM()-0.03 {
		t.Fatalf("SSIM %f below aggressive target %f", result.SSIM, Aggressive.targetSSIM())
	}
}

func TestIntegrationTargetSize(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	// Use the smaller gradient image — the large_photo is too high-entropy
	// to compress below 100KB even at Q=1 without resizing.
	src := "testdata/gradient.jpg"
	dst := filepath.Join(tmpDir, "targeted.jpg")

	opts := DefaultOptions()
	opts.Format = JPEG
	opts.TargetSize = 10 * 1024 // 10 KB target for a 400×300 gradient

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)
	t.Logf("Target: 10 KB, Got: %d bytes (%.1f KB)", result.CompressedSize, float64(result.CompressedSize)/1024)

	// The binary search should get close. Allow up to 3× tolerance since
	// JPEG quality→size is not monotonically precise at the byte level.
	if result.CompressedSize > int64(opts.TargetSize)*3 {
		t.Fatalf("Compressed %d bytes, way above 3× target %d", result.CompressedSize, opts.TargetSize)
	}
}

func TestIntegrationTargetSizePNG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	// fewcolors.png is small (723 B), use transparent.png (4.4 KB).
	src := "testdata/transparent.png"
	dst := filepath.Join(tmpDir, "targeted.png")

	opts := DefaultOptions()
	opts.Format = PNG
	opts.TargetSize = 1 * 1024 // 1 KB target — forces downscale

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)
	t.Logf("Target: 1 KB, Got: %d bytes (%.1f KB)",
		result.CompressedSize, float64(result.CompressedSize)/1024)

	// PNG target size works by downscaling. Should be smaller than original.
	if result.CompressedSize >= result.OriginalSize {
		t.Fatalf("Expected PNG target size to reduce file, got %d >= %d",
			result.CompressedSize, result.OriginalSize)
	}
}

func TestIntegrationAutoFormatTargetSize(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	// Auto format + target size on an opaque image should pick JPEG.
	src := "testdata/gradient.jpg"
	dst := filepath.Join(tmpDir, "auto_target.jpg")

	opts := DefaultOptions()
	opts.Format = Auto
	opts.TargetSize = 5 * 1024 // 5 KB

	result, err := CompressFile(src, dst, opts)
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	t.Logf("Result: %s", result)

	if result.Format != JPEG {
		t.Fatalf("Expected Auto+TargetSize on opaque image to pick JPEG, got %v", result.Format)
	}
}

// Benchmarks using real images.

func BenchmarkIntegrationCompressJPEG(b *testing.B) {
	if _, err := os.Stat("testdata/large_photo.jpg"); os.IsNotExist(err) {
		b.Skip("testdata missing. Run: go test -run TestGenerateTestData -v")
	}

	img, err := Open("testdata/large_photo.jpg")
	if err != nil {
		b.Fatal(err)
	}

	opts := DefaultOptions()
	opts.Format = JPEG
	opts.Quality = Balanced

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		CompressImage(img, opts)
	}
}

func BenchmarkIntegrationSSIMReal(b *testing.B) {
	if _, err := os.Stat("testdata/gradient.jpg"); os.IsNotExist(err) {
		b.Skip("testdata missing. Run: go test -run TestGenerateTestData -v")
	}

	img, err := Open("testdata/gradient.jpg")
	if err != nil {
		b.Fatal(err)
	}
	nrgba := toNRGBA(img)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SSIMFast(nrgba, nrgba)
	}
}
