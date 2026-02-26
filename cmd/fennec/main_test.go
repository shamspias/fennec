package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests require the binary to be built first: go build -o bin/fennec ./cmd/fennec
// Or just run: make build && go test ./cmd/fennec -v

func fennecBin(t *testing.T) string {
	t.Helper()
	// Look for the binary relative to the project root.
	candidates := []string{
		"../../bin/fennec",
		filepath.Join(os.Getenv("GOPATH"), "bin", "fennec"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}

	// Try building it.
	out := filepath.Join(t.TempDir(), "fennec")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = "."
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot build fennec binary: %v", err)
	}
	return out
}

func ensureFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("Fixture %s not found. Run: make fixtures", name)
	}
	abs, _ := filepath.Abs(path)
	return abs
}

func TestCLINoArgs(t *testing.T) {
	bin := fennecBin(t)
	cmd := exec.Command(bin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Expected non-zero exit with no args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Fatalf("Expected usage message, got: %s", out)
	}
}

func TestCLIAnalyze(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")

	cmd := exec.Command(bin, "-analyze", src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec -analyze failed: %v\n%s", err, out)
	}

	output := string(out)
	for _, want := range []string{"Dimensions", "Entropy", "Recommended"} {
		if !strings.Contains(output, want) {
			t.Errorf("Expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestCLICompressJPEG(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")
	dst := filepath.Join(t.TempDir(), "out.jpg")

	cmd := exec.Command(bin, "-quality", "balanced", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec compress failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "Fennec Result") {
		t.Fatalf("Expected result output, got: %s", out)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("Output file is empty")
	}
}

func TestCLICompressPNG(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "fewcolors.png")
	dst := filepath.Join(t.TempDir(), "out.png")

	cmd := exec.Command(bin, src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec compress failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "PNG") {
		t.Fatalf("Expected PNG format in output, got: %s", out)
	}
}

func TestCLITargetSize(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")
	dst := filepath.Join(t.TempDir(), "small.jpg")

	cmd := exec.Command(bin, "-target-size", "5KB", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec target-size failed: %v\n%s", err, out)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Output not created: %v", err)
	}
	// Should be reasonably close to 5KB.
	if info.Size() > 15*1024 {
		t.Fatalf("Output %d bytes, way over 5KB target", info.Size())
	}
}

func TestCLIMaxWidth(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")
	dst := filepath.Join(t.TempDir(), "resized.jpg")

	cmd := exec.Command(bin, "-max-width", "200", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec max-width failed: %v\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "200x") {
		t.Fatalf("Expected 200px width in output, got: %s", output)
	}
}

func TestCLIAutoOutput(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")

	// Copy fixture to temp dir so the auto-named output goes there too.
	tmpDir := t.TempDir()
	tmpSrc := filepath.Join(tmpDir, "photo.jpg")
	data, _ := os.ReadFile(src)
	os.WriteFile(tmpSrc, data, 0644)

	cmd := exec.Command(bin, tmpSrc)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fennec auto-output failed: %v\n%s", err, out)
	}

	// Should create photo_compressed.jpg.
	expected := filepath.Join(tmpDir, "photo_compressed.jpg")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Fatalf("Expected auto-named output %s not found", expected)
	}
}

func TestCLIInvalidQuality(t *testing.T) {
	bin := fennecBin(t)
	src := ensureFixture(t, "gradient.jpg")

	cmd := exec.Command(bin, "-quality", "potato", src, "/dev/null")
	_, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Expected error for invalid quality")
	}
}

func TestCLIBadInput(t *testing.T) {
	bin := fennecBin(t)
	cmd := exec.Command(bin, "/nonexistent/file.jpg", "/dev/null")
	_, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Expected error for nonexistent input")
	}
}

func TestCLIParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"100KB", 100 * 1024},
		{"2MB", 2 * 1024 * 1024},
		{"500B", 500},
		{"1.5MB", int(1.5 * 1024 * 1024)},
	}

	for _, tt := range tests {
		got, err := parseSize(tt.input)
		if err != nil {
			t.Errorf("parseSize(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
