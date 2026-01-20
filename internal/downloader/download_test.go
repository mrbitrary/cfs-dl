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
			_, _ = w.Write([]byte("init data"))
		case "/media_0.mp4":
			_, _ = w.Write([]byte("media 0"))
		case "/media_1.mp4":
			_, _ = w.Write([]byte("media 1"))
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
	// 4 secs total / 2 segDur = 2 segments (0, 1) -> +1 logic makes it 3.
	// We only mock 2 segments (0, 1), so usage 3.0s total duration.
	// int(3.0 / 2.0) = 1. 1 + 1 = 2.
	totalDuration := 3.0

	ctx := context.Background()
	filename, err := DownloadStream(ctx, ts.URL, rep, totalDuration)
	if err != nil {
		t.Fatalf("DownloadStream failed: %v", err)
	}
	defer func() { _ = os.Remove(filename) }()

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

func TestDownloadStream_InitFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	rep := &model.Representation{
		ID: "test_rep_fail",
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "/init.mp4",
		},
	}

	ctx := context.Background()
	_, err := DownloadStream(ctx, ts.URL, rep, 10.0)
	if err == nil {
		t.Error("expected error on init failure, got nil")
	}
}

func TestDownloadStream_SegmentFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/init.mp4":
			_, _ = w.Write([]byte("init"))
		case "/media_0.mp4":
			_, _ = w.Write([]byte("s0"))
		case "/media_1.mp4":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	rep := &model.Representation{
		ID: "test_rep_seg_fail",
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "/init.mp4",
			Media:          "/media_$Number$.mp4",
			StartNumber:    0,
			Timescale:      1,
			Duration:       5,
		},
	}
	// Total duration 11s, duration 5s => 2.2 segments => 3 segments (0, 1, 2)
	// Segment 0 OK, Segment 1 Fail.

	ctx := context.Background()
	filename, err := DownloadStream(ctx, ts.URL, rep, 11.0)
	if err == nil {
		_ = os.Remove(filename)
		t.Error("expected error when segment download fails, got nil")
	}
}

func TestResolveSegmentUrl_Fail(t *testing.T) {
	// url.Parse fails on control characters
	_, err := resolveSegmentUrl("http://base.com", "seg\nment.mp4", "id")
	if err == nil {
		t.Error("expected error on invalid relative url, got nil")
	}

	_, err = resolveSegmentUrl("http://ba\nse.com", "segment.mp4", "id")
	if err == nil {
		t.Error("expected error on invalid base url, got nil")
	}
}

func TestDownloadStream_SegmentNetworkFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/init.mp4" {
			_, _ = w.Write([]byte("init"))
			return
		}
		// Should not be reached for segment if we point it elsewhere
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	rep := &model.Representation{
		ID: "test_net_fail",
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "/init.mp4",
			Media:          "http://invalid-domain-that-does-not-exist.local/seg.mp4", // Absolute URL override
			StartNumber:    0,
			Timescale:      1,
			Duration:       5,
		},
	}

	ctx := context.Background()
	filename, err := DownloadStream(ctx, ts.URL, rep, 6.0)
	if err == nil {
		_ = os.Remove(filename)
		t.Error("expected error on segment network failure, got nil")
	}
}

func TestDownloadStream_InitNetworkFail(t *testing.T) {
	// Use an invalid URL for init segment
	rep := &model.Representation{
		ID: "test_init_net_fail",
		SegmentTemplate: model.SegmentTemplate{
			Initialization: "http://invalid-domain.local/init.mp4",
			Media:          "http://invalid-domain.local/media.mp4",
		},
	}

	ctx := context.Background()
	_, err := DownloadStream(ctx, "http://base.com", rep, 10.0)
	if err == nil {
		t.Error("expected error on init network failure, got nil")
	}
}
