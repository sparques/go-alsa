package pcm

import "fmt"

// SincStream preserves a SincConverter's phase and FIR history across input
// blocks. It is not safe for concurrent use. Process and Flush allocate no
// memory after construction.
type SincStream struct {
	converter   *SincConverter
	channels    int
	history     []int32
	historyFrom int64
	historySize int
	totalInput  int64
	nextCenter  int64
	remainder   int64
	outputs     int64
	flushed     bool
}

// NewSincStream constructs streaming state for an immutable converter.
func NewSincStream(converter *SincConverter, channels int) (*SincStream, error) {
	if converter == nil || converter.sourceRate <= 0 || converter.destinationRate <= 0 {
		return nil, fmt.Errorf("alsa pcm: streaming resampler requires an initialized converter")
	}
	if channels <= 0 || channels > maximumRouteChannels {
		return nil, fmt.Errorf("alsa pcm: streaming resampler channels must be between 1 and %d", maximumRouteChannels)
	}
	return &SincStream{converter: converter, channels: channels, history: make([]int32, converter.taps*channels)}, nil
}

// LatencySourceFrames returns the symmetric FIR lookahead retained before an
// output can be emitted. LatencyDestinationFrames expresses the same duration
// in destination frames, rounded upward.
func (s *SincStream) LatencySourceFrames() int { return s.converter.taps / 2 }

func (s *SincStream) LatencyDestinationFrames() int {
	latency := int64(s.LatencySourceFrames()) * int64(s.converter.destinationRate)
	return int((latency + int64(s.converter.sourceRate) - 1) / int64(s.converter.sourceRate))
}

// PendingFrames returns how many frames Process would emit after accepting the
// given number of new source frames, without changing stream state.
func (s *SincStream) PendingFrames(sourceFrames int) (int, error) {
	if sourceFrames < 0 || s.flushed {
		return 0, fmt.Errorf("alsa pcm: invalid streaming resampler input state")
	}
	if int64(sourceFrames) > int64(maximumInt)-s.totalInput {
		return 0, fmt.Errorf("alsa pcm: streaming resampler frame count overflows int")
	}
	last := s.totalInput + int64(sourceFrames) - 1
	center, remainder := s.nextCenter, s.remainder
	count := 0
	for center+int64(s.converter.taps/2) <= last {
		count++
		center, remainder = s.advance(center, remainder)
	}
	return count, nil
}

// Process consumes all complete interleaved source frames and emits every
// output whose full FIR window is available. Destination must hold all frames
// reported by PendingFrames or no state is changed.
func (s *SincStream) Process(destination, source []int32) (int, error) {
	if s == nil || s.converter == nil {
		return 0, fmt.Errorf("alsa pcm: uninitialized streaming resampler")
	}
	if s.flushed {
		return 0, fmt.Errorf("alsa pcm: streaming resampler was flushed; call Reset")
	}
	if len(source)%s.channels != 0 {
		return 0, fmt.Errorf("alsa pcm: streaming resampler source contains a partial frame")
	}
	sourceFrames := len(source) / s.channels
	pending, err := s.PendingFrames(sourceFrames)
	if err != nil {
		return 0, err
	}
	if pending > maximumInt/s.channels || len(destination) < pending*s.channels {
		return 0, fmt.Errorf("alsa pcm: streaming resampler destination is too short")
	}
	blockStart := s.totalInput
	blockEnd := blockStart + int64(sourceFrames)
	for output := 0; output < pending; output++ {
		s.render(destination[output*s.channels:], source, blockStart, blockEnd)
		s.nextCenter, s.remainder = s.advance(s.nextCenter, s.remainder)
		s.outputs++
	}
	s.updateHistory(source, blockStart, blockEnd)
	s.totalInput = blockEnd
	return pending, nil
}

// Flush emits the zero-padded FIR tail required to produce the same total
// frame count as block conversion, then marks the stream flushed.
func (s *SincStream) Flush(destination []int32) (int, error) {
	if s == nil || s.converter == nil || s.flushed {
		return 0, fmt.Errorf("alsa pcm: streaming resampler cannot flush")
	}
	target, err := s.converter.OutputFrames(int(s.totalInput))
	if err != nil {
		return 0, err
	}
	pending := target - int(s.outputs)
	if pending < 0 || pending > maximumInt/s.channels || len(destination) < pending*s.channels {
		return 0, fmt.Errorf("alsa pcm: streaming resampler flush destination is too short")
	}
	for output := 0; output < pending; output++ {
		s.render(destination[output*s.channels:], nil, s.totalInput, s.totalInput)
		s.nextCenter, s.remainder = s.advance(s.nextCenter, s.remainder)
		s.outputs++
	}
	s.flushed = true
	return pending, nil
}

// Reset clears phase, history, and flush state without allocating.
func (s *SincStream) Reset() {
	clear(s.history)
	s.historyFrom, s.historySize, s.totalInput = 0, 0, 0
	s.nextCenter, s.remainder, s.outputs = 0, 0, 0
	s.flushed = false
}

func (s *SincStream) advance(center, remainder int64) (int64, int64) {
	remainder += int64(s.converter.sourceRate)
	center += remainder / int64(s.converter.destinationRate)
	return center, remainder % int64(s.converter.destinationRate)
}

func (s *SincStream) render(destination, source []int32, blockStart, blockEnd int64) {
	phase := int((s.remainder*int64(s.converter.phases-1) + int64(s.converter.destinationRate)/2) / int64(s.converter.destinationRate))
	coefficients := s.converter.coefficients[phase*s.converter.taps:]
	first := s.nextCenter - int64(s.converter.taps/2) + 1
	for channel := 0; channel < s.channels; channel++ {
		var mixed float64
		for tap := 0; tap < s.converter.taps; tap++ {
			frame := first + int64(tap)
			var sample int32
			switch {
			case frame >= blockStart && frame < blockEnd:
				sample = source[(frame-blockStart)*int64(s.channels)+int64(channel)]
			case frame >= s.historyFrom && frame < s.historyFrom+int64(s.historySize):
				sample = s.history[(frame-s.historyFrom)*int64(s.channels)+int64(channel)]
			}
			mixed += float64(sample) * coefficients[tap]
		}
		destination[channel] = saturateFloatQ31(mixed)
	}
}

func (s *SincStream) updateHistory(source []int32, blockStart, blockEnd int64) {
	keep := min(int64(s.converter.taps), blockEnd)
	from := blockEnd - keep
	for frame := from; frame < blockEnd; frame++ {
		for channel := 0; channel < s.channels; channel++ {
			var sample int32
			if frame >= blockStart {
				sample = source[(frame-blockStart)*int64(s.channels)+int64(channel)]
			} else if frame >= s.historyFrom && frame < s.historyFrom+int64(s.historySize) {
				sample = s.history[(frame-s.historyFrom)*int64(s.channels)+int64(channel)]
			}
			s.history[(frame-from)*int64(s.channels)+int64(channel)] = sample
		}
	}
	s.historyFrom, s.historySize = from, int(keep)
}
