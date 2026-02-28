package fennec

import (
	"image"
	"math"
)

// ImageStats contains analysis results for an image.
type ImageStats struct {
	Width, Height  int
	HasAlpha       bool
	IsGrayscale    bool
	UniqueColors   int
	Entropy        float64
	EdgeDensity    float64
	MeanBrightness float64
	Contrast       float64

	RecommendedFormat    Format
	RecommendedQuality   Quality
	EstimatedCompression float64
}

// Analyze performs comprehensive image analysis to inform compression decisions.
// Uses toNRGBARef for zero-copy when the input is already NRGBA.
func Analyze(img image.Image) ImageStats {
	src := toNRGBARef(img) // Phase 1 fix: no copy for read-only path.
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()

	stats := ImageStats{
		Width:  w,
		Height: h,
	}

	if w == 0 || h == 0 {
		return stats
	}

	// Single pass: collect color info, brightness, alpha.
	histogram := [256]float64{}
	var brightSum float64
	colorSet := make(map[uint32]struct{})
	maxSample := 50000
	step := 1
	if w*h > maxSample {
		step = w * h / maxSample
	}

	allGray := true
	hasAlpha := false
	idx := 0

	for y := 0; y < h; y++ {
		off := y * src.Stride
		for x := 0; x < w; x++ {
			i := off + x*4
			r := src.Pix[i]
			g := src.Pix[i+1]
			b := src.Pix[i+2]
			a := src.Pix[i+3]

			lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
			brightSum += lum
			histogram[int(lum+0.5)]++

			if a < 255 {
				hasAlpha = true
			}
			if r != g || g != b {
				allGray = false
			}
			if idx%step == 0 && len(colorSet) < 1024 {
				key := uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | uint32(a)
				colorSet[key] = struct{}{}
			}
			idx++
		}
	}

	n := float64(w * h)
	stats.HasAlpha = hasAlpha
	stats.IsGrayscale = allGray
	stats.UniqueColors = len(colorSet)
	stats.MeanBrightness = brightSum / n

	// Phase 1 fix: compute contrast with consistent sampling.
	// Use a fixed grid approach so the sample count matches the actual iterations.
	stepY := int(math.Max(1, math.Ceil(float64(h)/100)))
	stepX := int(math.Max(1, math.Ceil(float64(w)/100)))

	var varianceSum float64
	var sampleCount int
	mean := stats.MeanBrightness

	for y := 0; y < h; y += stepY {
		off := y * src.Stride
		for x := 0; x < w; x += stepX {
			i := off + x*4
			lum := 0.299*float64(src.Pix[i]) + 0.587*float64(src.Pix[i+1]) + 0.114*float64(src.Pix[i+2])
			d := lum - mean
			varianceSum += d * d
			sampleCount++
		}
	}
	if sampleCount > 0 {
		stats.Contrast = math.Sqrt(varianceSum / float64(sampleCount))
	}

	// Compute entropy from luminance histogram.
	stats.Entropy = computeEntropy(histogram[:], n)

	// Compute edge density using Sobel operator (sampled).
	stats.EdgeDensity = computeEdgeDensity(src)

	// Make recommendations.
	stats.RecommendedFormat = recommendFormat(stats)
	stats.RecommendedQuality = recommendQuality(stats)
	stats.EstimatedCompression = estimateCompression(stats)

	return stats
}

// computeEntropy calculates Shannon entropy from a histogram.
func computeEntropy(histogram []float64, total float64) float64 {
	if total == 0 {
		return 0
	}
	var entropy float64
	for _, count := range histogram {
		if count > 0 {
			p := count / total
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// computeEdgeDensity uses a Sobel operator to detect edges.
func computeEdgeDensity(img *image.NRGBA) float64 {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	if w < 3 || h < 3 {
		return 0
	}

	stepX := int(math.Max(1, float64(w)/200))
	stepY := int(math.Max(1, float64(h)/200))

	edgeCount := 0
	totalCount := 0
	threshold := 30.0

	for y := 1; y < h-1; y += stepY {
		for x := 1; x < w-1; x += stepX {
			gx := sobelLum(img, x+1, y-1) - sobelLum(img, x-1, y-1) +
				2*sobelLum(img, x+1, y) - 2*sobelLum(img, x-1, y) +
				sobelLum(img, x+1, y+1) - sobelLum(img, x-1, y+1)

			gy := sobelLum(img, x-1, y+1) - sobelLum(img, x-1, y-1) +
				2*sobelLum(img, x, y+1) - 2*sobelLum(img, x, y-1) +
				sobelLum(img, x+1, y+1) - sobelLum(img, x+1, y-1)

			mag := math.Sqrt(gx*gx + gy*gy)
			if mag > threshold {
				edgeCount++
			}
			totalCount++
		}
	}

	if totalCount == 0 {
		return 0
	}
	return float64(edgeCount) / float64(totalCount)
}

func sobelLum(img *image.NRGBA, x, y int) float64 {
	off := y*img.Stride + x*4
	return 0.299*float64(img.Pix[off]) + 0.587*float64(img.Pix[off+1]) + 0.114*float64(img.Pix[off+2])
}

func recommendFormat(stats ImageStats) Format {
	if stats.HasAlpha {
		return PNG
	}
	if stats.UniqueColors <= 256 {
		return PNG
	}
	if stats.EdgeDensity > 0.3 && stats.UniqueColors < 1000 {
		return PNG
	}
	return JPEG
}

func recommendQuality(stats ImageStats) Quality {
	if stats.Entropy > 6 && stats.EdgeDensity < 0.15 {
		return Balanced
	}
	if stats.Entropy < 4 {
		return Aggressive
	}
	if stats.EdgeDensity > 0.25 {
		return High
	}
	return Balanced
}

func estimateCompression(stats ImageStats) float64 {
	if stats.RecommendedFormat == PNG {
		if stats.UniqueColors <= 256 {
			return 5.0 + (256-float64(stats.UniqueColors))/50
		}
		if stats.IsGrayscale {
			return 3.0
		}
		return 2.0
	}

	base := 10.0
	if stats.Entropy > 7 {
		base = 5.0
	} else if stats.Entropy > 5 {
		base = 8.0
	}
	if stats.EdgeDensity > 0.2 {
		base *= 0.7
	}
	return base
}
