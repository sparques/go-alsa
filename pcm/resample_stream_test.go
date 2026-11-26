package pcm

import (
	"reflect"
	"testing"
)

func TestSincStreamMatchesBlockConversion(t *testing.T) {
	converter, err := NewSincConverter(48000, 44100, 32, 1024)
	if err != nil {
		t.Fatal(err)
	}
	source := make([]int32, 480*2)
	for index := range source {
		source[index] = int32((index*7919)%65536-32768) << 15
	}
	frames, _ := converter.OutputFrames(480)
	want := make([]int32, frames*2)
	if _, err := converter.Convert(want, source, 2); err != nil {
		t.Fatal(err)
	}
	stream, err := NewSincStream(converter, 2)
	if err != nil {
		t.Fatal(err)
	}
	var got []int32
	for first := 0; first < 480; {
		chunk := min(37, 480-first)
		pending, err := stream.PendingFrames(chunk)
		if err != nil {
			t.Fatal(err)
		}
		output := make([]int32, pending*2)
		written, err := stream.Process(output, source[first*2:(first+chunk)*2])
		if err != nil || written != pending {
			t.Fatalf("Process = %d, %v; want %d", written, err, pending)
		}
		got = append(got, output...)
		first += chunk
	}
	tail := make([]int32, (frames-len(got)/2)*2)
	written, err := stream.Flush(tail)
	if err != nil || written != len(tail)/2 {
		t.Fatalf("Flush = %d, %v", written, err)
	}
	got = append(got, tail...)
	if !reflect.DeepEqual(got, want) {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("stream sample %d = %d; block = %d", index, got[index], want[index])
			}
		}
	}
}

func TestSincStreamLatencyResetAndAllocation(t *testing.T) {
	converter, _ := NewSincConverter(48000, 16000, 32, 256)
	stream, err := NewSincStream(converter, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stream.LatencySourceFrames() != 16 || stream.LatencyDestinationFrames() != 6 {
		t.Fatalf("latency = %d source, %d destination", stream.LatencySourceFrames(), stream.LatencyDestinationFrames())
	}
	source := make([]int32, 96)
	destination := make([]int32, 32)
	if allocations := testing.AllocsPerRun(100, func() {
		stream.Reset()
		_, _ = stream.Process(destination, source)
	}); allocations != 0 {
		t.Fatalf("stream Process allocations = %f", allocations)
	}
	stream.Reset()
	if _, err := stream.Process(nil, source); err == nil {
		t.Fatal("short destination succeeded")
	}
}

func TestSincStreamFlushState(t *testing.T) {
	converter, _ := NewSincConverter(48000, 44100, 16, 64)
	stream, _ := NewSincStream(converter, 1)
	source := make([]int32, 32)
	pending, _ := stream.PendingFrames(32)
	if _, err := stream.Process(make([]int32, pending), source); err != nil {
		t.Fatal(err)
	}
	target, _ := converter.OutputFrames(32)
	if _, err := stream.Flush(make([]int32, target-pending)); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Process(nil, nil); err == nil {
		t.Fatal("Process after Flush succeeded")
	}
	if _, err := stream.Flush(nil); err == nil {
		t.Fatal("second Flush succeeded")
	}
	stream.Reset()
	if _, err := stream.Process(nil, nil); err != nil {
		t.Fatalf("Process after Reset: %v", err)
	}
}

func FuzzSincStreamMatchesBlock(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5}, uint16(48000), uint16(44100), uint8(3))
	f.Fuzz(func(t *testing.T, input []byte, sourceRateValue, destinationRateValue uint16, chunkValue uint8) {
		sourceRate := int(sourceRateValue%56000) + 8000
		destinationRate := int(destinationRateValue%56000) + 8000
		converter, err := NewSincConverter(sourceRate, destinationRate, 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		source := make([]int32, len(input))
		for index := range input {
			source[index] = int32(int8(input[index])) << 24
		}
		frames, _ := converter.OutputFrames(len(source))
		want := make([]int32, frames)
		if _, err := converter.Convert(want, source, 1); err != nil {
			t.Fatal(err)
		}
		stream, err := NewSincStream(converter, 1)
		if err != nil {
			t.Fatal(err)
		}
		chunkSize := int(chunkValue%32) + 1
		got := make([]int32, 0, frames)
		for first := 0; first < len(source); {
			last := min(first+chunkSize, len(source))
			pending, err := stream.PendingFrames(last - first)
			if err != nil {
				t.Fatal(err)
			}
			output := make([]int32, pending)
			if _, err := stream.Process(output, source[first:last]); err != nil {
				t.Fatal(err)
			}
			got = append(got, output...)
			first = last
		}
		tail := make([]int32, frames-len(got))
		if _, err := stream.Flush(tail); err != nil {
			t.Fatal(err)
		}
		got = append(got, tail...)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("stream output differs from block output")
		}
	})
}
