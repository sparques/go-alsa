package pcm

import (
	"fmt"
	"math"
)

// SincConverter is an immutable, precomputed windowed-sinc sample-rate
// converter. Convert is safe for concurrent use with distinct buffers and
// performs no allocation. It is block-based; streaming boundary history is a
// separate graph-stage concern.
type SincConverter struct {
	sourceRate      int
	destinationRate int
	taps            int
	phases          int
	coefficients    []float64
}

// NewSincConverter constructs a band-limited converter. Taps must be even and
// between 8 and 256; phases must be between 16 and 4096. A useful general
// purpose setting is 32 taps and 1024 phases.
func NewSincConverter(sourceRate, destinationRate, taps, phases int) (*SincConverter, error) {
	if sourceRate <= 0 || destinationRate <= 0 {
		return nil, fmt.Errorf("alsa pcm: resampler rates must be positive")
	}
	if taps < 8 || taps > 256 || taps%2 != 0 {
		return nil, fmt.Errorf("alsa pcm: resampler taps must be even and between 8 and 256")
	}
	if phases < 16 || phases > 4096 {
		return nil, fmt.Errorf("alsa pcm: resampler phases must be between 16 and 4096")
	}
	converter := &SincConverter{
		sourceRate: sourceRate, destinationRate: destinationRate,
		taps: taps, phases: phases, coefficients: make([]float64, taps*phases),
	}
	cutoff := math.Min(1, float64(destinationRate)/float64(sourceRate))
	for phase := 0; phase < phases; phase++ {
		fraction := float64(phase) / float64(phases-1)
		row := converter.coefficients[phase*taps:]
		var sum float64
		for tap := 0; tap < taps; tap++ {
			offset := float64(tap-taps/2+1) - fraction
			window := 0.42 - 0.5*math.Cos(2*math.Pi*float64(tap)/float64(taps-1)) + 0.08*math.Cos(4*math.Pi*float64(tap)/float64(taps-1))
			coefficient := cutoff * sinc(cutoff*offset) * window
			row[tap] = coefficient
			sum += coefficient
		}
		if sum == 0 {
			return nil, fmt.Errorf("alsa pcm: resampler produced an empty phase")
		}
		for tap := 0; tap < taps; tap++ {
			row[tap] /= sum
		}
	}
	return converter, nil
}

func sinc(value float64) float64 {
	if value == 0 {
		return 1
	}
	angle := math.Pi * value
	return math.Sin(angle) / angle
}

// OutputFrames returns the number of complete destination frames generated
// from a stateless input block.
func (c *SincConverter) OutputFrames(inputFrames int) (int, error) {
	if c == nil || c.sourceRate <= 0 || c.destinationRate <= 0 {
		return 0, fmt.Errorf("alsa pcm: uninitialized resampler")
	}
	if inputFrames < 0 {
		return 0, fmt.Errorf("alsa pcm: negative resampler frame count")
	}
	frames := uint64(inputFrames) * uint64(c.destinationRate) / uint64(c.sourceRate)
	if frames > uint64(maximumInt) {
		return 0, fmt.Errorf("alsa pcm: resampler output size overflows int")
	}
	return int(frames), nil
}

// Convert resamples interleaved signed Q1.31 frames. Samples outside the input
// block are treated as silence. Source and destination must not overlap.
func (c *SincConverter) Convert(destination, source []int32, channels int) (int, error) {
	if channels <= 0 || channels > maximumRouteChannels {
		return 0, fmt.Errorf("alsa pcm: resampler channels must be between 1 and %d", maximumRouteChannels)
	}
	if len(source)%channels != 0 {
		return 0, fmt.Errorf("alsa pcm: resampler source contains a partial frame")
	}
	inputFrames := len(source) / channels
	outputFrames, err := c.OutputFrames(inputFrames)
	if err != nil {
		return 0, err
	}
	if outputFrames > maximumInt/channels || len(destination) < outputFrames*channels {
		return 0, fmt.Errorf("alsa pcm: resampler destination is too short")
	}
	if c.sourceRate == c.destinationRate {
		copy(destination[:len(source)], source)
		return inputFrames, nil
	}
	for outputFrame := 0; outputFrame < outputFrames; outputFrame++ {
		positionNumerator := uint64(outputFrame) * uint64(c.sourceRate)
		center := int(positionNumerator / uint64(c.destinationRate))
		remainder := positionNumerator % uint64(c.destinationRate)
		phase := int((remainder*uint64(c.phases-1) + uint64(c.destinationRate)/2) / uint64(c.destinationRate))
		coefficients := c.coefficients[phase*c.taps:]
		first := center - c.taps/2 + 1
		for channel := 0; channel < channels; channel++ {
			var mixed float64
			for tap := 0; tap < c.taps; tap++ {
				inputFrame := first + tap
				if inputFrame >= 0 && inputFrame < inputFrames {
					mixed += float64(source[inputFrame*channels+channel]) * coefficients[tap]
				}
			}
			destination[outputFrame*channels+channel] = saturateFloatQ31(mixed)
		}
	}
	return outputFrames, nil
}

func saturateFloatQ31(value float64) int32 {
	if value >= float64(maxInt32) {
		return maxInt32
	}
	if value <= float64(minInt32) {
		return minInt32
	}
	return int32(math.Round(value))
}
