package main

import (
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "fennec")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binary
}

func createTestJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(i % 256)
		img.Pix[i+1] = uint8((i * 3) % 256)
		img.Pix[i+2] = uint8((i * 7) % 256)
		img.Pix[i+3] = 0xff
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
}

func createTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0x33
		img.Pix[i+1] = 0x66
		img.Pix[i+2] = 0x99
		img.Pix[i+3] = uint8(i % 256)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestCLIBasicCompression(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	dst := filepath.Join(tmpDir, "output.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Fatal("Output file not created")
	}
}

func TestCLIAnalyze(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, "-analyze", src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI analyze failed: %v\n%s", err, out)
	}
	output := string(out)
	if len(output) == 0 {
		t.Fatal("No analyze output")
	}
}

func TestCLIQualityPresets(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	createTestJPEG(t, src)

	presets := []string{"ultra", "high", "balanced", "aggressive", "maximum"}
	for _, preset := range presets {
		t.Run(preset, func(t *testing.T) {
			dst := filepath.Join(tmpDir, "out_"+preset+".jpg")
			cmd := exec.Command(binary, "-quality", preset, src, dst)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("CLI failed with quality=%s: %v\n%s", preset, err, out)
			}
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				t.Fatalf("Output not created for quality=%s", preset)
			}
		})
	}
}

func TestCLIFormatPNG(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.png")
	dst := filepath.Join(tmpDir, "output.png")
	createTestPNG(t, src)

	cmd := exec.Command(binary, "-format", "png", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI PNG failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Fatal("PNG output not created")
	}
}

func TestCLIMaxDimensions(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	dst := filepath.Join(tmpDir, "output.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, "-max-width", "100", "-max-height", "100", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI max-dim failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Fatal("Output not created")
	}
}

func TestCLITargetSize(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	dst := filepath.Join(tmpDir, "output.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, "-target-size", "5000", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI target-size failed: %v\n%s", err, out)
	}
	info, err := os.Stat(dst)
	if os.IsNotExist(err) {
		t.Fatal("Output not created")
	}
	if info.Size() > 6000 {
		t.Logf("Warning: output size %d exceeds target by more than 20%%", info.Size())
	}
}

func TestCLINoArgs(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary)
	err := cmd.Run()
	if err == nil {
		t.Fatal("Expected error with no args")
	}
}

func TestCLIInvalidInput(t *testing.T) {
	binary := buildBinary(t)
	cmd := exec.Command(binary, "nonexistent.jpg", "out.jpg")
	err := cmd.Run()
	if err == nil {
		t.Fatal("Expected error with invalid input")
	}
}

func TestCLIVerbose(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	dst := filepath.Join(tmpDir, "output.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, "-v", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI verbose failed: %v\n%s", err, out)
	}
}

func TestCLIAutoOutput(t *testing.T) {
	binary := buildBinary(t)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "input.jpg")
	createTestJPEG(t, src)

	cmd := exec.Command(binary, src)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI auto-output failed: %v\n%s", err, out)
	}
}
