package pcm

import (
	"bytes"
	"testing"
)

func TestConvertRouteInterleavedStereoToMono(t *testing.T) {
	// Two S16_LE stereo frames: full-scale opposing values, then 0.5 + 0.5.
	source := []byte{0xff, 0x7f, 0x00, 0x80, 0x00, 0x40, 0x00, 0x40}
	destination := make([]byte, 2)
	scratch := make([]int32, 6)
	frames, err := ConvertRouteInterleaved(destination, U8, 1, source, S16LE, 2,
		[]int32{RouteUnity / 2, RouteUnity / 2}, scratch)
	if err != nil || frames != 2 || !bytes.Equal(destination, []byte{127, 192}) {
		t.Fatalf("ConvertRouteInterleaved = %v, %d, %v; want [127 192]", destination, frames, err)
	}
}

func TestConvertRouteInterleavedMonoToStereoEndian(t *testing.T) {
	source := []byte{0x80, 0x7f}
	destination := make([]byte, 8)
	scratch := make([]int32, 6)
	frames, err := ConvertRouteInterleaved(destination, S16BE, 2, source, S8, 1,
		[]int32{RouteUnity, RouteUnity}, scratch)
	want := []byte{0x80, 0, 0x80, 0, 0x7f, 0, 0x7f, 0}
	if err != nil || frames != 2 || !bytes.Equal(destination, want) {
		t.Fatalf("ConvertRouteInterleaved = %x, %d, %v; want %x", destination, frames, err, want)
	}
}

func TestConvertRouteScratchAndValidation(t *testing.T) {
	if got, err := ConvertRouteScratchSamples(10, 2, 1); err != nil || got != 30 {
		t.Fatalf("ConvertRouteScratchSamples = %d, %v", got, err)
	}
	if _, err := ConvertRouteScratchSamples(-1, 2, 1); err == nil {
		t.Fatal("negative scratch frame count succeeded")
	}
	matrix := []int32{RouteUnity}
	if _, err := ConvertRouteInterleaved(nil, S16LE, 1, []byte{0}, S16LE, 1, matrix, nil); err == nil {
		t.Fatal("partial input frame succeeded")
	}
	if _, err := ConvertRouteInterleaved(make([]byte, 2), S16LE, 1, []byte{0, 0}, S16LE, 1, matrix, nil); err == nil {
		t.Fatal("short scratch succeeded")
	}
	if _, err := ConvertRouteInterleaved(nil, S16LE, 1, []byte{0, 0}, S16LE, 1, matrix, make([]int32, 2)); err == nil {
		t.Fatal("short destination succeeded")
	}
}

func FuzzConvertRouteInterleaved(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0}, uint8(2), uint8(1), uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, input []byte, sourceByte, destinationByte, sourceChannelByte, destinationChannelByte uint8) {
		sourceFormat := integerFormats[int(sourceByte)%len(integerFormats)]
		destinationFormat := integerFormats[int(destinationByte)%len(integerFormats)]
		sourceChannels := int(sourceChannelByte%4) + 1
		destinationChannels := int(destinationChannelByte%4) + 1
		sourceWidth, _ := sourceFormat.PhysicalBytes()
		frameBytes := sourceWidth * sourceChannels
		input = input[:len(input)/frameBytes*frameBytes]
		frames := len(input) / frameBytes
		destinationWidth, _ := destinationFormat.PhysicalBytes()
		destination := make([]byte, frames*destinationChannels*destinationWidth)
		scratchCount, err := ConvertRouteScratchSamples(frames, sourceChannels, destinationChannels)
		if err != nil {
			t.Fatal(err)
		}
		scratch := make([]int32, scratchCount)
		matrix := make([]int32, sourceChannels*destinationChannels)
		for channel := 0; channel < min(sourceChannels, destinationChannels); channel++ {
			matrix[channel*sourceChannels+channel] = RouteUnity
		}
		got, err := ConvertRouteInterleaved(destination, destinationFormat, destinationChannels, input, sourceFormat, sourceChannels, matrix, scratch)
		if err != nil || got != frames {
			t.Fatalf("ConvertRouteInterleaved = %d, %v; want %d", got, err, frames)
		}
	})
}
