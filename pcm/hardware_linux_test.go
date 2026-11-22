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

func TestConfigureHardware(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_PCM")
	if name == "" || os.Getenv("GO_ALSA_TEST_CONFIGURE") != "1" {
		t.Skip("set GO_ALSA_TEST_PCM and GO_ALSA_TEST_CONFIGURE=1")
	}
	stream := alsa.Playback
	channels := 2
	if os.Getenv("GO_ALSA_TEST_PCM_STREAM") == "capture" {
		stream = alsa.Capture
	}
	handle, err := OpenHardware(name, stream)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: channels, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800}
	setup, err := handle.ConfigureHardware(config)
	if err != nil {
		t.Fatalf("ConfigureHardware: %v", err)
	}
	if setup.Access != config.Access || setup.Format != config.Format || setup.Channels != config.Channels || setup.Rate != config.Rate {
		t.Errorf("setup = %#v; requested %#v", setup, config)
	}
	software, err := handle.ConfigureSoftwareDefaults()
	if err != nil {
		t.Fatalf("ConfigureSoftwareDefaults: %v", err)
	}
	if software.AvailableMinimum != setup.PeriodFrames || software.Boundary <= uint64(setup.BufferFrames) {
		t.Errorf("software setup = %#v", software)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}

func TestTransferHardware(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_PCM")
	if name == "" || os.Getenv("GO_ALSA_TEST_TRANSFER") != "1" {
		t.Skip("set GO_ALSA_TEST_PCM and GO_ALSA_TEST_TRANSFER=1")
	}
	stream := alsa.Playback
	if os.Getenv("GO_ALSA_TEST_PCM_STREAM") == "capture" {
		stream = alsa.Capture
	}
	handle, err := OpenHardware(name, stream)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800}
	if _, err := handle.ConfigureHardware(config); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.ConfigureSoftwareDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, config.PeriodFrames*config.Channels)
	if stream == alsa.Capture {
		if err := handle.Start(); err != nil {
			t.Fatal(err)
		}
		frames, err := handle.ReadInterleaved(t.Context(), samples)
		if err != nil {
			t.Fatal(err)
		}
		if frames <= 0 || frames > config.PeriodFrames {
			t.Fatalf("captured frames = %d", frames)
		}
		return
	}
	frames, err := handle.WriteInterleaved(t.Context(), samples)
	if err != nil {
		t.Fatal(err)
	}
	if frames <= 0 || frames > config.PeriodFrames {
		t.Fatalf("played frames = %d", frames)
	}
}
