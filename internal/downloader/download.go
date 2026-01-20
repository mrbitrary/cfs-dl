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
	defer func() { _ = tmpFile.Close() }()

	// 1. Download Initialization Segment
	initUrl, err := resolveSegmentUrl(baseUrl, rep.SegmentTemplate.Initialization, rep.ID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve init segment url: %w", err)
	}

	fmt.Printf("Downloading init segment: %s\n", initUrl)
	if err := downloadAndAppend(ctx, initUrl, tmpFile); err != nil {
		return "", fmt.Errorf("failed to download init segment: %w", err)
	}

	// 2. Download Media Segments
	// Calculate total segments based on duration
	segDurationSecs := float64(rep.SegmentTemplate.Duration) / float64(rep.SegmentTemplate.Timescale)
	totalSegments := int(totalDurationSecs / segDurationSecs)
	// Add an extra segment to cover any potential rounding issues or final short segments
	if totalDurationSecs > 0 && segDurationSecs > 0 {
		totalSegments++
	}

	fmt.Printf("Estimated segments: %d (Segment Duration: %.2fs)\n", totalSegments, segDurationSecs)

	workerCount := 5

	jobs := make(chan int, totalSegments)
	results := make(chan segmentResult, totalSegments)
	var wg sync.WaitGroup

	// Start workers
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

	startNum := rep.SegmentTemplate.StartNumber
	endNum := startNum + totalSegments

	for i := startNum; i < endNum; i++ {
		jobs <- i
	}
	close(jobs)

	// Collect results and write strictly in order
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
	mediaUrlStr := strings.ReplaceAll(rep.SegmentTemplate.Media, "$Number$", fmt.Sprintf("%d", num))

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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func resolveSegmentUrl(base, relative, repID string) (string, error) {
	// Preserves query parameters from the manifest URL if present
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

func downloadAndAppend(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}
