//go:build linux && (amd64 || arm64)

package pcm

import (
	"context"
	"encoding/binary"
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

// TestKernelLoopbackPlugCapture exercises the complete capture direction of a
// plug plan: kernel reads, stereo-to-mono routing, 48-to-16 kHz resampling, and
// application-format encoding. Configuring playback first fixes snd-aloop's
// cable parameters so capture negotiation must adapt to the existing stream.
func TestKernelLoopbackPlugCapture(t *testing.T) {
	if os.Getenv("GO_ALSA_TEST_ALOOP") != "1" {
		t.Skip("set GO_ALSA_TEST_ALOOP=1 when snd-aloop is loaded")
	}
	playbackName := testSelector("GO_ALSA_TEST_ALOOP_PLAYBACK_PCM", "hw:CARD=Loopback,DEV=0,SUBDEV=1")
	captureName := testSelector("GO_ALSA_TEST_ALOOP_CAPTURE_PCM", "hw:CARD=Loopback,DEV=1,SUBDEV=1")
	const (
		hardwareRate = 48000
		appRate      = 16000
		channels     = 2
		periodFrames = 480
		bufferFrames = 4800
		periods      = 50
		toneHz       = 1000.0
	)

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
	playbackConfig := HardwareConfig{
		Access: RWInterleaved, Format: S16LE, Channels: channels, Rate: hardwareRate,
		PeriodFrames: periodFrames, BufferFrames: bufferFrames,
	}
	if err := configurePrepared(playback, playbackConfig); err != nil {
		t.Fatalf("configure playback: %v", err)
	}
	application := ConversionParams{Format: S16LE, Channels: 1, Rate: appRate}
	plan := NamedPlan{Stream: alsa.Capture, RequiresPlug: true}
	setup, converter, err := plan.ConfigureHandle(capture, PlugHardwareRequest{
		Application: application, PeriodFrames: periodFrames, BufferFrames: bufferFrames,
	}, nil)
	if err != nil {
		t.Fatalf("configure capture plug: %v", err)
	}
	if setup.Format != S16LE || setup.Channels != channels || setup.Rate != hardwareRate {
		t.Fatalf("capture hardware setup = %#v; want S16_LE stereo/48 kHz", setup.HardwareConfig)
	}
	if _, err := capture.ConfigureSoftwareDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := capture.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := capture.Start(); err != nil {
		t.Fatal(err)
	}

	output := make([]int16, periods*periodFrames*channels)
	for frame := 0; frame < periods*periodFrames; frame++ {
		sample := int16(math.Round(10000 * math.Sin(2*math.Pi*toneHz*float64(frame)/hardwareRate)))
		output[frame*channels] = sample
		output[frame*channels+1] = sample
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	playbackDone := make(chan error, 1)
	go func() { playbackDone <- writeFrames(ctx, playback, output, channels, periodFrames) }()

	hardwareFrameBytes := channels * 2
	var applicationBytes []byte
	for hardwareFrames := 0; hardwareFrames < periods*periodFrames; {
		requestFrames := min(periodFrames, periods*periodFrames-hardwareFrames)
		hardwareBuffer := make([]byte, requestFrames*hardwareFrameBytes)
		pending, err := converter.PendingFrames(requestFrames)
		if err != nil {
			t.Fatal(err)
		}
		scratchSamples, err := converter.ScratchSamples(requestFrames)
		if err != nil {
			t.Fatal(err)
		}
		converted := make([]byte, pending*2)
		frames, consumed, err := converter.ReadInterleavedBytes(ctx, capture, converted, hardwareBuffer, make([]int32, scratchSamples))
		if err != nil {
			t.Fatalf("converted capture: %v", err)
		}
		if consumed <= 0 {
			t.Fatalf("converted capture consumed %d hardware frames", consumed)
		}
		hardwareFrames += consumed
		applicationBytes = append(applicationBytes, converted[:frames*2]...)
	}
	if err := <-playbackDone; err != nil {
		t.Fatalf("playback: %v", err)
	}
	tailFrames, err := converter.PendingFlushFrames()
	if err != nil {
		t.Fatal(err)
	}
	tail := make([]byte, tailFrames*2)
	if _, err := converter.Flush(tail, make([]int32, tailFrames)); err != nil {
		t.Fatal(err)
	}
	applicationBytes = append(applicationBytes, tail...)
	wantBytes := periods * periodFrames * appRate / hardwareRate * 2
	if len(applicationBytes) != wantBytes {
		t.Fatalf("application bytes = %d; want %d", len(applicationBytes), wantBytes)
	}
	samples := make([]int16, len(applicationBytes)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(applicationBytes[index*2:]))
	}
	magnitude := toneMagnitude(samples, appRate, toneHz)
	offTone := toneMagnitude(samples, appRate, 1500)
	t.Logf("plug capture: %d frames, RMS %.1f, 1 kHz %.1f, 1.5 kHz %.1f", len(samples), signalRMS(samples), magnitude, offTone)
	if magnitude < 5000 || magnitude < 10*offTone {
		t.Fatalf("converted capture did not preserve the looped tone")
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
