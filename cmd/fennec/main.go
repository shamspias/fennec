// Command fennec is a CLI tool for intelligent image compression.
//
// Usage:
//
//	fennec [flags] <input> [output]
//	fennec -analyze <input>
//
// Examples:
//
//	fennec photo.jpg compressed.jpg
//	fennec -quality ultra photo.png out.png
//	fennec -quality aggressive -max-width 1920 photo.jpg out.jpg
//	fennec -target-size 100KB photo.jpg out.jpg
//	fennec -analyze photo.jpg
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shamspias/fennec"
)

func main() {
	var (
		quality    string
		format     string
		maxWidth   int
		maxHeight  int
		targetSize string
		analyze    bool
		ssim       float64
	)

	flag.StringVar(&quality, "quality", "balanced", "Quality preset: lossless|ultra|high|balanced|aggressive|maximum")
	flag.StringVar(&format, "format", "auto", "Output format: auto|jpeg|png")
	flag.IntVar(&maxWidth, "max-width", 0, "Maximum width (0 = no limit)")
	flag.IntVar(&maxHeight, "max-height", 0, "Maximum height (0 = no limit)")
	flag.StringVar(&targetSize, "target-size", "", "Target file size (e.g. 100KB, 2MB)")
	flag.BoolVar(&analyze, "analyze", false, "Analyze image without compressing")
	flag.Float64Var(&ssim, "ssim", 0, "Custom SSIM target (0.0-1.0, overrides quality)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: fennec [flags] <input> [output]")
		fmt.Fprintln(os.Stderr, "       fennec -analyze <input>")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		os.Exit(1)
	}

	input := args[0]

	if analyze {
		runAnalyze(input)
		return
	}

	output := ""
	if len(args) >= 2 {
		output = args[1]
	} else {
		ext := filepath.Ext(input)
		base := strings.TrimSuffix(input, ext)
		output = base + "_compressed" + ext
	}

	opts := fennec.DefaultOptions()

	switch strings.ToLower(quality) {
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
		fmt.Fprintf(os.Stderr, "Unknown quality: %s\n", quality)
		os.Exit(1)
	}

	switch strings.ToLower(format) {
	case "auto":
		opts.Format = fennec.Auto
	case "jpeg", "jpg":
		opts.Format = fennec.JPEG
	case "png":
		opts.Format = fennec.PNG
	}

	opts.MaxWidth = maxWidth
	opts.MaxHeight = maxHeight

	if ssim > 0 {
		opts.TargetSSIM = ssim
	}

	if targetSize != "" {
		bytes, err := parseSize(targetSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid target-size %q: %v\n", targetSize, err)
			os.Exit(1)
		}
		opts.TargetSize = bytes
	}

	result, err := fennec.CompressFile(input, output, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// If the target-size engine chose a different format than the output
	// extension, rename the file so it opens correctly.
	actualExt := ".png"
	if result.Format == fennec.JPEG {
		actualExt = ".jpg"
	}
	outExt := strings.ToLower(filepath.Ext(output))
	if outExt != actualExt && outExt != ".jpeg" {
		newOutput := strings.TrimSuffix(output, filepath.Ext(output)) + actualExt
		if err := os.Rename(output, newOutput); err == nil {
			output = newOutput
			fmt.Fprintf(os.Stderr, "Note: format changed to %s → saved as %s\n", strings.ToUpper(actualExt[1:]), newOutput)
		}
	}

	fmt.Println(result)
}

func runAnalyze(path string) {
	img, err := fennec.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", path, err)
		os.Exit(1)
	}

	info, _ := os.Stat(path)
	stats := fennec.Analyze(img)

	fmt.Printf("📐 File:         %s\n", path)
	if info != nil {
		fmt.Printf("💾 Size:         %s\n", humanBytes(info.Size()))
	}
	fmt.Printf("📏 Dimensions:   %d × %d\n", stats.Width, stats.Height)
	fmt.Printf("🎭 Alpha:        %v\n", stats.HasAlpha)
	fmt.Printf("⬛ Grayscale:    %v\n", stats.IsGrayscale)
	fmt.Printf("🎨 Unique colors: %d+\n", stats.UniqueColors)
	fmt.Printf("📊 Entropy:      %.2f bits\n", stats.Entropy)
	fmt.Printf("🔲 Edge density: %.1f%%\n", stats.EdgeDensity*100)
	fmt.Printf("☀️  Brightness:   %.0f\n", stats.MeanBrightness)
	fmt.Printf("🌗 Contrast:     %.1f\n", stats.Contrast)
	fmt.Println()
	fmt.Printf("💡 Recommended format:  %s\n", formatName(stats.RecommendedFormat))
	fmt.Printf("💡 Recommended quality: %s\n", stats.RecommendedQuality)
	fmt.Printf("💡 Est. compression:    ~%.0f%%\n", stats.EstimatedCompression*100)
}

func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := 1
	if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return int(n * float64(multiplier)), nil
}

func humanBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatName(f fennec.Format) string {
	switch f {
	case fennec.JPEG:
		return "JPEG"
	case fennec.PNG:
		return "PNG"
	default:
		return "Auto"
	}
}
