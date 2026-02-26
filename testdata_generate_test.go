package fennec

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateTestData creates test fixture images if they don't exist.
// Run with: go test -run TestGenerateTestData -v
func TestGenerateTestData(t *testing.T) {
	dir := "testdata"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	// 1. Gradient JPEG — simulates a photograph.
	genIfMissing(t, filepath.Join(dir, "gradient.jpg"), func(path string) {
		img := image.NewNRGBA(image.Rect(0, 0, 400, 300))
		for y := 0; y < 300; y++ {
			for x := 0; x < 400; x++ {
				off := y*img.Stride + x*4
				img.Pix[off] = uint8(x * 255 / 400)
				img.Pix[off+1] = uint8(y * 255 / 300)
				img.Pix[off+2] = uint8((x + y) % 256)
				img.Pix[off+3] = 0xff
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
			t.Fatal(err)
		}
		t.Logf("Generated %s", path)
	})

	// 2. Transparent PNG — logo/icon simulation.
	genIfMissing(t, filepath.Join(dir, "transparent.png"), func(path string) {
		img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				off := y*img.Stride + x*4
				// Circle with transparency.
				cx, cy := x-100, y-100
				dist := cx*cx + cy*cy
				if dist < 80*80 {
					img.Pix[off] = 0x33
					img.Pix[off+1] = 0x99
					img.Pix[off+2] = 0xff
					img.Pix[off+3] = 0xff
				} else if dist < 90*90 {
					img.Pix[off] = 0x33
					img.Pix[off+1] = 0x99
					img.Pix[off+2] = 0xff
					img.Pix[off+3] = uint8(255 * (90*90 - dist) / (90*90 - 80*80))
				}
				// else: fully transparent (zero-initialized)
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("Generated %s", path)
	})

	// 3. Few-color PNG — screenshot/diagram simulation.
	genIfMissing(t, filepath.Join(dir, "fewcolors.png"), func(path string) {
		img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
		colors := []color.NRGBA{
			{0xff, 0xff, 0xff, 0xff}, // white
			{0x33, 0x33, 0x33, 0xff}, // dark gray
			{0x00, 0x66, 0xcc, 0xff}, // blue
			{0xcc, 0x00, 0x00, 0xff}, // red
		}
		for y := 0; y < 200; y++ {
			for x := 0; x < 300; x++ {
				off := y*img.Stride + x*4
				c := colors[(y/50+x/75)%len(colors)]
				img.Pix[off] = c.R
				img.Pix[off+1] = c.G
				img.Pix[off+2] = c.B
				img.Pix[off+3] = c.A
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("Generated %s", path)
	})

	// 4. Large photo-like JPEG for benchmarking.
	genIfMissing(t, filepath.Join(dir, "large_photo.jpg"), func(path string) {
		img := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
		for y := 0; y < 1080; y++ {
			for x := 0; x < 1920; x++ {
				off := y*img.Stride + x*4
				// Simulate noisy photo with gradients.
				img.Pix[off] = uint8((x*y + x*3) % 256)
				img.Pix[off+1] = uint8((x*y + y*7) % 256)
				img.Pix[off+2] = uint8((x + y*11) % 256)
				img.Pix[off+3] = 0xff
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 92}); err != nil {
			t.Fatal(err)
		}
		t.Logf("Generated %s", path)
	})

	// 5. Grayscale PNG.
	genIfMissing(t, filepath.Join(dir, "grayscale.png"), func(path string) {
		img := image.NewGray(image.Rect(0, 0, 200, 200))
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				img.Pix[y*img.Stride+x] = uint8(x * 255 / 200)
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("Generated %s", path)
	})
}

func genIfMissing(t *testing.T, path string, gen func(string)) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Logf("Exists: %s", path)
		return
	}
	gen(path)
}
