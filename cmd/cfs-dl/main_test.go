package main

import (
	"bytes"
	"cfs-dl/internal/model"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"NormalFile", "NormalFile"},
		{"File/With/Slashes", "File-With-Slashes"},
		{"File:With:Colons", "File-With-Colons"},
		{"  TrimSpaces  ", "TrimSpaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseResolution(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1080p", 1080},
		{"720", 720},
		{"invalid", 1080},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseResolution(tt.input)
			if got != tt.expected {
				t.Errorf("parseResolution(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"PT1M30S", 90.0},
		{"PT45S", 45.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, _ := parseDuration(tt.input)
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %f, want %f", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractManifestUrl(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/video/iframe", "https://example.com/video/manifest/video.mpd"},
		{"https://example.com/video.mpd", "https://example.com/video.mpd"},
		{"https://example.com/video", "https://example.com/video/manifest/video.mpd"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, _ := extractManifestUrl(tt.input)
			if got != tt.expected {
				t.Errorf("extractManifestUrl(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRun_CheckDependencies(t *testing.T) {
	// Assumes ffmpeg is installed in devbox
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	args := []string{"cfs-dl", "--check-dependencies"}

	code := run(args, stdout, stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "PASS") {
		t.Errorf("expected PASS in stdout, got %q", stdout.String())
	}
}

func TestRun_NoUrl(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	args := []string{"cfs-dl"}

	code := run(args, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error: --url is required") {
		t.Errorf("expected error message in stdout, got %q", stdout.String())
	}
}

func TestRun_ParseManifestFail(t *testing.T) {
	orig := parseManifestFunc
	defer func() { parseManifestFunc = orig }()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return nil, fmt.Errorf("mock parse error")
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--url", "https://example.com/iframe"}
	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error parsing manifest") {
		t.Errorf("expected error message, got %s", stdout.String())
	}
}

func TestRun_StreamSelectFail(t *testing.T) {
	origParse := parseManifestFunc
	defer func() { parseManifestFunc = origParse }()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		// Return MPD with NO video representations
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{MimeType: "audio/mp4"},
				},
			},
		}, nil
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--url", "https://example.com/iframe"}
	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error selecting video stream") {
		t.Errorf("expected error message, got %s", stdout.String())
	}
}

func TestRun_DownloadFail(t *testing.T) {
	origParse := parseManifestFunc
	origDL := downloadStreamFunc
	defer func() {
		parseManifestFunc = origParse
		downloadStreamFunc = origDL
	}()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{
						MimeType: "video/mp4",
						Representations: []model.Representation{
							{ID: "1080p", Height: 1080, Bandwidth: 1000},
						},
					},
					{
						MimeType: "audio/mp4",
						Representations: []model.Representation{
							{ID: "audio", Bandwidth: 100},
						},
					},
				},
			},
		}, nil
	}

	downloadStreamFunc = func(ctx context.Context, base string, rep *model.Representation, dur float64) (string, error) {
		return "", fmt.Errorf("mock download error")
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--url", "https://example.com/iframe"}
	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error downloading video") {
		t.Errorf("expected error message, got %s", stdout.String())
	}
}

func TestRun_MergeFail(t *testing.T) {
	origParse := parseManifestFunc
	origDL := downloadStreamFunc
	origMerge := mergeAudioVideoFunc
	defer func() {
		parseManifestFunc = origParse
		downloadStreamFunc = origDL
		mergeAudioVideoFunc = origMerge
	}()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{
						MimeType: "video/mp4",
						Representations: []model.Representation{
							{ID: "1080p", Height: 1080, Bandwidth: 1000},
						},
					},
					{
						MimeType: "audio/mp4",
						Representations: []model.Representation{
							{ID: "audio", Bandwidth: 100},
						},
					},
				},
			},
		}, nil
	}

	downloadStreamFunc = func(ctx context.Context, base string, rep *model.Representation, dur float64) (string, error) {
		return "temp.mp4", nil
	}

	mergeAudioVideoFunc = func(v, a, o string) error {
		return fmt.Errorf("mock merge error")
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--url", "https://example.com/iframe"}
	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error combining video and audio") {
		t.Errorf("expected error message, got %s", stdout.String())
	}
}

func TestRun_Success(t *testing.T) {
	origParse := parseManifestFunc
	origDL := downloadStreamFunc
	origMerge := mergeAudioVideoFunc
	defer func() {
		parseManifestFunc = origParse
		downloadStreamFunc = origDL
		mergeAudioVideoFunc = origMerge
	}()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{
						MimeType: "video/mp4",
						Representations: []model.Representation{
							{ID: "1080p", Height: 1080, Bandwidth: 1000},
						},
					},
					{
						MimeType: "audio/mp4",
						Representations: []model.Representation{
							{ID: "audio", Bandwidth: 100},
						},
					},
				},
			},
			ProgramInformation: &model.ProgramInformation{Title: "TestTitle"},
		}, nil
	}

	downloadStreamFunc = func(ctx context.Context, base string, rep *model.Representation, dur float64) (string, error) {
		return "temp.mp4", nil
	}

	mergeAudioVideoFunc = func(v, a, o string) error {
		return nil
	}

	stdout := new(bytes.Buffer)
	// Use temp dir for output to avoid permission issues or clutter
	tmpDir := t.TempDir()
	args := []string{"cfs-dl", "--url", "https://example.com/iframe", "--output-dir", tmpDir}
	code := run(args, stdout, new(bytes.Buffer))
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Successfully created") {
		t.Errorf("expected success message, got %s", stdout.String())
	}
}

func TestRun_CheckDepsFail(t *testing.T) {
	origLookPath := lookPathFunc
	defer func() { lookPathFunc = origLookPath }()

	lookPathFunc = func(file string) (string, error) {
		return "", fmt.Errorf("mock found error")
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--check-dependencies"}

	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in stdout, got %q", stdout.String())
	}
}

func TestRun_MkdirFail(t *testing.T) {
	origParse := parseManifestFunc
	defer func() { parseManifestFunc = origParse }()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{
						MimeType: "video/mp4",
						Representations: []model.Representation{
							{ID: "1080p", Height: 1080, Bandwidth: 1000},
						},
					},
					{
						MimeType: "audio/mp4",
						Representations: []model.Representation{
							{ID: "audio", Bandwidth: 100},
						},
					},
				},
			},
		}, nil
	}

	stdout := new(bytes.Buffer)
	// Try to write to a dev null or something that fails mkdir
	// On linux /dev/null/output is a good candidate since /dev/null is a file
	args := []string{"cfs-dl", "--url", "https://example.com/iframe", "--output-dir", "/dev/null/output"}

	code := run(args, stdout, new(bytes.Buffer))
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Error creating output directory") {
		t.Errorf("expected error message, got %s", stdout.String())
	}
}

func TestRun_DownloadCancel(t *testing.T) {
	origParse := parseManifestFunc
	origDL := downloadStreamFunc
	defer func() {
		parseManifestFunc = origParse
		downloadStreamFunc = origDL
	}()

	parseManifestFunc = func(url string) (*model.MPD, error) {
		return &model.MPD{
			Period: model.Period{
				AdaptationSets: []model.AdaptationSet{
					{MimeType: "video/mp4", Representations: []model.Representation{{ID: "1080p", Height: 1080}}},
					{MimeType: "audio/mp4", Representations: []model.Representation{{ID: "a", Bandwidth: 100}}},
				},
			},
		}, nil
	}

	downloadStreamFunc = func(ctx context.Context, base string, rep *model.Representation, dur float64) (string, error) {
		return "", context.Canceled
	}

	stdout := new(bytes.Buffer)
	args := []string{"cfs-dl", "--url", "https://example.com"}

	code := run(args, stdout, new(bytes.Buffer))
	if code != 0 {
		t.Errorf("expected exit code 0 on cancel, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Download cancelled") {
		t.Errorf("expected cancelled message, got %s", stdout.String())
	}
}
