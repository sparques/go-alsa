package pcm

import "fmt"

const maximumRouteChannels = 4096

// RouteUnity is unity gain in the signed Q2.30 coefficients accepted by
// RouteInterleaved. Q2.30 permits gains approaching 2 while retaining enough
// precision for common averaging and channel-routing matrices.
const RouteUnity int32 = 1 << 30

// RouteInterleaved applies a row-major output-by-input Q2.30 gain matrix to
// interleaved signed Q1.31 samples. It performs no allocation, saturates mixed
// outputs to int32, and returns the number of complete frames routed.
// Source and destination must not overlap.
func RouteInterleaved(destination []int32, destinationChannels int, source []int32, sourceChannels int, matrix []int32) (int, error) {
	if sourceChannels <= 0 || sourceChannels > maximumRouteChannels || destinationChannels <= 0 || destinationChannels > maximumRouteChannels {
		return 0, fmt.Errorf("alsa pcm: route channels must be between 1 and %d", maximumRouteChannels)
	}
	if len(source)%sourceChannels != 0 {
		return 0, fmt.Errorf("alsa pcm: route source contains a partial frame")
	}
	if len(matrix) != sourceChannels*destinationChannels {
		return 0, fmt.Errorf("alsa pcm: route matrix has %d coefficients, need %d", len(matrix), sourceChannels*destinationChannels)
	}
	frames := len(source) / sourceChannels
	needed := frames * destinationChannels
	if len(destination) < needed {
		return 0, fmt.Errorf("alsa pcm: route destination holds %d samples, need %d", len(destination), needed)
	}
	for frame := 0; frame < frames; frame++ {
		input := source[frame*sourceChannels:]
		output := destination[frame*destinationChannels:]
		for destinationChannel := 0; destinationChannel < destinationChannels; destinationChannel++ {
			row := matrix[destinationChannel*sourceChannels:]
			var mixed int64
			for sourceChannel := 0; sourceChannel < sourceChannels; sourceChannel++ {
				mixed += int64(input[sourceChannel]) * int64(row[sourceChannel]) >> 30
			}
			output[destinationChannel] = saturateQ31(mixed)
		}
	}
	return frames, nil
}

func saturateQ31(value int64) int32 {
	if value > int64(maxInt32) {
		return maxInt32
	}
	if value < int64(minInt32) {
		return minInt32
	}
	return int32(value)
}

const (
	maxInt32 = int32(1<<31 - 1)
	minInt32 = int32(-1 << 31)
)
