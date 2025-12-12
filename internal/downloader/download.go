package downloader

import (
	"cfs-dl/internal/model"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// DownloadStream downloads all segments for a given representation and merges them into a temporary file.
// Returns the path to the temporary file.
func DownloadStream(ctx context.Context, baseUrl string, rep *model.Representation, totalDurationSecs float64) (string, error) {
	fmt.Printf("Starting download for stream: %s (bandwidth: %d)\n", rep.ID, rep.Bandwidth)

	// Create a temp file to store the merged output
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("stream-%s-*.mp4", rep.ID))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	// 1. Download Initialization Segment
	initUrl, err := resolveSegmentUrl(baseUrl, rep.SegmentTemplate.Initialization, rep.ID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve init segment url: %w", err)
	}

	fmt.Printf("Downloading init segment: %s\n", initUrl)
	if err := downloadAndAppend(initUrl, tmpFile); err != nil {
		return "", fmt.Errorf("failed to download init segment: %w", err)
	}

	// 2. Download Media Segments
	// Calculate total segments execution.
	// Note: totalDurationSecs comes from MPD "mediaPresentationDuration"
	// Segment duration is in SegmentTemplate "duration" / "timescale"
	segDurationSecs := float64(rep.SegmentTemplate.Duration) / float64(rep.SegmentTemplate.Timescale)
	totalSegments := int(totalDurationSecs / segDurationSecs)
	// Add a few extra just in case of rounding errors or variable segment lengths,
	// checking for 404s to stop might be safer but for static DASH usually calculation is fine.
	// Actually better: loop until 404 if we want to be sure, or trust duration.
	// Let's trust duration + 1 for now.
	if totalDurationSecs > 0 && segDurationSecs > 0 {
		totalSegments++
	}

	fmt.Printf("Estimated segments: %d (Segment Duration: %.2fs)\n", totalSegments, segDurationSecs)

	// Sequential download for simplicity and appending order
	// Parallel download is faster but requires assembling in order.
	// Given the request for a simple tool, let's start sequential or simple parallel.
	// Parallel is much better for video.

	// Let's implement a simple parallel downloader with an ordered buffer/write.
	// Or simpler: Download to separate temp files then concat?
	// Or simplest: Download sequential.
	// Let's do sequential first to ensure correctness, user can request speed later if needed.
	// Actually, "downloader" implies we want speed. Let's do parallel 5 workers.

	jobs := make(chan int, totalSegments)
	results := make(chan segmentResult, totalSegments)
	var wg sync.WaitGroup

	// Start workers
	workerCount := 5
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case segNum, ok := <-jobs:
					if !ok {
						return
					}
					data, err := downloadSegment(ctx, baseUrl, rep, segNum)
					select {
					case <-ctx.Done():
						return
					case results <- segmentResult{index: segNum, data: data, err: err}:
					}
				}
			}
		}()
	}

	// Queue jobs
	startNum := rep.SegmentTemplate.StartNumber
	endNum := startNum + totalSegments
	// We might need to adjust endNum based on actual existence.
	// For now let's try the calculated amount.

	for i := startNum; i < endNum; i++ {
		jobs <- i
	}
	close(jobs)

	// Collect results
	// We need to write them in order.
	// We can store them in a map and write as they become available in sequence.
	segMap := make(map[int][]byte)
	nextToWrite := startNum

	// Wait for workers in a separate goroutine so we can close results
	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		if res.err != nil {
			fmt.Printf("Warning: failed to download segment %d: %v\n", res.index, res.err)
			// Decide if we stop or skip. Skipping breaks the stream usually.
			// Return error?
			return "", fmt.Errorf("failed to download segment %d: %w", res.index, res.err)
		}
		segMap[res.index] = res.data

		// Write all available consecutive segments
		for {
			data, ok := segMap[nextToWrite]
			if !ok {
				break
			}
			if _, err := tmpFile.Write(data); err != nil {
				return "", fmt.Errorf("failed to write segment %d to file: %w", nextToWrite, err)
			}
			delete(segMap, nextToWrite) // Free memory
			nextToWrite++
			fmt.Printf("\rDownloaded %d/%d segments...", nextToWrite-startNum, totalSegments)
		}
	}
	fmt.Println("\nDownload complete.")

	return tmpFile.Name(), nil
}

type segmentResult struct {
	index int
	data  []byte
	err   error
}

func downloadSegment(ctx context.Context, baseUrl string, rep *model.Representation, num int) ([]byte, error) {
	// Replace $Number$ with actual number
	mediaUrlStr := strings.Replace(rep.SegmentTemplate.Media, "$Number$", fmt.Sprintf("%d", num), -1)

	fullUrl, err := resolveSegmentUrl(baseUrl, mediaUrlStr, rep.ID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullUrl, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func resolveSegmentUrl(base, relative, repID string) (string, error) {
	// The URL in the manifest might contain query parameters that need to be preserved
	// relative might be: "../../something/seg_1.mp4?query=params"
	// base might be: "https://.../manifest/video.mpd"

	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	rel, err := url.Parse(relative)
	if err != nil {
		return "", err
	}

	// ResolveReference handles the "../../" relative paths correctly
	resolved := u.ResolveReference(rel)
	return resolved.String(), nil
}

func downloadAndAppend(url string, w io.Writer) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}
