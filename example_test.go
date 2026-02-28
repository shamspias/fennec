package fennec_test

import (
	"context"
	"fmt"

	"github.com/shamspias/fennec"
)

func ExampleCompressFile() {
	ctx := context.Background()
	opts := fennec.DefaultOptions() // Balanced preset, SSIM ≥ 0.94

	result, err := fennec.CompressFile(ctx, "photo.jpg", "optimized.jpg", opts)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}

func ExampleCompressImage() {
	ctx := context.Background()

	img, err := fennec.Open("photo.jpg")
	if err != nil {
		panic(err)
	}

	opts := fennec.DefaultOptions()
	opts.Quality = fennec.High // SSIM ≥ 0.97
	opts.MaxWidth = 1920

	result, err := fennec.CompressImage(ctx, img, opts)
	if err != nil {
		panic(err)
	}
	fmt.Printf("SSIM: %.4f, Saved: %.1f%%\n", result.SSIM, result.SavingsPercent)
}

func ExampleCompressBytes() {
	ctx := context.Background()

	// Common server-side pattern: receive bytes, compress, return bytes.
	inputData := []byte{} // ... from HTTP request, S3, etc.

	result, err := fennec.CompressBytes(ctx, inputData, fennec.DefaultOptions())
	if err != nil {
		panic(err)
	}

	outputData := result.Bytes() // Ready to write to response or storage.
	_ = outputData
}

func ExampleAnalyze() {
	img, err := fennec.Open("photo.jpg")
	if err != nil {
		panic(err)
	}

	stats := fennec.Analyze(img)
	fmt.Printf("Format: %s, Quality: %s, Entropy: %.2f\n",
		stats.RecommendedFormat, stats.RecommendedQuality, stats.Entropy)
}

func ExampleCompressBatch() {
	ctx := context.Background()

	items := []fennec.BatchItem{
		{Src: "photo1.jpg", Dst: "out/photo1.jpg"},
		{Src: "photo2.png", Dst: "out/photo2.jpg"},
		{Src: "photo3.jpg", Dst: "out/photo3.jpg"},
	}

	results := fennec.CompressBatch(ctx, items, fennec.BatchOptions{
		Workers:     4,
		DefaultOpts: fennec.DefaultOptions(),
		OnItem: func(completed, total int) {
			fmt.Printf("Progress: %d/%d\n", completed, total)
		},
	})

	summary := fennec.Summarize(results)
	fmt.Println(summary)
}
