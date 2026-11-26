package pcm

import (
	"reflect"
	"testing"
)

func TestRouteInterleavedIdentityAndDuplication(t *testing.T) {
	source := []int32{1, -2, 3, -4}
	identity := []int32{RouteUnity, 0, 0, RouteUnity}
	destination := make([]int32, len(source))
	frames, err := RouteInterleaved(destination, 2, source, 2, identity)
	if err != nil || frames != 2 || !reflect.DeepEqual(destination, source) {
		t.Fatalf("identity route = %v, %d, %v", destination, frames, err)
	}
	mono := []int32{100, -200}
	stereo := make([]int32, 4)
	frames, err = RouteInterleaved(stereo, 2, mono, 1, []int32{RouteUnity, RouteUnity})
	if err != nil || frames != 2 || !reflect.DeepEqual(stereo, []int32{100, 100, -200, -200}) {
		t.Fatalf("duplication route = %v, %d, %v", stereo, frames, err)
	}
}

func TestRouteInterleavedAverageAndSilence(t *testing.T) {
	source := []int32{100, 300, -400, 200}
	destination := make([]int32, 4)
	matrix := []int32{
		RouteUnity / 2, RouteUnity / 2,
		0, 0,
	}
	frames, err := RouteInterleaved(destination, 2, source, 2, matrix)
	want := []int32{200, 0, -100, 0}
	if err != nil || frames != 2 || !reflect.DeepEqual(destination, want) {
		t.Fatalf("average/silence route = %v, %d, %v; want %v", destination, frames, err, want)
	}
}

func TestRouteInterleavedSaturates(t *testing.T) {
	source := []int32{maxInt32, maxInt32, minInt32, minInt32}
	destination := make([]int32, 2)
	frames, err := RouteInterleaved(destination, 1, source, 2, []int32{RouteUnity, RouteUnity})
	if err != nil || frames != 2 || !reflect.DeepEqual(destination, []int32{maxInt32, minInt32}) {
		t.Fatalf("saturating route = %v, %d, %v", destination, frames, err)
	}
}

func TestRouteInterleavedValidation(t *testing.T) {
	tests := []struct {
		destination         []int32
		destinationChannels int
		source              []int32
		sourceChannels      int
		matrix              []int32
	}{
		{nil, 1, nil, 0, nil},
		{nil, 0, nil, 1, nil},
		{nil, 1, []int32{1}, 2, nil},
		{nil, 1, []int32{1}, 1, nil},
		{nil, 2, []int32{1}, 1, []int32{RouteUnity, RouteUnity}},
	}
	for index, test := range tests {
		if _, err := RouteInterleaved(test.destination, test.destinationChannels, test.source, test.sourceChannels, test.matrix); err == nil {
			t.Errorf("validation case %d succeeded", index)
		}
	}
}

func FuzzRouteInterleaved(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3}, uint8(2), uint8(2))
	f.Fuzz(func(t *testing.T, bytes []byte, sourceByte, destinationByte uint8) {
		sourceChannels := int(sourceByte%8) + 1
		destinationChannels := int(destinationByte%8) + 1
		frames := len(bytes) / sourceChannels
		source := make([]int32, frames*sourceChannels)
		for index := range source {
			source[index] = int32(int8(bytes[index])) << 24
		}
		matrix := make([]int32, sourceChannels*destinationChannels)
		for index := range matrix {
			matrix[index] = int32((index%5)-2) * (RouteUnity / 2)
		}
		destination := make([]int32, frames*destinationChannels)
		if got, err := RouteInterleaved(destination, destinationChannels, source, sourceChannels, matrix); err != nil || got != frames {
			t.Fatalf("RouteInterleaved = %d, %v; want %d", got, err, frames)
		}
	})
}
