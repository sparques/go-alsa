package control

import (
	"encoding/binary"
	"testing"
)

func FuzzParseTLV(f *testing.F) {
	seeds := [][]uint32{
		{uint32(TLVDBScale), 8, signedWord(-3000), 50},
		{uint32(TLVContainer), 16, uint32(TLVChannelMapFixed), 8, 3, 4},
		{uint32(TLVDBRange), 24, 0, 10, uint32(TLVDBMinMax), 8, 0, 100},
		{}, {0}, {0, ^uint32(0)},
	}
	for _, words := range seeds {
		buffer := make([]byte, len(words)*4)
		for index, word := range words {
			binary.LittleEndian.PutUint32(buffer[index*4:], word)
		}
		f.Add(buffer)
	}
	f.Fuzz(func(t *testing.T, buffer []byte) {
		// Production reads are bounded to 1 MiB. Keep the fuzz corpus compact
		// while exercising every parser length and nesting transition.
		if len(buffer) > 16<<10 {
			buffer = buffer[:16<<10]
		}
		words := make([]uint32, len(buffer)/4)
		for index := range words {
			words[index] = binary.LittleEndian.Uint32(buffer[index*4:])
		}
		_, _ = ParseTLV(words)
	})
}
