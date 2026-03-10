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

func TestGenerateTestData(t *testing.T) {
	dir := "testdata"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	genIfMissing(t, filepath.Join(dir, "gradient.jpg"), generateGradient)
	genIfMissing(t, filepath.Join(dir, "transparent.png"), generateTransparent)
	genIfMissing(t, filepath.Join(dir, "fewcolors.png"), generateFewColors)
	genIfMissing(t, filepath.Join(dir, "large_photo.jpg"), generateLargePhoto)
	genIfMissing(t, filepath.Join(dir, "grayscale.png"), generateGrayscale)
}

func generateGradient(path string) {
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
	saveJPEG(path, img, 95)
}

func generateTransparent(path string) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			off := y*img.Stride + x*4
			dist := (x-100)*(x-100) + (y-100)*(y-100)
			if dist < 80*80 {
				copy(img.Pix[off:off+4], []uint8{0x33, 0x99, 0xff, 0xff})
			} else if dist < 90*90 {
				alpha := uint8(255 * (90*90 - dist) / (90*90 - 80*80))
				copy(img.Pix[off:off+4], []uint8{0x33, 0x99, 0xff, alpha})
			}
		}
	}
	savePNG(path, img)
}

func generateFewColors(path string) {
	img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	colors := []color.NRGBA{{0xff, 0xff, 0xff, 0xff}, {0x33, 0x33, 0x33, 0xff}, {0x00, 0x66, 0xcc, 0xff}, {0xcc, 0x00, 0x00, 0xff}}
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			c := colors[(y/50+x/75)%len(colors)]
			off := y*img.Stride + x*4
			img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = c.R, c.G, c.B, c.A
		}
	}
	savePNG(path, img)
}

func generateLargePhoto(path string) {
	img := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			off := y*img.Stride + x*4
			img.Pix[off] = uint8((x*y + x*3) % 256)
			img.Pix[off+1] = uint8((x*y + y*7) % 256)
			img.Pix[off+2] = uint8((x + y*11) % 256)
			img.Pix[off+3] = 0xff
		}
	}
	saveJPEG(path, img, 92)
}

func generateGrayscale(path string) {
	img := image.NewGray(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Pix[y*img.Stride+x] = uint8(x * 255 / 200)
		}
	}
	savePNG(path, img)
}

func saveJPEG(path string, img image.Image, q int) {
	f, _ := os.Create(path)
	defer f.Close()
	jpeg.Encode(f, img, &jpeg.Options{Quality: q})
}

func savePNG(path string, img image.Image) {
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
}

func genIfMissing(t *testing.T, path string, gen func(string)) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Logf("Exists: %s", path)
		return
	}
	gen(path)
}
