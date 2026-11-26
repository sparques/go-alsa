package pcm

import (
	"bytes"
	"testing"
)

func TestStreamConverterMatchesComposedReference(t *testing.T) {
	converter, _ := NewSincConverter(48000, 16000, 32, 1024)
	matrix := []int32{RouteUnity / 2, RouteUnity / 2}
	stream, err := NewStreamConverter(S16LE, 2, U8, 1, matrix, converter)
	if err != nil {
		t.Fatal(err)
	}
	sourceSamples := make([]int32, 480*2)
	for frame := 0; frame < 480; frame++ {
		sourceSamples[frame*2] = int32(frame-240) << 20
		sourceSamples[frame*2+1] = int32(240-frame) << 19
	}
	source := make([]byte, len(sourceSamples)*2)
	if _, err := EncodeSamples(source, sourceSamples, S16LE); err != nil {
		t.Fatal(err)
	}
	decoded := make([]int32, len(sourceSamples))
	routed := make([]int32, 480)
	if _, err := DecodeSamples(decoded, source, S16LE); err != nil {
		t.Fatal(err)
	}
	if _, err := RouteInterleaved(routed, 1, decoded, 2, matrix); err != nil {
		t.Fatal(err)
	}
	frames, _ := converter.OutputFrames(480)
	resampled := make([]int32, frames)
	if _, err := converter.Convert(resampled, routed, 1); err != nil {
		t.Fatal(err)
	}
	want := make([]byte, frames)
	if _, err := EncodeSamples(want, resampled, U8); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for first := 0; first < 480; {
		chunk := min(43, 480-first)
		pending, _ := stream.PendingFrames(chunk)
		scratchCount, _ := stream.ScratchSamples(chunk)
		output := make([]byte, pending)
		if written, err := stream.Process(output, source[first*4:(first+chunk)*4], make([]int32, scratchCount)); err != nil || written != pending {
			t.Fatalf("Process = %d, %v", written, err)
		}
		got = append(got, output...)
		first += chunk
	}
	tailFrames, _ := stream.PendingFlushFrames()
	tail := make([]byte, tailFrames)
	if written, err := stream.Flush(tail, make([]int32, tailFrames)); err != nil || written != tailFrames {
		t.Fatalf("Flush = %d, %v", written, err)
	}
	got = append(got, tail...)
	if !bytes.Equal(got, want) {
		t.Fatalf("stream conversion differs from composed reference")
	}
}

func TestStreamConverterAllocationAndValidation(t *testing.T) {
	converter, _ := NewSincConverter(48000, 16000, 16, 64)
	stream, err := NewStreamConverter(S16LE, 1, S16LE, 1, []int32{RouteUnity}, converter)
	if err != nil {
		t.Fatal(err)
	}
	source := make([]byte, 192)
	pending, _ := stream.PendingFrames(96)
	scratchCount, _ := stream.ScratchSamples(96)
	destination := make([]byte, pending*2)
	scratch := make([]int32, scratchCount)
	if allocations := testing.AllocsPerRun(100, func() {
		stream.Reset()
		_, _ = stream.Process(destination, source, scratch)
	}); allocations != 0 {
		t.Fatalf("stream converter allocations = %f", allocations)
	}
	stream.Reset()
	if _, err := stream.Process(nil, source[:1], nil); err == nil {
		t.Fatal("partial frame succeeded")
	}
	if _, err := NewStreamConverter(Format(999), 1, S16LE, 1, []int32{RouteUnity}, converter); err == nil {
		t.Fatal("unsupported source format succeeded")
	}
	if _, err := NewStreamConverter(S16LE, 1, S16LE, 2, []int32{RouteUnity}, converter); err == nil {
		t.Fatal("invalid matrix succeeded")
	}
}

func FuzzStreamConverter(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0, 2, 0, 3, 0}, uint8(2))
	f.Fuzz(func(t *testing.T, input []byte, chunkValue uint8) {
		input = input[:len(input)/4*4]
		converter, err := NewSincConverter(48000, 16000, 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		matrix := []int32{RouteUnity / 2, RouteUnity / 2}
		stream, err := NewStreamConverter(S16LE, 2, U8, 1, matrix, converter)
		if err != nil {
			t.Fatal(err)
		}
		frames := len(input) / 4
		decoded := make([]int32, frames*2)
		routed := make([]int32, frames)
		_, _ = DecodeSamples(decoded, input, S16LE)
		_, _ = RouteInterleaved(routed, 1, decoded, 2, matrix)
		outputFrames, _ := converter.OutputFrames(frames)
		resampled := make([]int32, outputFrames)
		_, _ = converter.Convert(resampled, routed, 1)
		want := make([]byte, outputFrames)
		_, _ = EncodeSamples(want, resampled, U8)
		chunkFrames := int(chunkValue%32) + 1
		got := make([]byte, 0, outputFrames)
		for first := 0; first < frames; {
			last := min(first+chunkFrames, frames)
			pending, _ := stream.PendingFrames(last - first)
			scratchCount, _ := stream.ScratchSamples(last - first)
			output := make([]byte, pending)
			if _, err := stream.Process(output, input[first*4:last*4], make([]int32, scratchCount)); err != nil {
				t.Fatal(err)
			}
			got = append(got, output...)
			first = last
		}
		tailFrames, _ := stream.PendingFlushFrames()
		tail := make([]byte, tailFrames)
		if _, err := stream.Flush(tail, make([]int32, tailFrames)); err != nil {
			t.Fatal(err)
		}
		got = append(got, tail...)
		if !bytes.Equal(got, want) {
			t.Fatalf("stream conversion differs from reference")
		}
	})
}
