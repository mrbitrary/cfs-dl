package main

import (
	"bytes"
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
