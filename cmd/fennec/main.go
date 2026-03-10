package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shamspias/fennec"
)

// parseSize parses a human-readable size string like "100KB", "2MB", or raw bytes "51200".
func parseSize(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}

	upper := strings.ToUpper(s)

	multipliers := []struct {
		suffix string
		mult   int
	}{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(upper, m.suffix) {
			numStr := strings.TrimSpace(s[:len(s)-len(m.suffix)])
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", s, err)
			}
			return int(val * float64(m.mult)), nil
		}
	}

	// Fallback: try parsing as raw integer (bytes).
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: expected number or value like 100KB, 2MB", s)
	}
	return val, nil
}

func main() {
	quality := flag.String("quality", "balanced", "Quality preset: lossless, ultra, high, balanced, aggressive, maximum")
	format := flag.String("format", "auto", "Output format: auto, jpeg, png")
	maxWidth := flag.Int("max-width", 0, "Maximum output width (0 = no constraint)")
	maxHeight := flag.Int("max-height", 0, "Maximum output height (0 = no constraint)")
	targetSize := flag.String("target-size", "", "Target file size (e.g. 100KB, 2MB, or raw bytes)")
	ssimTarget := flag.Float64("ssim", 0, "Custom SSIM target (0.0-1.0, overrides quality preset)")
	noOrient := flag.Bool("no-orient", false, "Don't auto-rotate based on EXIF orientation")
	analyze := flag.Bool("analyze", false, "Analyze image without compressing")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: fennec [options] <input> [output]\n")
		fmt.Fprintf(os.Stderr, "  If output is omitted, uses <input>_fennec.<ext>\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	input := args[0]

	if *analyze {
		img, err := fennec.Open(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		stats := fennec.Analyze(img)
		fmt.Printf("Image Analysis: %s\n", input)
		fmt.Printf("  Dimensions:     %d x %d\n", stats.Width, stats.Height)
		fmt.Printf("  Has Alpha:      %v\n", stats.HasAlpha)
		fmt.Printf("  Grayscale:      %v\n", stats.IsGrayscale)
		fmt.Printf("  Unique Colors:  %d\n", stats.UniqueColors)
		fmt.Printf("  Entropy:        %.2f bits\n", stats.Entropy)
		fmt.Printf("  Edge Density:   %.2f%%\n", stats.EdgeDensity*100)
		fmt.Printf("  Mean Bright:    %.1f\n", stats.MeanBrightness)
		fmt.Printf("  Contrast:       %.1f\n", stats.Contrast)
		fmt.Printf("  Recommended:    %s / %s\n", stats.RecommendedFormat, stats.RecommendedQuality)
		fmt.Printf("  Est. Compress:  %.1fx\n", stats.EstimatedCompression)
		return
	}

	output := ""
	if len(args) >= 2 {
		output = args[1]
	} else {
		ext := ".jpg"
		base := strings.TrimSuffix(input, ".jpg")
		base = strings.TrimSuffix(base, ".jpeg")
		base = strings.TrimSuffix(base, ".png")
		output = base + "_fennec" + ext
	}

	opts := fennec.DefaultOptions()
	opts.MaxWidth = *maxWidth
	opts.MaxHeight = *maxHeight

	// Parse target size (supports human-readable strings).
	if *targetSize != "" {
		ts, err := parseSize(*targetSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid target-size: %v\n", err)
			os.Exit(1)
		}
		opts.TargetSize = ts
	}

	// Apply custom SSIM target (overrides quality preset).
	if *ssimTarget > 0 {
		if *ssimTarget < 0 || *ssimTarget > 1.0 {
			fmt.Fprintf(os.Stderr, "Invalid SSIM target: must be between 0.0 and 1.0\n")
			os.Exit(1)
		}
		opts.TargetSSIM = *ssimTarget
	}

	// Apply no-orient flag.
	if *noOrient {
		opts.AutoOrient = false
	}

	switch strings.ToLower(*quality) {
	case "lossless":
		opts.Quality = fennec.Lossless
	case "ultra":
		opts.Quality = fennec.Ultra
	case "high":
		opts.Quality = fennec.High
	case "balanced":
		opts.Quality = fennec.Balanced
	case "aggressive":
		opts.Quality = fennec.Aggressive
	case "maximum", "max":
		opts.Quality = fennec.Maximum
	default:
		fmt.Fprintf(os.Stderr, "Unknown quality: %s\n", *quality)
		os.Exit(1)
	}

	switch strings.ToLower(*format) {
	case "auto":
		opts.Format = fennec.Auto
	case "jpeg", "jpg":
		opts.Format = fennec.JPEG
	case "png":
		opts.Format = fennec.PNG
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *format)
		os.Exit(1)
	}

	if *verbose {
		opts.OnProgress = func(stage fennec.ProgressStage, pct float64) error {
			fmt.Fprintf(os.Stderr, "  [%s] %.0f%%\n", stage, pct*100)
			return nil
		}
	}

	start := time.Now()
	ctx := context.Background()

	result, err := fennec.CompressFile(ctx, input, output, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	if *verbose {
		fmt.Println(result)
		fmt.Printf("  Time: %v\n", elapsed.Round(time.Millisecond))
	} else {
		fmt.Printf("%s -> %s | %s | SSIM: %.4f | Saved: %.1f%% | %v\n",
			input, output, result.Format,
			result.SSIM, result.SavingsPercent,
			elapsed.Round(time.Millisecond))
	}
}
