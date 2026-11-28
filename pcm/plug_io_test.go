package pcm

import (
	"context"
	"errors"
	"testing"

	alsa "github.com/sparques/go-alsa"
)

func TestPlugIOValidation(t *testing.T) {
	parameters := ConversionParams{Format: S16LE, Channels: 2, Rate: 48000}
	playback, err := NewPlugConverter(PlugConfig{Application: parameters, Hardware: parameters}, alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := playback.WriteInterleavedBytes(context.Background(), nil, nil, nil, nil); err == nil {
		t.Fatal("playback with nil handle succeeded")
	}
	if _, err := playback.WriteInterleavedBytes(context.Background(), &Handle{stream: streamCapture}, nil, nil, nil); !errors.Is(err, ErrDirection) {
		t.Fatalf("playback direction error = %v", err)
	}
	if _, err := playback.FlushPlayback(context.Background(), &Handle{stream: streamCapture}, nil, nil); !errors.Is(err, ErrDirection) {
		t.Fatalf("playback flush direction error = %v", err)
	}
	capture, err := NewPlugConverter(PlugConfig{Application: parameters, Hardware: parameters}, alsa.Capture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.ReadInterleavedBytes(context.Background(), &Handle{stream: streamPlayback}, nil, nil, nil); !errors.Is(err, ErrDirection) {
		t.Fatalf("capture direction error = %v", err)
	}
}
