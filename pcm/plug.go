package pcm

import (
	"fmt"

	alsa "github.com/sparques/go-alsa"
)

// ConversionParams describes one side of a software PCM conversion boundary.
type ConversionParams struct {
	Format   Format
	Channels int
	Rate     int
}

// PlugConfig keeps application-visible and hardware-visible parameters
// separate. Matrix, when non-nil, is row-major in the actual stream direction:
// application-to-hardware for playback and hardware-to-application for capture.
type PlugConfig struct {
	Application ConversionParams
	Hardware    ConversionParams
	Matrix      []int32
	Taps        int
	Phases      int
}

// PlugConverter is a prepared software conversion stage plus the two parameter
// domains it connects. It does not open or negotiate the hardware endpoint.
type PlugConverter struct {
	*StreamConverter
	application ConversionParams
	hardware    ConversionParams
	stream      alsa.Stream
}

// NewPlugConverter constructs the conversion stage for playback or capture.
// Zero Taps/Phases select 32/1024. A nil Matrix selects DefaultRouteMatrix.
func NewPlugConverter(config PlugConfig, stream alsa.Stream) (*PlugConverter, error) {
	if stream != alsa.Playback && stream != alsa.Capture {
		return nil, fmt.Errorf("alsa pcm: invalid plug stream direction")
	}
	if err := validateConversionParams("application", config.Application); err != nil {
		return nil, err
	}
	if err := validateConversionParams("hardware", config.Hardware); err != nil {
		return nil, err
	}
	source, destination := config.Application, config.Hardware
	if stream == alsa.Capture {
		source, destination = config.Hardware, config.Application
	}
	matrix := config.Matrix
	if matrix == nil {
		matrix = DefaultRouteMatrix(source.Channels, destination.Channels)
	}
	taps, phases := config.Taps, config.Phases
	if taps == 0 {
		taps = 32
	}
	if phases == 0 {
		phases = 1024
	}
	sinc, err := NewSincConverter(source.Rate, destination.Rate, taps, phases)
	if err != nil {
		return nil, err
	}
	converter, err := NewStreamConverter(source.Format, source.Channels, destination.Format, destination.Channels, matrix, sinc)
	if err != nil {
		return nil, err
	}
	return &PlugConverter{StreamConverter: converter, application: config.Application, hardware: config.Hardware, stream: stream}, nil
}

func validateConversionParams(label string, parameters ConversionParams) error {
	if _, err := integerLayout(parameters.Format); err != nil {
		return fmt.Errorf("alsa pcm: %s parameters: %w", label, err)
	}
	if parameters.Channels <= 0 || parameters.Channels > maximumRouteChannels || parameters.Rate <= 0 {
		return fmt.Errorf("alsa pcm: invalid %s conversion parameters", label)
	}
	return nil
}

// ApplicationParams and HardwareParams report the independently negotiated
// sides without changing them for capture direction.
func (c *PlugConverter) ApplicationParams() ConversionParams { return c.application }
func (c *PlugConverter) HardwareParams() ConversionParams    { return c.hardware }
func (c *PlugConverter) Stream() alsa.Stream                 { return c.stream }

// DefaultRouteMatrix returns setup-time channel policy suitable for ordinary
// plug conversion: identity for matching channels, mono duplication when
// expanding mono, equal-weight averaging when reducing to mono, and identity
// for shared channel indices with silence in additional destination channels.
func DefaultRouteMatrix(sourceChannels, destinationChannels int) []int32 {
	if sourceChannels <= 0 || sourceChannels > maximumRouteChannels || destinationChannels <= 0 || destinationChannels > maximumRouteChannels {
		return nil
	}
	matrix := make([]int32, sourceChannels*destinationChannels)
	switch {
	case sourceChannels == 1:
		for destination := 0; destination < destinationChannels; destination++ {
			matrix[destination*sourceChannels] = RouteUnity
		}
	case destinationChannels == 1:
		gain := int32(int64(RouteUnity) / int64(sourceChannels))
		for source := 0; source < sourceChannels; source++ {
			matrix[source] = gain
		}
	default:
		for channel := 0; channel < min(sourceChannels, destinationChannels); channel++ {
			matrix[channel*sourceChannels+channel] = RouteUnity
		}
	}
	return matrix
}
