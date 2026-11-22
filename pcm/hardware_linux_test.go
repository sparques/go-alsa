//go:build linux && (amd64 || arm64)

package pcm

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

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

func TestCaptureXRUNRecovery(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_PCM")
	if name == "" || os.Getenv("GO_ALSA_TEST_XRUN") != "1" || os.Getenv("GO_ALSA_TEST_PCM_STREAM") != "capture" {
		t.Skip("set a capture GO_ALSA_TEST_PCM, GO_ALSA_TEST_PCM_STREAM=capture, and GO_ALSA_TEST_XRUN=1")
	}
	handle, err := OpenHardware(name, alsa.Capture)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 960}
	if _, err := handle.ConfigureHardware(config); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.ConfigureSoftwareDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	samples := make([]int16, config.PeriodFrames*config.Channels)
	if _, err := handle.ReadInterleaved(t.Context(), samples); !errors.Is(err, ErrXRUN) || !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("strict read error = %v; want ErrXRUN and EPIPE", err)
	}
	if got := handle.Stats().Overruns; got != 1 {
		t.Fatalf("overruns = %d; want 1", got)
	}
	if err := handle.SetRecoveryPolicy(RecoveryAutomatic); err != nil {
		t.Fatal(err)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	frames, err := handle.ReadInterleaved(t.Context(), samples)
	if err != nil {
		t.Fatalf("automatic recovery read: %v", err)
	}
	if frames <= 0 {
		t.Fatalf("automatic recovery frames = %d", frames)
	}
	if got := handle.Stats().Overruns; got < 2 {
		t.Fatalf("overruns = %d; want at least 2", got)
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
	if err := handle.Drop(); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if err := handle.FreeHardware(); err != nil {
		t.Fatalf("FreeHardware: %v", err)
	}
	if err := handle.FreeHardware(); err != nil {
		t.Fatalf("second FreeHardware: %v", err)
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
	format := S16LE
	if os.Getenv("GO_ALSA_TEST_FORMAT") == "s32le" {
		format = S32LE
	}
	handle, err := OpenHardware(name, stream)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: format, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800}
	if _, err := handle.ConfigureHardware(config); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.ConfigureSoftwareDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatal(err)
	}
	bytesPerSample, _ := config.Format.PhysicalBytes()
	buffer := make([]byte, config.PeriodFrames*config.Channels*bytesPerSample)
	if stream == alsa.Capture {
		if err := handle.Start(); err != nil {
			t.Fatal(err)
		}
		frames, err := handle.ReadInterleavedBytes(t.Context(), buffer)
		if err != nil {
			t.Fatal(err)
		}
		if frames <= 0 || frames > config.PeriodFrames {
			t.Fatalf("captured frames = %d", frames)
		}
		return
	}
	frames, err := handle.WriteInterleavedBytes(t.Context(), buffer)
	if err != nil {
		t.Fatal(err)
	}
	if frames <= 0 || frames > config.PeriodFrames {
		t.Fatalf("played frames = %d", frames)
	}
}
