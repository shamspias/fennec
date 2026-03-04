package fennec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ensureTestdata skips the test if fixture images don't exist.
func ensureTestdata(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("testdata/gradient.jpg"); os.IsNotExist(err) {
		t.Skip("Testdata missing. Run: make fixtures")
	}
}

// \u2500\u2500 Integration Tests \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500

func TestIntegrationCompressFileJPEG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "output.jpg")

	result, err := CompressFile(context.Background(), "testdata/gradient.jpg", dst, DefaultOptions())
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	if result.Format != JPEG {
		t.Fatalf("expected JPEG, got %v", result.Format)
	}
	if result.SSIM < 0.90 {
		t.Fatalf("SSIM too low: %f", result.SSIM)
	}
	if result.OriginalSize == 0 {
		t.Fatal("OriginalSize should be populated from file")
	}
	if result.CompressedSize == 0 {
		t.Fatal("CompressedSize should be populated")
	}
	if result.SavingsPercent <= 0 {
		t.Logf("Warning: no savings for gradient fixture (%.1f%%)", result.SavingsPercent)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestIntegrationCompressFilePNG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "output.png")

	opts := DefaultOptions()
	opts.Format = PNG

	result, err := CompressFile(context.Background(), "testdata/transparent.png", dst, opts)
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	if result.Format != PNG {
		t.Fatalf("expected PNG, got %v", result.Format)
	}
	if result.SSIM != 1.0 {
		t.Fatalf("PNG should be lossless, SSIM: %f", result.SSIM)
	}
}

func TestIntegrationCompressFewColorsPNG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "output.png")

	result, err := CompressFile(context.Background(), "testdata/fewcolors.png", dst, DefaultOptions())
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	// Few-color images should be auto-detected as PNG.
	if result.Format != PNG {
		t.Fatalf("expected PNG for few-color image, got %v", result.Format)
	}
}

func TestIntegrationResizeAndCompress(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "resized.jpg")

	opts := DefaultOptions()
	opts.MaxWidth = 200
	opts.MaxHeight = 200

	result, err := CompressFile(context.Background(), "testdata/gradient.jpg", dst, opts)
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	if result.FinalDimensions.X > 200 || result.FinalDimensions.Y > 200 {
		t.Fatalf("should fit in 200x200, got %dx%d", result.FinalDimensions.X, result.FinalDimensions.Y)
	}
}

func TestIntegrationTargetSize(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "small.jpg")

	opts := DefaultOptions()
	opts.TargetSize = 5 * 1024 // 5 KB

	result, err := CompressFile(context.Background(), "testdata/gradient.jpg", dst, opts)
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("output not created: %v", err)
	}
	// Allow 3x overshoot for very constrained targets on small images.
	if info.Size() > int64(opts.TargetSize)*3 {
		t.Fatalf("output %d bytes, target was %d", info.Size(), opts.TargetSize)
	}
	_ = result
}

func TestIntegrationAnalyzeFile(t *testing.T) {
	ensureTestdata(t)

	img, err := OpenAndOrient("testdata/gradient.jpg")
	if err != nil {
		t.Fatalf("OpenAndOrient: %v", err)
	}

	stats := Analyze(img)
	if stats.Width == 0 || stats.Height == 0 {
		t.Fatal("analyze should return non-zero dimensions")
	}
	if stats.RecommendedFormat != JPEG {
		t.Fatalf("expected JPEG recommendation for gradient, got %v", stats.RecommendedFormat)
	}
}

func TestIntegrationGrayscalePNG(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "output.png")

	opts := DefaultOptions()
	opts.Format = PNG

	result, err := CompressFile(context.Background(), "testdata/grayscale.png", dst, opts)
	if err != nil {
		t.Fatalf("CompressFile: %v", err)
	}

	if result.Format != PNG {
		t.Fatalf("expected PNG, got %v", result.Format)
	}
}

func TestIntegrationSaveAndReload(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	// Open, compress, save, then reopen and verify.
	img, err := Open("testdata/gradient.jpg")
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmpDir, "saved.jpg")
	if err := Save(img, outPath, DefaultOptions()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	bounds := reloaded.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Fatal("reloaded image has zero dimensions")
	}
}

func TestIntegrationBatchCompress(t *testing.T) {
	ensureTestdata(t)
	tmpDir := t.TempDir()

	items := []BatchItem{
		{
			Src: "testdata/gradient.jpg",
			Dst: filepath.Join(tmpDir, "batch_1.jpg"),
		},
		{
			Src: "testdata/fewcolors.png",
			Dst: filepath.Join(tmpDir, "batch_2.png"),
		},
	}

	results := CompressBatch(context.Background(), items, BatchOptions{
		Workers:     2,
		DefaultOpts: DefaultOptions(),
	})

	summary := Summarize(results)
	if summary.Succeeded != 2 {
		for _, r := range results {
			if r.Err != nil {
				t.Logf("Item %d error: %v", r.Index, r.Err)
			}
		}
		t.Fatalf("expected 2 succeeded, got %d (failed: %d)", summary.Succeeded, summary.Failed)
	}
}
