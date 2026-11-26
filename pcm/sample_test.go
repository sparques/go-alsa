package pcm

import (
	"bytes"
	"testing"
)

var integerFormats = []Format{
	S8, U8, S16LE, S16BE, U16LE, U16BE,
	S24LE, S24BE, U24LE, U24BE, S24_3LE, S24_3BE, U24_3LE, U24_3BE,
	S32LE, S32BE, U32LE, U32BE,
}

func TestSampleCodecRoundTrip(t *testing.T) {
	samples := []int32{-1 << 31, -123456789, -1, 0, 1, 123456789, 1<<31 - 1}
	for _, format := range integerFormats {
		t.Run(format.String(), func(t *testing.T) {
			layout, err := integerLayout(format)
			if err != nil {
				t.Fatal(err)
			}
			encoded := make([]byte, len(samples)*layout.bytes)
			if count, err := EncodeSamples(encoded, samples, format); err != nil || count != len(samples) {
				t.Fatalf("EncodeSamples = %d, %v", count, err)
			}
			decoded := make([]int32, len(samples))
			if count, err := DecodeSamples(decoded, encoded, format); err != nil || count != len(samples) {
				t.Fatalf("DecodeSamples = %d, %v", count, err)
			}
			shift := 32 - layout.bits
			for index, sample := range samples {
				want := sample >> shift << shift
				if decoded[index] != want {
					t.Errorf("sample %d = %d; want %d", index, decoded[index], want)
				}
			}
		})
	}
}

func TestPacked24SignExtensionAndEndian(t *testing.T) {
	tests := []struct {
		format Format
		input  []byte
		want   int32
	}{
		{S24_3LE, []byte{0, 0, 0x80}, -1 << 31},
		{S24_3BE, []byte{0x80, 0, 0}, -1 << 31},
		{U24_3LE, []byte{0, 0, 0}, -1 << 31},
		{U24_3BE, []byte{0xff, 0xff, 0xff}, 0x7fffff00},
	}
	for _, test := range tests {
		var sample [1]int32
		if _, err := DecodeSamples(sample[:], test.input, test.format); err != nil || sample[0] != test.want {
			t.Errorf("DecodeSamples(%s) = %#x, %v; want %#x", test.format, sample[0], err, test.want)
		}
	}
}

func TestConvertSamplesEndianAndWidth(t *testing.T) {
	source := []byte{0x00, 0x80, 0xff, 0x7f}
	destination := make([]byte, 8)
	count, err := ConvertSamples(destination, S32BE, source, S16LE)
	want := []byte{0x80, 0, 0, 0, 0x7f, 0xff, 0, 0}
	if err != nil || count != 2 || !bytes.Equal(destination, want) {
		t.Fatalf("ConvertSamples = %x, %d, %v; want %x", destination, count, err, want)
	}
}

func TestSampleCodecValidation(t *testing.T) {
	if _, err := DecodeSamples(make([]int32, 1), []byte{1}, S16LE); err == nil {
		t.Fatal("DecodeSamples accepted partial sample")
	}
	if _, err := DecodeSamples(nil, []byte{1}, U8); err == nil {
		t.Fatal("DecodeSamples accepted short destination")
	}
	if _, err := EncodeSamples(nil, []int32{1}, S16LE); err == nil {
		t.Fatal("EncodeSamples accepted short destination")
	}
	if _, err := ConvertSamples(make([]byte, 1), U8, []byte{0, 0}, S16LE); err != nil {
		t.Fatalf("ConvertSamples exact destination: %v", err)
	}
	if _, err := ConvertSamples(nil, U8, []byte{0, 0}, S16LE); err == nil {
		t.Fatal("ConvertSamples accepted short destination")
	}
	if _, err := DecodeSamples(nil, nil, Format(999)); err == nil {
		t.Fatal("DecodeSamples accepted unsupported format")
	}
}

func FuzzSampleConversion(f *testing.F) {
	f.Add([]byte{0, 0, 0x80}, uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, input []byte, sourceIndex, destinationIndex uint8) {
		sourceFormat := integerFormats[int(sourceIndex)%len(integerFormats)]
		destinationFormat := integerFormats[int(destinationIndex)%len(integerFormats)]
		sourceBytes, _ := sourceFormat.PhysicalBytes()
		input = input[:len(input)/sourceBytes*sourceBytes]
		count := len(input) / sourceBytes
		destinationBytes, _ := destinationFormat.PhysicalBytes()
		output := make([]byte, count*destinationBytes)
		converted, err := ConvertSamples(output, destinationFormat, input, sourceFormat)
		if err != nil || converted != count {
			t.Fatalf("ConvertSamples = %d, %v; want %d", converted, err, count)
		}
	})
}
