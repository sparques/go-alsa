//go:build linux && (amd64 || arm64)

package pcm

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	alsa "github.com/sparques/go-alsa"
)

// TestKernelLoopback exercises a deterministic snd-aloop path. Device 0
// playback is connected by the module to device 1 capture at the same
// subdevice. Stable ALSA card IDs avoid depending on probe-order card numbers.
func TestKernelLoopback(t *testing.T) {
	if os.Getenv("GO_ALSA_TEST_ALOOP") != "1" {
		t.Skip("set GO_ALSA_TEST_ALOOP=1 when snd-aloop is loaded")
	}
	playbackName := testSelector("GO_ALSA_TEST_ALOOP_PLAYBACK_PCM", "hw:CARD=Loopback,DEV=0,SUBDEV=0")
	captureName := testSelector("GO_ALSA_TEST_ALOOP_CAPTURE_PCM", "hw:CARD=Loopback,DEV=1,SUBDEV=0")
	const (
		rate         = 48000
		channels     = 2
		periodFrames = 480
		bufferFrames = 4800
		totalFrames  = rate / 2
		toneHz       = 1000.0
	)
	config := HardwareConfig{
		Access: RWInterleaved, Format: S16LE, Channels: channels, Rate: rate,
		PeriodFrames: periodFrames, BufferFrames: bufferFrames,
	}

	capture, err := OpenHardware(captureName, alsa.Capture)
	if err != nil {
		t.Fatalf("open capture %q: %v", captureName, err)
	}
	defer capture.Close()
	playback, err := OpenHardware(playbackName, alsa.Playback)
	if err != nil {
		t.Fatalf("open playback %q: %v", playbackName, err)
	}
	defer playback.Close()
	if err := configurePrepared(capture, config); err != nil {
		t.Fatalf("configure capture: %v", err)
	}
	if err := configurePrepared(playback, config); err != nil {
		t.Fatalf("configure playback: %v", err)
	}
	if err := capture.Start(); err != nil {
		t.Fatalf("start capture: %v", err)
	}

	output := make([]int16, totalFrames*channels)
	for frame := 0; frame < totalFrames; frame++ {
		sample := int16(math.Round(10000 * math.Sin(2*math.Pi*toneHz*float64(frame)/rate)))
		for channel := 0; channel < channels; channel++ {
			output[frame*channels+channel] = sample
		}
	}
	input := make([]int16, len(output))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	captureDone := make(chan error, 1)
	go func() { captureDone <- readFrames(ctx, capture, input, channels, periodFrames) }()
	if err := writeFrames(ctx, playback, output, channels, periodFrames); err != nil {
		t.Fatalf("write loopback signal: %v", err)
	}
	if err := <-captureDone; err != nil {
		t.Fatalf("read loopback signal: %v", err)
	}

	for channel := 0; channel < channels; channel++ {
		channelSamples := make([]int16, totalFrames)
		for frame := range channelSamples {
			channelSamples[frame] = input[frame*channels+channel]
		}
		magnitude := toneMagnitude(channelSamples, rate, toneHz)
		offTone := toneMagnitude(channelSamples, rate, 1500)
		t.Logf("channel %d: RMS %.1f, 1 kHz %.1f, 1.5 kHz %.1f", channel, signalRMS(channelSamples), magnitude, offTone)
		if magnitude < 1000 || magnitude < 10*offTone {
			t.Fatalf("channel %d did not preserve the looped tone", channel)
		}
	}
}

// TestDummyHardwareLifecycle exercises negotiation and state transitions on
// snd-dummy without requiring physical audio hardware.
func TestDummyHardwareLifecycle(t *testing.T) {
	if os.Getenv("GO_ALSA_TEST_DUMMY") != "1" {
		t.Skip("set GO_ALSA_TEST_DUMMY=1 when snd-dummy is loaded")
	}
	name := testSelector("GO_ALSA_TEST_DUMMY_PCM", "hw:CARD=Dummy,DEV=0,SUBDEV=0")
	handle, err := OpenHardware(name, alsa.Playback)
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800}
	if err := configurePrepared(handle, config); err != nil {
		t.Fatal(err)
	}
	if status, err := handle.Status(); err != nil || status.State != StatePrepared {
		t.Fatalf("prepared status = %#v, %v", status, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := writeFrames(ctx, handle, make([]int16, 4800*2), 2, 480); err != nil {
		t.Fatal(err)
	}
	if err := handle.Drop(); err != nil {
		t.Fatal(err)
	}
	if err := handle.FreeHardware(); err != nil {
		t.Fatal(err)
	}
	if status, err := handle.Status(); err != nil || status.State != StateOpen {
		t.Fatalf("freed status = %#v, %v", status, err)
	}
}

func testSelector(environment, fallback string) string {
	if value := os.Getenv(environment); value != "" {
		return value
	}
	return fallback
}
