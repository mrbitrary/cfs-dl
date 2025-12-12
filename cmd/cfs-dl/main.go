package main

import (
	"cfs-dl/internal/downloader"
	"cfs-dl/internal/merger"
	"cfs-dl/internal/model"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	// Parse flags using a custom FlagSet to allow testing
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)

	urlPtr := fs.String("url", "", "Cloudflare Stream iframe URL")
	outputDirPtr := fs.String("output-dir", "data/download", "Directory to save the output file")
	outputFilePtr := fs.String("filename", "output.mp4", "Output filename")
	resolutionPtr := fs.String("resolution", "1080p", "Target video resolution (e.g., 1080p, 720p)")
	checkDepsPtr := fs.Bool("check-dependencies", false, "Check if required dependencies (ffmpeg) are installed")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s --url <url> [options]\n", args[0])
		fmt.Fprintf(stderr, "\nDownloads videos from Cloudflare Stream iframe URLs.\n")
		fmt.Fprintf(stderr, "\nRequired:\n")
		fmt.Fprintf(stderr, "  --url string\n    \tCloudflare Stream iframe URL\n")
		fmt.Fprintf(stderr, "\nOptions:\n")
		fs.VisitAll(func(f *flag.Flag) {
			if f.Name == "url" {
				return // Already printed in Required
			}
			fmt.Fprintf(stderr, "  --%s %s\n    \t%s (default: %q)\n", f.Name, f.Value.String(), f.Usage, f.DefValue)
		})
		fmt.Fprintf(stderr, "\nExample:\n  %s --url \"https://.../iframe\" --resolution 720p\n", args[0])
	}

	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	if *checkDepsPtr {
		if err := checkRequirements(); err != nil {
			fmt.Fprintf(stdout, "Dependency Check: FAIL\n%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Dependency Check: PASS\nffmpeg is installed and available.\n")
		return 0
	}

	if *urlPtr == "" {
		fmt.Fprintln(stdout, "Error: --url is required")
		fs.Usage()
		return 1
	}

	if err := checkRequirements(); err != nil {
		fmt.Fprintf(stdout, "Error: %v\n", err)
		return 1
	}

	manifestUrl, err := extractManifestUrl(*urlPtr)
	if err != nil {
		fmt.Fprintf(stdout, "Error extracting manifest URL: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Fetching manifest from: %s\n", manifestUrl)
	mpd, err := model.ParseManifest(manifestUrl)
	if err != nil {
		fmt.Fprintf(stdout, "Error parsing manifest: %v\n", err)
		return 1
	}

	finalFilename := *outputFilePtr
	if finalFilename == "output.mp4" {
		if mpd.ProgramInformation != nil && mpd.ProgramInformation.Title != "" {
			safeTitle := sanitizeFilename(mpd.ProgramInformation.Title)
			if safeTitle != "" {
				finalFilename = safeTitle + ".mp4"
				fmt.Fprintf(stdout, "Using title from manifest: %s\n", finalFilename)
			}
		}
	}

	if err := os.MkdirAll(*outputDirPtr, 0755); err != nil {
		fmt.Fprintf(stdout, "Error creating output directory: %v\n", err)
		return 1
	}

	outputPath := fmt.Sprintf("%s/%s", strings.TrimRight(*outputDirPtr, "/"), finalFilename)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Fprintln(stdout, "\nReceived interrupt signal, stopping...")
		cancel()
	}()

	targetHeight := parseResolution(*resolutionPtr)

	videoRep, err := mpd.SelectVideoRepresentation(targetHeight)
	if err != nil {
		fmt.Fprintf(stdout, "Error selecting video stream: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Selected video stream: ID=%s, Bandwidth=%d, Height=%d (Requested: %s)\n", videoRep.ID, videoRep.Bandwidth, videoRep.Height, *resolutionPtr)

	audioRep, err := mpd.SelectAudioRepresentation()
	if err != nil {
		fmt.Fprintf(stdout, "Error selecting audio stream: %v\n", err)
		return 1
	}

	totalDuration, _ := parseDuration(mpd.MediaPresentationDuration)

	videoFile, err := downloader.DownloadStream(ctx, manifestUrl, videoRep, totalDuration)
	if err != nil {
		if err == context.Canceled {
			fmt.Fprintln(stdout, "Download cancelled.")
			cleanup(videoFile)
			return 0
		}
		fmt.Fprintf(stdout, "Error downloading video: %v\n", err)
		cleanup(videoFile)
		return 1
	}
	defer cleanup(videoFile)

	audioFile, err := downloader.DownloadStream(ctx, manifestUrl, audioRep, totalDuration)
	if err != nil {
		if err == context.Canceled {
			fmt.Fprintln(stdout, "Download cancelled.")
			cleanup(videoFile)
			cleanup(audioFile)
			return 0
		}
		fmt.Fprintf(stdout, "Error downloading audio: %v\n", err)
		cleanup(audioFile)
		return 1
	}
	defer cleanup(audioFile)

	if err := merger.MergeAudioVideo(videoFile, audioFile, outputPath); err != nil {
		fmt.Fprintf(stdout, "Error combining video and audio: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Successfully created %s\n", outputPath)
	return 0
}

// Helpers (unchanged, just kept here for completeness of file write)
func extractManifestUrl(iframeUrl string) (string, error) {
	if strings.HasSuffix(iframeUrl, "/iframe") {
		return strings.TrimSuffix(iframeUrl, "/iframe") + "/manifest/video.mpd", nil
	}
	if strings.HasSuffix(iframeUrl, ".mpd") {
		return iframeUrl, nil
	}
	return iframeUrl + "/manifest/video.mpd", nil
}

func sanitizeFilename(name string) string {
	safe := strings.ReplaceAll(name, "/", "-")
	safe = strings.ReplaceAll(safe, "\\", "-")
	safe = strings.ReplaceAll(safe, ":", "-")
	safe = strings.ReplaceAll(safe, "*", "")
	safe = strings.ReplaceAll(safe, "?", "")
	safe = strings.ReplaceAll(safe, "\"", "")
	safe = strings.ReplaceAll(safe, "<", "")
	safe = strings.ReplaceAll(safe, ">", "")
	safe = strings.ReplaceAll(safe, "|", "")
	safe = strings.TrimSpace(safe)
	return safe
}

func parseResolution(res string) int {
	res = strings.ToLower(res)
	res = strings.TrimSuffix(res, "p")
	h, err := strconv.Atoi(res)
	if err != nil {
		return 1080
	}
	return h
}

func parseDuration(durationStr string) (float64, error) {
	s := strings.TrimPrefix(durationStr, "PT")
	var minutes, seconds float64
	if idx := strings.Index(s, "M"); idx != -1 {
		fmt.Sscanf(s[:idx], "%f", &minutes)
		s = s[idx+1:]
	}
	if idx := strings.Index(s, "S"); idx != -1 {
		fmt.Sscanf(s[:idx], "%f", &seconds)
	}
	return minutes*60 + seconds, nil
}

func cleanup(f string) {
	if f != "" {
		os.Remove(f)
	}
}

func checkRequirements() error {
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed or not in PATH. It is required to merge audio and video")
	}
	return nil
}
