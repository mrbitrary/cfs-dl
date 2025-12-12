package downloader

import (
	"cfs-dl/internal/model"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestResolveSegmentUrl(t *testing.T) {
	tests := []struct {
		name       string
		baseUrl    string
		relative   string
		repID      string
		expected   string
		shouldFail bool
	}{
		{
			name:     "Simple Relative",
			baseUrl:  "https://example.com/video/manifest.mpd",
			relative: "segment.mp4",
			expected: "https://example.com/video/segment.mp4",
		},
		{
			name:     "Relative with Parent Dir",
			baseUrl:  "https://example.com/video/manifest/video.mpd",
			relative: "../../segment.mp4",
			expected: "https://example.com/segment.mp4",
		},
		{
			name:     "Relative with Query Params",
			baseUrl:  "https://example.com/manifest.mpd?token=123",
			relative: "segment.mp4?query=abc",
			expected: "https://example.com/segment.mp4?query=abc",
		},
		// NOTE: Current implementation of resolveSegmentUrl uses url.ResolveReference
		// If the relative URL has a query string, it replaces the base query string?
		// Or if baseUrl is the manifest URL, we expect segments to be relative to it.
		// Cloudflare segments often have huge query strings.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: resolveSegmentUrl is unexported, but we are in the same package (whitebox test)
			got, err := resolveSegmentUrl(tt.baseUrl, tt.relative, tt.repID)
			if (err != nil) != tt.shouldFail {
				t.Fatalf("resolveSegmentUrl() error = %v, shouldFail %v", err, tt.shouldFail)
			}
			if got != tt.expected {
				t.Errorf("resolveSegmentUrl() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDownloadStream(t *testing.T) {
	// Mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/init.mp4":
			w.Write([]byte("init data"))
		case "/media_0.mp4":
			w.Write([]byte("media 0"))
		case "/media_1.mp4":
			w.Write([]byte("media 1"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Create a mock representation
	rep := &model.Representation{
		ID:        "test_rep",
		Bandwidth: 1000,
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "/init.mp4",
			Media:          "/media_$Number$.mp4",
			StartNumber:    0,
			Timescale:      1,
			Duration:       2, // 2 seconds per segment
		},
	}

	// 2 seconds total, 2 second segments => 1 segment (0 to 0)?
	// logic: totalSegments = total / segDur
	// 4 secs total / 2 segDur = 2 segments (0, 1)
	totalDuration := 4.0

	ctx := context.Background()
	filename, err := DownloadStream(ctx, ts.URL, rep, totalDuration)
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer os.Remove(filename)

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	expected := "init datamedia 0media 1"
	if string(content) != expected {
		t.Errorf("expected content %q, got %q", expected, string(content))
	}
}

func TestDownloadStream_Cancel(t *testing.T) {
	// Mock server that hangs
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {} // Block forever
	}))
	defer ts.Close()

	rep := &model.Representation{
		ID: "test_rep_cancel",
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "/init.mp4",
			Media:          "/media_$Number$.mp4",
			Timescale:      1,
			Duration:       1,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	_, err := DownloadStream(ctx, ts.URL, rep, 10.0)
	if err == nil {
		t.Error("expected error on cancel, got nil")
	}
}
