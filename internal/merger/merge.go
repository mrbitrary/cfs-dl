package merger

import (
	"fmt"
	"os"
	"os/exec"
)

// var allows mocking in tests
var execCommand = exec.Command

func MergeAudioVideo(videoFile, audioFile, outputFile string) error {
	fmt.Printf("Merging video: %s and audio: %s to %s\n", videoFile, audioFile, outputFile)

	// ffmpeg -i video.mp4 -i audio.mp4 -c:v copy -c:a copy output.mp4
	cmd := execCommand("ffmpeg",
		"-y", // Overwrite output file
		"-i", videoFile,
		"-i", audioFile,
		"-c:v", "copy", // Copy video stream without re-encoding
		"-c:a", "copy", // Copy audio stream without re-encoding
		outputFile,
	)

	// Capture output for debugging if needed, or redirect to stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg merge failed: %w", err)
	}

	return nil
}
