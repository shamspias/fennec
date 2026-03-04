package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shamspias/fennec"
)

func main() {
	quality := flag.String("quality", "balanced", "Quality preset: lossless, ultra, high, balanced, aggressive, maximum")
	format := flag.String("format", "auto", "Output format: auto, jpeg, png")
	maxWidth := flag.Int("max-width", 0, "Maximum output width (0 = no constraint)")
	maxHeight := flag.Int("max-height", 0, "Maximum output height (0 = no constraint)")
	targetSize := flag.Int("target-size", 0, "Target file size in bytes (0 = disabled)")
	analyze := flag.Bool("analyze", false, "Analyze image without compressing")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: fennec [options] <input> [output]\n")
		fmt.Fprintf(os.Stderr, "  If output is omitted, uses <input>_fennec.<ext>\n")
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
	opts.TargetSize = *targetSize

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
