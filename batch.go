package fennec

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// BatchItem represents one file to compress in a batch operation.
type BatchItem struct {
	// Src is the input file path.
	Src string
	// Dst is the output file path.
	Dst string
	// Opts are the per-item compression options. If nil, BatchOptions.DefaultOpts is used.
	Opts *Options
}

// BatchResult holds the result for a single item in a batch.
type BatchResult struct {
	// Item is the original batch item.
	Item BatchItem
	// Result is the compression result (nil if Err is non-nil).
	Result *Result
	// Err is any error that occurred.
	Err error
	// Index is the position in the original input slice.
	Index int
}

// BatchOptions configures batch compression behavior.
type BatchOptions struct {
	// Workers is the number of concurrent workers. 0 = runtime.NumCPU().
	Workers int
	// DefaultOpts is used for any BatchItem where Opts is nil.
	DefaultOpts Options
	// OnItem is called after each item completes (for progress reporting).
	// It receives the item index and total count.
	OnItem func(completed, total int)
}

// CompressBatch compresses multiple image files concurrently using a worker pool.
// Results are returned in the same order as the input items.
// The context can be used to cancel the entire batch — in-flight items will
// finish but no new items will be started.
//
// Example:
//
//	items := []fennec.BatchItem{
//	    {Src: "photo1.jpg", Dst: "out1.jpg"},
//	    {Src: "photo2.png", Dst: "out2.jpg"},
//	}
//	results := fennec.CompressBatch(ctx, items, fennec.BatchOptions{
//	    Workers: 4,
//	    DefaultOpts: fennec.DefaultOptions(),
//	})
func CompressBatch(ctx context.Context, items []BatchItem, batchOpts BatchOptions) []BatchResult {
	if len(items) == 0 {
		return nil
	}

	workers := batchOpts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(items) {
		workers = len(items)
	}

	results := make([]BatchResult, len(items))
	workCh := make(chan int, len(items))
	var wg sync.WaitGroup
	var completed int
	var completedMu sync.Mutex

	// Feed work.
	for i := range items {
		workCh <- i
	}
	close(workCh)

	// Start workers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range workCh {
				// Check cancellation before starting new work.
				select {
				case <-ctx.Done():
					results[idx] = BatchResult{
						Item:  items[idx],
						Err:   ctx.Err(),
						Index: idx,
					}
					continue
				default:
				}

				item := items[idx]
				opts := batchOpts.DefaultOpts
				if item.Opts != nil {
					opts = *item.Opts
				}

				result, err := CompressFile(ctx, item.Src, item.Dst, opts)
				results[idx] = BatchResult{
					Item:   item,
					Result: result,
					Err:    err,
					Index:  idx,
				}

				if batchOpts.OnItem != nil {
					completedMu.Lock()
					completed++
					c := completed
					completedMu.Unlock()
					batchOpts.OnItem(c, len(items))
				}
			}
		}()
	}

	wg.Wait()
	return results
}

// BatchSummary provides aggregate statistics for a batch operation.
type BatchSummary struct {
	Total      int
	Succeeded  int
	Failed     int
	TotalSaved int64
	AvgSSIM    float64
}

// Summarize computes aggregate statistics from batch results.
func Summarize(results []BatchResult) BatchSummary {
	s := BatchSummary{Total: len(results)}
	var ssimSum float64
	for _, r := range results {
		if r.Err != nil {
			s.Failed++
			continue
		}
		s.Succeeded++
		if r.Result != nil {
			s.TotalSaved += r.Result.OriginalSize - r.Result.CompressedSize
			ssimSum += r.Result.SSIM
		}
	}
	if s.Succeeded > 0 {
		s.AvgSSIM = ssimSum / float64(s.Succeeded)
	}
	return s
}

// String returns a human-readable batch summary.
func (s BatchSummary) String() string {
	return fmt.Sprintf(
		"Batch: %d/%d succeeded | %s saved | Avg SSIM: %.4f",
		s.Succeeded, s.Total, humanBytes(s.TotalSaved), s.AvgSSIM,
	)
}
