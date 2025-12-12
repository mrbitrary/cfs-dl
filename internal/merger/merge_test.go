package merger

import (
	"os"
	"os/exec"
	"testing"
)

// We want to mock exec.Command, but in Go that's tricky without an interface or variable.
// However, since we are testing "MergeAudioVideo", and it calls ffmpeg, we can do a simple integration-like test
// or we can refactor code to be testable.
// A common Go trick for mocking exec.Command involves re-executing the test binary.

func TestMergeAudioVideo(t *testing.T) {
	// Mock successful ffmpeg call
	// We replace execCommand with a helper that just exits success
	execCommand = func(name string, arg ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, arg...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
	defer func() { execCommand = exec.Command }()

	err := MergeAudioVideo("video.mp4", "audio.mp4", "output.mp4")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestMergeAudioVideo_Fail(t *testing.T) {
	// Mock failed ffmpeg call
	execCommand = func(name string, arg ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcessFail", "--", name}
		cs = append(cs, arg...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
	defer func() { execCommand = exec.Command }()

	err := MergeAudioVideo("video.mp4", "audio.mp4", "output.mp4")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestHelperProcess isn't a real test. It's used as a helper process to mock exec.Command
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// Verify arguments if needed, for now just exit 0
	os.Exit(0)
}

func TestHelperProcessFail(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}
