package pcm

import "fmt"

// PlugTarget is a deterministic conversion-friendly candidate selected from
// unconstrained hardware capabilities. The kernel must still jointly refine
// and accept the complete HardwareConfig before this target is authoritative.
type PlugTarget struct {
	Params ConversionParams
	Access Access
}

// SelectPlugTarget selects an interleaved integer target, preferring exact
// application parameters, then the closest sample representation and nearest
// channel/rate values. Capabilities are an unconstrained prefilter; this does
// not replace the kernel's joint HW_REFINE/HW_PARAMS validation.
func SelectPlugTarget(application ConversionParams, capabilities Capabilities) (PlugTarget, error) {
	if err := validateConversionParams("application", application); err != nil {
		return PlugTarget{}, err
	}
	if !containsAccess(capabilities.Accesses, RWInterleaved) {
		return PlugTarget{}, fmt.Errorf("alsa pcm: plug target requires RW_INTERLEAVED hardware access")
	}
	format, err := nearestIntegerFormat(application.Format, capabilities.Formats)
	if err != nil {
		return PlugTarget{}, err
	}
	channels, err := nearestRangeValue(application.Channels, capabilities.Channels, "channels")
	if err != nil {
		return PlugTarget{}, err
	}
	rate, err := nearestRangeValue(application.Rate, capabilities.Rate, "rate")
	if err != nil {
		return PlugTarget{}, err
	}
	return PlugTarget{Params: ConversionParams{Format: format, Channels: channels, Rate: rate}, Access: RWInterleaved}, nil
}

func containsAccess(values []Access, target Access) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func nearestRangeValue(target int, values Range, label string) (int, error) {
	if values.Empty || uint64(values.Minimum) > uint64(maximumInt) || uint64(values.Maximum) > uint64(maximumInt) {
		return 0, fmt.Errorf("alsa pcm: plug hardware has no usable %s range", label)
	}
	minimum, maximum := int(values.Minimum), int(values.Maximum)
	if values.OpenMinimum {
		minimum++
	}
	if values.OpenMaximum {
		maximum--
	}
	if minimum > maximum || maximum <= 0 {
		return 0, fmt.Errorf("alsa pcm: plug hardware has no usable %s", label)
	}
	minimum = max(minimum, 1)
	return min(max(target, minimum), maximum), nil
}

func nearestIntegerFormat(target Format, formats []Format) (Format, error) {
	best, bestScore, found := Format(0), maximumInt, false
	for _, candidate := range formats {
		if _, err := integerLayout(candidate); err != nil {
			continue
		}
		score := formatDistance(target, candidate)
		if !found || score < bestScore || score == bestScore && candidate < best {
			best, bestScore, found = candidate, score, true
		}
	}
	if !found {
		return 0, fmt.Errorf("alsa pcm: plug hardware exposes no supported integer format")
	}
	return best, nil
}

func formatDistance(target, candidate Format) int {
	if target == candidate {
		return 0
	}
	targetLayout, targetErr := integerLayout(target)
	candidateLayout, _ := integerLayout(candidate)
	if targetErr != nil {
		return maximumInt / 2
	}
	score := absInt(int(targetLayout.bits)-int(candidateLayout.bits)) * 16
	if candidateLayout.bits < targetLayout.bits {
		score += 32
	}
	if targetLayout.signed != candidateLayout.signed {
		score += 256
	}
	if targetLayout.big != candidateLayout.big && targetLayout.bytes > 1 && candidateLayout.bytes > 1 {
		score += 4
	}
	if targetLayout.bytes != candidateLayout.bytes {
		score += 2
	}
	return score
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// ConverterForCapabilities selects a provisional hardware target and prepares
// this named plan's converter. Direct graphs accept only an exact target.
func (p NamedPlan) ConverterForCapabilities(application ConversionParams, capabilities Capabilities, matrix []int32) (PlugTarget, *PlugConverter, error) {
	target, err := SelectPlugTarget(application, capabilities)
	if err != nil {
		return PlugTarget{}, nil, err
	}
	converter, err := p.Converter(application, target.Params, matrix)
	if err != nil {
		return PlugTarget{}, nil, err
	}
	return target, converter, nil
}
