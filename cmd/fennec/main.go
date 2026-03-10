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

type appConfig struct {
	quality, format, targetSize string
	maxWidth, maxHeight         int
	ssimTarget                  float64
	noOrient, analyze, verbose  bool
	input, output               string
}

func main() {
	cfg := parseFlags()
	if cfg.analyze {
		runAnalyze(cfg.input)
		return
	}
	runCompression(cfg)
}

func parseFlags() appConfig {
	cfg := appConfig{}
	flag.StringVar(&cfg.quality, "quality", "balanced", "Quality preset")
	flag.StringVar(&cfg.format, "format", "auto", "Output format")
	flag.IntVar(&cfg.maxWidth, "max-width", 0, "Max width")
	flag.IntVar(&cfg.maxHeight, "max-height", 0, "Max height")
	flag.StringVar(&cfg.targetSize, "target-size", "", "Target file size")
	flag.Float64Var(&cfg.ssimTarget, "ssim", 0, "Custom SSIM target")
	flag.BoolVar(&cfg.noOrient, "no-orient", false, "Don't auto-rotate")
	flag.BoolVar(&cfg.analyze, "analyze", false, "Analyze image")
	flag.BoolVar(&cfg.verbose, "v", false, "Verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: fennec [options] <input> [output]\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	cfg.input = args[0]
	if len(args) >= 2 {
		cfg.output = args[1]
	} else {
		base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(cfg.input, ".jpg"), ".jpeg"), ".png")
		cfg.output = base + "_fennec.jpg"
	}
	return cfg
}

func runAnalyze(input string) {
	img, err := fennec.Open(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	stats := fennec.Analyze(img)
	fmt.Printf("Image Analysis: %s\n", input)
	// Fixed the Printf arguments to include stats.UniqueColors
	fmt.Printf("  Dimensions:     %d x %d\n  Has Alpha:      %v\n  Grayscale:      %v\n  Unique Colors:  %d\n", stats.Width, stats.Height, stats.HasAlpha, stats.IsGrayscale, stats.UniqueColors)
	fmt.Printf("  Entropy:        %.2f bits\n  Edge Density:   %.2f%%\n", stats.Entropy, stats.EdgeDensity*100)
	fmt.Printf("  Recommended:    %s / %s\n", stats.RecommendedFormat, stats.RecommendedQuality)
}

func runCompression(cfg appConfig) {
	opts := buildOptions(cfg)
	start := time.Now()
	result, err := fennec.CompressFile(context.Background(), cfg.input, cfg.output, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start).Round(time.Millisecond)

	if cfg.verbose {
		fmt.Printf("%v\n  Time: %v\n", result, elapsed)
	} else {
		fmt.Printf("%s -> %s | %s | SSIM: %.4f | Saved: %.1f%% | %v\n", cfg.input, cfg.output, result.Format, result.SSIM, result.SavingsPercent, elapsed)
	}
}

func buildOptions(cfg appConfig) fennec.Options {
	opts := fennec.DefaultOptions()
	opts.MaxWidth, opts.MaxHeight = cfg.maxWidth, cfg.maxHeight
	if cfg.noOrient {
		opts.AutoOrient = false
	}
	if cfg.ssimTarget > 0 {
		if cfg.ssimTarget > 1.0 {
			os.Exit(1)
		}
		opts.TargetSSIM = cfg.ssimTarget
	}
	if cfg.targetSize != "" {
		ts, err := parseSize(cfg.targetSize)
		if err != nil {
			os.Exit(1)
		}
		opts.TargetSize = ts
	}
	opts.Quality, opts.Format = parseQuality(cfg.quality), parseFormat(cfg.format)
	if cfg.verbose {
		opts.OnProgress = func(stage fennec.ProgressStage, pct float64) error {
			fmt.Fprintf(os.Stderr, "  [%s] %.0f%%\n", stage, pct*100)
			return nil
		}
	}
	return opts
}

func parseQuality(q string) fennec.Quality {
	switch strings.ToLower(q) {
	case "lossless":
		return fennec.Lossless
	case "ultra":
		return fennec.Ultra
	case "high":
		return fennec.High
	case "aggressive":
		return fennec.Aggressive
	case "maximum", "max":
		return fennec.Maximum
	default:
		return fennec.Balanced
	}
}

func parseFormat(f string) fennec.Format {
	switch strings.ToLower(f) {
	case "jpeg", "jpg":
		return fennec.JPEG
	case "png":
		return fennec.PNG
	default:
		return fennec.Auto
	}
}
