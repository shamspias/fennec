package fennec

import (
	"image"
	"math"
)

// ImageStats contains analysis results for an image.
type ImageStats struct {
	// Width and Height in pixels.
	Width, Height int

	// HasAlpha indicates the image uses transparency.
	HasAlpha bool

	// IsGrayscale indicates all pixels have R == G == B.
	IsGrayscale bool

	// UniqueColors is the number of distinct colors (sampled for large images).
	UniqueColors int

	// Entropy measures information density (0-8 bits per channel).
	// Low entropy = highly compressible, high entropy = complex/noisy.
	Entropy float64

	// EdgeDensity measures the proportion of edge pixels (0-1).
	// High edge density = text/diagrams, low = photographs.
	EdgeDensity float64

	// MeanBrightness is the average luminance (0-255).
	MeanBrightness float64

	// Contrast is the standard deviation of luminance (0-127.5).
	Contrast float64

	// RecommendedFormat based on the analysis.
	RecommendedFormat Format

	// RecommendedQuality based on the analysis.
	RecommendedQuality Quality

	// EstimatedCompression is the estimated achievable compression ratio.
	EstimatedCompression float64
}

// Analyze performs comprehensive image analysis to inform compression decisions.
func Analyze(img image.Image) ImageStats {
	src := toNRGBA(img)
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

	// Compute contrast (std dev of luminance).
	var varianceSum float64
	mean := stats.MeanBrightness
	for y := 0; y < h; y += int(math.Max(1, float64(h)/100)) {
		off := y * src.Stride
		for x := 0; x < w; x += int(math.Max(1, float64(w)/100)) {
			i := off + x*4
			lum := 0.299*float64(src.Pix[i]) + 0.587*float64(src.Pix[i+1]) + 0.114*float64(src.Pix[i+2])
			d := lum - mean
			varianceSum += d * d
		}
	}
	sampleCount := float64(int(math.Max(1, float64(h)/100))) * float64(int(math.Max(1, float64(w)/100)))
	if sampleCount == 0 {
		sampleCount = 1
	}
	// Approximate sample count
	sampledH := 0
	for y := 0; y < h; y += int(math.Max(1, float64(h)/100)) {
		sampledH++
	}
	sampledW := 0
	for x := 0; x < w; x += int(math.Max(1, float64(w)/100)) {
		sampledW++
	}
	sampleCount = float64(sampledH * sampledW)
	stats.Contrast = math.Sqrt(varianceSum / sampleCount)

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
// Returns the fraction of pixels that are edge pixels (0-1).
func computeEdgeDensity(img *image.NRGBA) float64 {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	if w < 3 || h < 3 {
		return 0
	}

	// Sample for performance.
	stepX := int(math.Max(1, float64(w)/200))
	stepY := int(math.Max(1, float64(h)/200))

	edgeCount := 0
	totalCount := 0
	threshold := 30.0 // Sobel magnitude threshold for edge detection.

	for y := 1; y < h-1; y += stepY {
		for x := 1; x < w-1; x += stepX {
			// Sobel X kernel: [-1 0 1; -2 0 2; -1 0 1]
			// Sobel Y kernel: [-1 -2 -1; 0 0 0; 1 2 1]
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
		// Screenshots, text, diagrams — PNG compresses better.
		return PNG
	}
	return JPEG
}

func recommendQuality(stats ImageStats) Quality {
	// High entropy, low edge density = photographs → aggressive compression works.
	if stats.Entropy > 6 && stats.EdgeDensity < 0.15 {
		return Balanced
	}
	// Low entropy = simple images → can compress more aggressively.
	if stats.Entropy < 4 {
		return Aggressive
	}
	// High edge density = text/diagrams → need higher quality.
	if stats.EdgeDensity > 0.25 {
		return High
	}
	return Balanced
}

func estimateCompression(stats ImageStats) float64 {
	// Rough estimate based on image characteristics.
	if stats.RecommendedFormat == PNG {
		if stats.UniqueColors <= 256 {
			return 5.0 + (256-float64(stats.UniqueColors))/50
		}
		if stats.IsGrayscale {
			return 3.0
		}
		return 2.0
	}

	// JPEG estimate.
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
