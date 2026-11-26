package pcm

import (
	"encoding/binary"
	"fmt"
)

// DecodeSamples converts physical integer PCM samples to signed Q1.31 values.
// It performs no allocation and returns the number of decoded samples.
func DecodeSamples(destination []int32, source []byte, format Format) (int, error) {
	layout, err := integerLayout(format)
	if err != nil {
		return 0, err
	}
	if len(source)%layout.bytes != 0 {
		return 0, fmt.Errorf("alsa pcm: %s input contains a partial sample", format)
	}
	count := len(source) / layout.bytes
	if len(destination) < count {
		return 0, fmt.Errorf("alsa pcm: sample destination holds %d samples, need %d", len(destination), count)
	}
	for index := 0; index < count; index++ {
		raw := readPhysical(source[index*layout.bytes:], layout)
		destination[index] = decodeInteger(raw, layout.bits, layout.signed)
	}
	return count, nil
}

// EncodeSamples converts signed Q1.31 values to physical integer PCM samples.
// Low-order precision not representable by the destination is discarded.
func EncodeSamples(destination []byte, source []int32, format Format) (int, error) {
	layout, err := integerLayout(format)
	if err != nil {
		return 0, err
	}
	needed := len(source) * layout.bytes
	if len(destination) < needed {
		return 0, fmt.Errorf("alsa pcm: byte destination holds %d bytes, need %d", len(destination), needed)
	}
	for index, sample := range source {
		raw := encodeInteger(sample, layout.bits, layout.signed)
		writePhysical(destination[index*layout.bytes:], raw, layout)
	}
	return len(source), nil
}

// ConvertSamples converts between supported integer PCM formats without
// allocating. Source and destination must not overlap.
func ConvertSamples(destination []byte, destinationFormat Format, source []byte, sourceFormat Format) (int, error) {
	sourceLayout, err := integerLayout(sourceFormat)
	if err != nil {
		return 0, err
	}
	destinationLayout, err := integerLayout(destinationFormat)
	if err != nil {
		return 0, err
	}
	if len(source)%sourceLayout.bytes != 0 {
		return 0, fmt.Errorf("alsa pcm: %s input contains a partial sample", sourceFormat)
	}
	count := len(source) / sourceLayout.bytes
	needed := count * destinationLayout.bytes
	if len(destination) < needed {
		return 0, fmt.Errorf("alsa pcm: byte destination holds %d bytes, need %d", len(destination), needed)
	}
	for index := 0; index < count; index++ {
		raw := readPhysical(source[index*sourceLayout.bytes:], sourceLayout)
		sample := decodeInteger(raw, sourceLayout.bits, sourceLayout.signed)
		writePhysical(destination[index*destinationLayout.bytes:], encodeInteger(sample, destinationLayout.bits, destinationLayout.signed), destinationLayout)
	}
	return count, nil
}

type sampleLayout struct {
	bytes  int
	bits   uint
	signed bool
	big    bool
}

func integerLayout(format Format) (sampleLayout, error) {
	switch format {
	case S8:
		return sampleLayout{bytes: 1, bits: 8, signed: true}, nil
	case U8:
		return sampleLayout{bytes: 1, bits: 8}, nil
	case S16LE, S16BE, U16LE, U16BE:
		return sampleLayout{bytes: 2, bits: 16, signed: format == S16LE || format == S16BE, big: format == S16BE || format == U16BE}, nil
	case S24_3LE, S24_3BE, U24_3LE, U24_3BE:
		return sampleLayout{bytes: 3, bits: 24, signed: format == S24_3LE || format == S24_3BE, big: format == S24_3BE || format == U24_3BE}, nil
	case S24LE, S24BE, U24LE, U24BE:
		return sampleLayout{bytes: 4, bits: 24, signed: format == S24LE || format == S24BE, big: format == S24BE || format == U24BE}, nil
	case S32LE, S32BE, U32LE, U32BE:
		return sampleLayout{bytes: 4, bits: 32, signed: format == S32LE || format == S32BE, big: format == S32BE || format == U32BE}, nil
	default:
		return sampleLayout{}, fmt.Errorf("alsa pcm: unsupported integer format %s", format)
	}
}

func readPhysical(source []byte, layout sampleLayout) uint32 {
	var raw uint32
	switch layout.bytes {
	case 1:
		raw = uint32(source[0])
	case 2:
		if layout.big {
			raw = uint32(binary.BigEndian.Uint16(source))
		} else {
			raw = uint32(binary.LittleEndian.Uint16(source))
		}
	case 3:
		if layout.big {
			raw = uint32(source[0])<<16 | uint32(source[1])<<8 | uint32(source[2])
		} else {
			raw = uint32(source[0]) | uint32(source[1])<<8 | uint32(source[2])<<16
		}
	case 4:
		if layout.big {
			raw = binary.BigEndian.Uint32(source)
		} else {
			raw = binary.LittleEndian.Uint32(source)
		}
	}
	if layout.bits < 32 {
		raw &= uint32(1<<layout.bits) - 1
	}
	return raw
}

func writePhysical(destination []byte, raw uint32, layout sampleLayout) {
	switch layout.bytes {
	case 1:
		destination[0] = byte(raw)
	case 2:
		if layout.big {
			binary.BigEndian.PutUint16(destination, uint16(raw))
		} else {
			binary.LittleEndian.PutUint16(destination, uint16(raw))
		}
	case 3:
		if layout.big {
			destination[0], destination[1], destination[2] = byte(raw>>16), byte(raw>>8), byte(raw)
		} else {
			destination[0], destination[1], destination[2] = byte(raw), byte(raw>>8), byte(raw>>16)
		}
	case 4:
		if layout.big {
			binary.BigEndian.PutUint32(destination, raw)
		} else {
			binary.LittleEndian.PutUint32(destination, raw)
		}
	}
}

func decodeInteger(raw uint32, bits uint, signed bool) int32 {
	if signed {
		shift := 32 - bits
		return int32(raw << shift)
	}
	centered := raw - uint32(1<<(bits-1))
	return int32(centered << (32 - bits))
}

func encodeInteger(sample int32, bits uint, signed bool) uint32 {
	shift := 32 - bits
	quantized := uint32(sample >> shift)
	if !signed {
		quantized += uint32(1 << (bits - 1))
	}
	if bits < 32 {
		quantized &= uint32(1<<bits) - 1
	}
	return quantized
}
