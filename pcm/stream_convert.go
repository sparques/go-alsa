package pcm

import "fmt"

// StreamConverter composes integer format conversion, channel routing, and
// continuous band-limited rate conversion. Construction allocates immutable
// matrix and FIR state; Process, Flush, and Reset allocate no memory.
type StreamConverter struct {
	sourceFormat        Format
	destinationFormat   Format
	sourceChannels      int
	destinationChannels int
	matrix              []int32
	resampler           *SincStream
}

// NewStreamConverter constructs a streaming conversion stage. Matrix is a
// row-major destination-by-source Q2.30 channel matrix and is defensively
// copied. Converter rates define the source and destination rates.
func NewStreamConverter(sourceFormat Format, sourceChannels int, destinationFormat Format, destinationChannels int, matrix []int32, converter *SincConverter) (*StreamConverter, error) {
	if _, err := integerLayout(sourceFormat); err != nil {
		return nil, err
	}
	if _, err := integerLayout(destinationFormat); err != nil {
		return nil, err
	}
	if sourceChannels <= 0 || sourceChannels > maximumRouteChannels || destinationChannels <= 0 || destinationChannels > maximumRouteChannels {
		return nil, fmt.Errorf("alsa pcm: stream converter channels must be between 1 and %d", maximumRouteChannels)
	}
	if len(matrix) != sourceChannels*destinationChannels {
		return nil, fmt.Errorf("alsa pcm: stream converter matrix has %d coefficients, need %d", len(matrix), sourceChannels*destinationChannels)
	}
	resampler, err := NewSincStream(converter, destinationChannels)
	if err != nil {
		return nil, err
	}
	return &StreamConverter{
		sourceFormat: sourceFormat, destinationFormat: destinationFormat,
		sourceChannels: sourceChannels, destinationChannels: destinationChannels,
		matrix: append([]int32(nil), matrix...), resampler: resampler,
	}, nil
}

// PendingFrames returns the output frames Process would produce for complete
// sourceFrames. ScratchSamples returns the exact int32 scratch requirement.
func (c *StreamConverter) PendingFrames(sourceFrames int) (int, error) {
	if c == nil || c.resampler == nil {
		return 0, fmt.Errorf("alsa pcm: uninitialized stream converter")
	}
	return c.resampler.PendingFrames(sourceFrames)
}

func (c *StreamConverter) ScratchSamples(sourceFrames int) (int, error) {
	outputFrames, err := c.PendingFrames(sourceFrames)
	if err != nil {
		return 0, err
	}
	if sourceFrames < 0 || sourceFrames > maximumInt/(c.sourceChannels+c.destinationChannels) {
		return 0, fmt.Errorf("alsa pcm: stream converter scratch size overflows int")
	}
	input := sourceFrames * (c.sourceChannels + c.destinationChannels)
	if outputFrames > (maximumInt-input)/c.destinationChannels {
		return 0, fmt.Errorf("alsa pcm: stream converter scratch size overflows int")
	}
	return input + outputFrames*c.destinationChannels, nil
}

// Process consumes all complete interleaved byte frames and emits every output
// currently available. Destination and scratch must meet the exact sizes
// reported by PendingFrames and ScratchSamples; no state changes on failure.
func (c *StreamConverter) Process(destination, source []byte, scratch []int32) (int, error) {
	if c == nil || c.resampler == nil {
		return 0, fmt.Errorf("alsa pcm: uninitialized stream converter")
	}
	sourceWidth, _ := c.sourceFormat.PhysicalBytes()
	frameBytes := sourceWidth * c.sourceChannels
	if len(source)%frameBytes != 0 {
		return 0, fmt.Errorf("alsa pcm: stream converter source contains a partial frame")
	}
	sourceFrames := len(source) / frameBytes
	outputFrames, err := c.PendingFrames(sourceFrames)
	if err != nil {
		return 0, err
	}
	scratchSamples, err := c.ScratchSamples(sourceFrames)
	if err != nil {
		return 0, err
	}
	if len(scratch) < scratchSamples {
		return 0, fmt.Errorf("alsa pcm: stream converter scratch holds %d samples, need %d", len(scratch), scratchSamples)
	}
	destinationWidth, _ := c.destinationFormat.PhysicalBytes()
	if outputFrames > maximumInt/(c.destinationChannels*destinationWidth) {
		return 0, fmt.Errorf("alsa pcm: stream converter destination size overflows int")
	}
	neededBytes := outputFrames * c.destinationChannels * destinationWidth
	if len(destination) < neededBytes {
		return 0, fmt.Errorf("alsa pcm: stream converter destination holds %d bytes, need %d", len(destination), neededBytes)
	}
	decodedCount := sourceFrames * c.sourceChannels
	routedCount := sourceFrames * c.destinationChannels
	decoded := scratch[:decodedCount]
	routed := scratch[decodedCount : decodedCount+routedCount]
	output := scratch[decodedCount+routedCount : scratchSamples]
	if _, err := DecodeSamples(decoded, source, c.sourceFormat); err != nil {
		return 0, err
	}
	if _, err := RouteInterleaved(routed, c.destinationChannels, decoded, c.sourceChannels, c.matrix); err != nil {
		return 0, err
	}
	written, err := c.resampler.Process(output, routed)
	if err != nil {
		return 0, err
	}
	if _, err := EncodeSamples(destination[:neededBytes], output, c.destinationFormat); err != nil {
		return 0, err
	}
	return written, nil
}

// PendingFlushFrames and Flush expose the resampler tail using caller-owned
// buffers. Flush scratch requires frames*destinationChannels int32 samples.
func (c *StreamConverter) PendingFlushFrames() (int, error) {
	if c == nil || c.resampler == nil {
		return 0, fmt.Errorf("alsa pcm: uninitialized stream converter")
	}
	return c.resampler.PendingFlushFrames()
}

func (c *StreamConverter) Flush(destination []byte, scratch []int32) (int, error) {
	frames, err := c.PendingFlushFrames()
	if err != nil {
		return 0, err
	}
	neededSamples := frames * c.destinationChannels
	if len(scratch) < neededSamples {
		return 0, fmt.Errorf("alsa pcm: stream converter flush scratch is too short")
	}
	destinationWidth, _ := c.destinationFormat.PhysicalBytes()
	if frames > maximumInt/(c.destinationChannels*destinationWidth) {
		return 0, fmt.Errorf("alsa pcm: stream converter flush size overflows int")
	}
	neededBytes := neededSamples * destinationWidth
	if len(destination) < neededBytes {
		return 0, fmt.Errorf("alsa pcm: stream converter flush destination is too short")
	}
	written, err := c.resampler.Flush(scratch[:neededSamples])
	if err != nil {
		return 0, err
	}
	if _, err := EncodeSamples(destination[:neededBytes], scratch[:neededSamples], c.destinationFormat); err != nil {
		return 0, err
	}
	return written, nil
}

func (c *StreamConverter) Reset() { c.resampler.Reset() }

func (c *StreamConverter) LatencySourceFrames() int { return c.resampler.LatencySourceFrames() }

func (c *StreamConverter) LatencyDestinationFrames() int {
	return c.resampler.LatencyDestinationFrames()
}
