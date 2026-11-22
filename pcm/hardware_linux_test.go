//go:build linux && (amd64 || arm64)

package pcm

import (
	"os"
	"testing"

	alsa "github.com/sparques/go-alsa"
)

// TestOpenHardware is opt-in because merely opening a PCM may conflict with a
// desktop sound server or another exclusive hardware user.
func TestOpenHardware(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_PCM")
	if name == "" {
		t.Skip("set GO_ALSA_TEST_PCM to a direct hw: PCM name")
	}
	stream := alsa.Playback
	if os.Getenv("GO_ALSA_TEST_PCM_STREAM") == "capture" {
		stream = alsa.Capture
	}

	handle, err := OpenHardware(name, stream)
	if err != nil {
		t.Fatalf("OpenHardware(%q, %s): %v", name, stream, err)
	}
	major, minor, micro := handle.ProtocolVersion()
	if major != 2 {
		t.Errorf("protocol = %d.%d.%d; want major 2", major, minor, micro)
	}
	info := handle.Info()
	if info.Card < 0 || info.Device < 0 || info.ID == "" || info.Name == "" {
		t.Errorf("invalid opened PCM info: %#v", info)
	}
	if err := handle.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
