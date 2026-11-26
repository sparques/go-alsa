package pcm

import (
	"reflect"
	"testing"

	alsa "github.com/sparques/go-alsa"
)

func TestDefaultRouteMatrix(t *testing.T) {
	tests := []struct {
		source, destination int
		want                []int32
	}{
		{2, 2, []int32{RouteUnity, 0, 0, RouteUnity}},
		{1, 2, []int32{RouteUnity, RouteUnity}},
		{2, 1, []int32{RouteUnity / 2, RouteUnity / 2}},
		{2, 3, []int32{RouteUnity, 0, 0, RouteUnity, 0, 0}},
		{3, 2, []int32{RouteUnity, 0, 0, 0, RouteUnity, 0}},
	}
	for _, test := range tests {
		if got := DefaultRouteMatrix(test.source, test.destination); !reflect.DeepEqual(got, test.want) {
			t.Errorf("DefaultRouteMatrix(%d,%d) = %v; want %v", test.source, test.destination, got, test.want)
		}
	}
	if DefaultRouteMatrix(0, 2) != nil {
		t.Fatal("invalid channel matrix was not nil")
	}
}

func TestPlugConverterDirectionAndParameters(t *testing.T) {
	config := PlugConfig{
		Application: ConversionParams{Format: U8, Channels: 1, Rate: 16000},
		Hardware:    ConversionParams{Format: S16LE, Channels: 2, Rate: 48000},
	}
	playback, err := NewPlugConverter(config, alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	if playback.sourceFormat != U8 || playback.sourceChannels != 1 || playback.destinationFormat != S16LE || playback.destinationChannels != 2 {
		t.Fatalf("playback direction = %#v", playback.StreamConverter)
	}
	capture, err := NewPlugConverter(config, alsa.Capture)
	if err != nil {
		t.Fatal(err)
	}
	if capture.sourceFormat != S16LE || capture.sourceChannels != 2 || capture.destinationFormat != U8 || capture.destinationChannels != 1 {
		t.Fatalf("capture direction = %#v", capture.StreamConverter)
	}
	if capture.ApplicationParams() != config.Application || capture.HardwareParams() != config.Hardware || capture.Stream() != alsa.Capture {
		t.Fatal("plug parameter domains changed for capture")
	}
}

func TestPlugConverterProcessesPlayback(t *testing.T) {
	converter, err := NewPlugConverter(PlugConfig{
		Application: ConversionParams{Format: U8, Channels: 1, Rate: 16000},
		Hardware:    ConversionParams{Format: S16BE, Channels: 2, Rate: 16000},
	}, alsa.Playback)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte{0, 128, 255}
	pending, _ := converter.PendingFrames(3)
	scratchCount, _ := converter.ScratchSamples(3)
	destination := make([]byte, pending*4)
	if _, err := converter.Process(destination, source, make([]int32, scratchCount)); err != nil {
		t.Fatal(err)
	}
	tailFrames, _ := converter.PendingFlushFrames()
	tail := make([]byte, tailFrames*4)
	if _, err := converter.Flush(tail, make([]int32, tailFrames*2)); err != nil {
		t.Fatal(err)
	}
	all := append(destination, tail...)
	want := []byte{0x80, 0, 0x80, 0, 0, 0, 0, 0, 0x7f, 0, 0x7f, 0}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("plug playback = %x; want %x", all, want)
	}
}

func TestPlugConverterValidation(t *testing.T) {
	valid := PlugConfig{
		Application: ConversionParams{Format: S16LE, Channels: 1, Rate: 48000},
		Hardware:    ConversionParams{Format: S16LE, Channels: 1, Rate: 48000},
	}
	if _, err := NewPlugConverter(valid, alsa.Stream(99)); err == nil {
		t.Fatal("invalid stream succeeded")
	}
	invalid := valid
	invalid.Application.Rate = 0
	if _, err := NewPlugConverter(invalid, alsa.Playback); err == nil {
		t.Fatal("invalid application parameters succeeded")
	}
	invalid = valid
	invalid.Matrix = []int32{RouteUnity, RouteUnity}
	if _, err := NewPlugConverter(invalid, alsa.Playback); err == nil {
		t.Fatal("invalid matrix succeeded")
	}
}
