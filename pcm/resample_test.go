package pcm

import (
	"math"
	"testing"
)

func TestSincConverterFrameAccountingAndIdentity(t *testing.T) {
	converter, err := NewSincConverter(48000, 44100, 32, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if frames, err := converter.OutputFrames(480); err != nil || frames != 441 {
		t.Fatalf("OutputFrames = %d, %v; want 441", frames, err)
	}
	identity, err := NewSincConverter(48000, 48000, 32, 1024)
	if err != nil {
		t.Fatal(err)
	}
	source := []int32{1, -2, 3, -4}
	destination := make([]int32, len(source))
	if frames, err := identity.Convert(destination, source, 2); err != nil || frames != 2 {
		t.Fatalf("identity Convert = %d, %v", frames, err)
	}
	for index := range source {
		if destination[index] != source[index] {
			t.Fatalf("identity sample %d = %d; want %d", index, destination[index], source[index])
		}
	}
}

func TestSincConverterPreservesInteriorDC(t *testing.T) {
	converter, err := NewSincConverter(48000, 44100, 32, 1024)
	if err != nil {
		t.Fatal(err)
	}
	source := make([]int32, 480)
	for index := range source {
		source[index] = 1 << 29
	}
	destination := make([]int32, 441)
	if _, err := converter.Convert(destination, source, 1); err != nil {
		t.Fatal(err)
	}
	for index := 32; index < len(destination)-32; index++ {
		if difference := math.Abs(float64(destination[index] - (1 << 29))); difference > 2 {
			t.Fatalf("DC sample %d differs by %f", index, difference)
		}
	}
}

func TestSincConverterAttenuatesDownsampleAlias(t *testing.T) {
	converter, err := NewSincConverter(48000, 16000, 64, 2048)
	if err != nil {
		t.Fatal(err)
	}
	source := make([]int32, 480)
	for index := range source {
		if index%2 == 0 {
			source[index] = 1 << 29
		} else {
			source[index] = -(1 << 29)
		}
	}
	destination := make([]int32, 160)
	if _, err := converter.Convert(destination, source, 1); err != nil {
		t.Fatal(err)
	}
	var peak int32
	for _, sample := range destination[24 : len(destination)-24] {
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	if peak > 1<<18 {
		t.Fatalf("downsampled Nyquist alias peak = %d", peak)
	}
}

func TestSincConverterAllocationAndValidation(t *testing.T) {
	converter, err := NewSincConverter(48000, 44100, 32, 1024)
	if err != nil {
		t.Fatal(err)
	}
	source := make([]int32, 480)
	destination := make([]int32, 441)
	if allocations := testing.AllocsPerRun(100, func() { _, _ = converter.Convert(destination, source, 1) }); allocations != 0 {
		t.Fatalf("Convert allocations = %f", allocations)
	}
	for _, test := range []struct{ sourceRate, destinationRate, taps, phases int }{
		{0, 48000, 32, 1024}, {48000, 0, 32, 1024}, {48000, 44100, 7, 1024}, {48000, 44100, 31, 1024}, {48000, 44100, 32, 8},
	} {
		if _, err := NewSincConverter(test.sourceRate, test.destinationRate, test.taps, test.phases); err == nil {
			t.Errorf("NewSincConverter%v succeeded", test)
		}
	}
	if _, err := converter.Convert(nil, []int32{1}, 2); err == nil {
		t.Fatal("partial source frame succeeded")
	}
}

func FuzzSincConverter(f *testing.F) {
	f.Add([]byte{0, 1, 255}, uint16(48000), uint16(44100), uint8(1))
	f.Fuzz(func(t *testing.T, input []byte, sourceRateValue, destinationRateValue uint16, channelValue uint8) {
		sourceRate := int(sourceRateValue%64000) + 8000
		destinationRate := int(destinationRateValue%64000) + 8000
		channels := int(channelValue%4) + 1
		source := make([]int32, len(input))
		for index := range input {
			source[index] = int32(int8(input[index])) << 24
		}
		source = source[:len(source)/channels*channels]
		converter, err := NewSincConverter(sourceRate, destinationRate, 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		frames, err := converter.OutputFrames(len(source) / channels)
		if err != nil {
			t.Fatal(err)
		}
		destination := make([]int32, frames*channels)
		if got, err := converter.Convert(destination, source, channels); err != nil || got != frames {
			t.Fatalf("Convert = %d, %v; want %d", got, err, frames)
		}
	})
}
