//go:build linux && (amd64 || arm64)

package pcm

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	alsa "github.com/sparques/go-alsa"
)

// TestHardwareLoopback is an opt-in analog test. It supports devices such as
// the C-Media USB adapter whose playback is stereo while capture is mono.
func TestHardwareLoopback(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_LOOPBACK_PCM")
	if name == "" {
		t.Skip("set GO_ALSA_TEST_LOOPBACK_PCM to a cabled direct hw: PCM")
	}
	const (
		rate         = 48000
		periodFrames = 480
		bufferFrames = 4800
		totalFrames  = rate
		toneHz       = 1000.0
	)

	capture, err := OpenHardware(name, alsa.Capture)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	playback, err := OpenHardware(name, alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	defer playback.Close()

	captureConfig := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 1, Rate: rate, PeriodFrames: periodFrames, BufferFrames: bufferFrames}
	playbackConfig := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: rate, PeriodFrames: periodFrames, BufferFrames: bufferFrames}
	if err := configurePrepared(capture, captureConfig); err != nil {
		t.Fatalf("configure capture: %v", err)
	}
	if err := configurePrepared(playback, playbackConfig); err != nil {
		t.Fatalf("configure playback: %v", err)
	}
	if err := capture.Start(); err != nil {
		t.Fatalf("start capture: %v", err)
	}

	output := make([]int16, totalFrames*playbackConfig.Channels)
	for frame := rate / 4; frame < 3*rate/4; frame++ {
		sample := int16(math.Round(10000 * math.Sin(2*math.Pi*toneHz*float64(frame)/rate)))
		output[frame*2] = sample
		output[frame*2+1] = sample
	}
	input := make([]int16, totalFrames)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	captureDone := make(chan error, 1)
	go func() { captureDone <- readFrames(ctx, capture, input, 1, periodFrames) }()
	if err := writeFrames(ctx, playback, output, 2, periodFrames); err != nil {
		t.Fatalf("write loopback signal: %v", err)
	}
	status, err := playback.Status()
	if err != nil {
		t.Fatalf("playback status: %v", err)
	}
	delay, err := playback.Delay()
	if err != nil {
		t.Fatalf("playback delay: %v", err)
	}
	t.Logf("playback state %s, queued delay %d frames", status.State, delay)
	if delay < 0 {
		t.Fatalf("playback delay = %d; want non-negative queued frames", delay)
	}
	if err := <-captureDone; err != nil {
		t.Fatalf("read loopback signal: %v", err)
	}
	if err := playback.Drain(ctx); err != nil {
		t.Fatalf("drain playback: %v", err)
	}

	oneKHz := toneMagnitude(input, rate, toneHz)
	onePointFiveKHz := toneMagnitude(input, rate, 1500)
	rms := signalRMS(input)
	t.Logf("captured RMS %.1f, 1 kHz magnitude %.1f, 1.5 kHz magnitude %.1f", rms, oneKHz, onePointFiveKHz)
	if rms < 50 {
		t.Fatalf("captured signal is too quiet: RMS %.1f", rms)
	}
	if oneKHz < 5*onePointFiveKHz {
		t.Fatalf("1 kHz loopback tone is not dominant: %.1f vs %.1f", oneKHz, onePointFiveKHz)
	}
	if stats := capture.Stats(); stats.Overruns != 0 {
		t.Fatalf("capture stats = %#v; want no overruns", stats)
	}
	if stats := playback.Stats(); stats.Underruns != 0 {
		t.Fatalf("playback stats = %#v; want no underruns", stats)
	}
}

func TestHardwareLifecycle(t *testing.T) {
	name := os.Getenv("GO_ALSA_TEST_LOOPBACK_PCM")
	if name == "" {
		t.Skip("set GO_ALSA_TEST_LOOPBACK_PCM to a direct hw: PCM")
	}
	const (
		channels     = 2
		periodFrames = 480
		bufferFrames = 4800
	)
	handle, err := OpenHardware(name, alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	config := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: channels, Rate: 48000, PeriodFrames: periodFrames, BufferFrames: bufferFrames}
	setup, err := handle.ConfigureHardware(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.ConfigureSoftwareDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := handle.SyncHardware(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Reset(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := writeFrames(ctx, handle, make([]int16, bufferFrames*channels), channels, periodFrames); err != nil {
		t.Fatal(err)
	}
	if setup.CanPause() {
		if err := handle.Pause(true); err != nil {
			t.Fatalf("pause: %v", err)
		}
		if err := handle.Pause(false); err != nil {
			t.Fatalf("release pause: %v", err)
		}
	}
	rewound, err := handle.Rewind(periodFrames)
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if rewound > periodFrames {
		t.Fatalf("rewound %d frames; requested %d", rewound, periodFrames)
	}
	forwarded, err := handle.Forward(rewound)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if forwarded > rewound {
		t.Fatalf("forwarded %d frames; requested %d", forwarded, rewound)
	}
	if err := handle.Drop(); err != nil {
		t.Fatal(err)
	}
	t.Logf("capabilities pause=%t resume=%t, rewound=%d forwarded=%d", setup.CanPause(), setup.CanResume(), rewound, forwarded)
}

func configurePrepared(handle *Handle, config HardwareConfig) error {
	if _, err := handle.ConfigureHardware(config); err != nil {
		return err
	}
	if _, err := handle.ConfigureSoftwareDefaults(); err != nil {
		return err
	}
	return handle.Prepare()
}

func writeFrames(ctx context.Context, handle *Handle, samples []int16, channels, periodFrames int) error {
	for offset := 0; offset < len(samples); {
		end := min(offset+periodFrames*channels, len(samples))
		frames, err := handle.WriteInterleaved(ctx, samples[offset:end])
		if err != nil {
			return err
		}
		if frames <= 0 {
			return fmt.Errorf("write returned %d frames", frames)
		}
		offset += frames * channels
	}
	return nil
}

func readFrames(ctx context.Context, handle *Handle, samples []int16, channels, periodFrames int) error {
	for offset := 0; offset < len(samples); {
		end := min(offset+periodFrames*channels, len(samples))
		frames, err := handle.ReadInterleaved(ctx, samples[offset:end])
		if err != nil {
			return err
		}
		if frames <= 0 {
			return fmt.Errorf("read returned %d frames", frames)
		}
		offset += frames * channels
	}
	return nil
}

func toneMagnitude(samples []int16, rate int, frequency float64) float64 {
	var real, imaginary float64
	for index, sample := range samples {
		angle := 2 * math.Pi * frequency * float64(index) / float64(rate)
		real += float64(sample) * math.Cos(angle)
		imaginary -= float64(sample) * math.Sin(angle)
	}
	return 2 * math.Hypot(real, imaginary) / float64(len(samples))
}

func signalRMS(samples []int16) float64 {
	var sum float64
	for _, sample := range samples {
		value := float64(sample)
		sum += value * value
	}
	return math.Sqrt(sum / float64(len(samples)))
}
