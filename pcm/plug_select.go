package pcm

import (
	"errors"
	"fmt"
	"sort"
)

// ErrNoPlugTarget reports that no deterministic integer format/channel tuple
// survived the kernel's joint refinement.
var ErrNoPlugTarget = errors.New("alsa pcm: no jointly supported plug target")

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

// PlugHardwareRequest combines application parameters with hardware timing
// goals. Format, channels, and rate are selected and jointly refined.
type PlugHardwareRequest struct {
	Application  ConversionParams
	PeriodFrames int
	PeriodCount  int
	BufferFrames int
}

// RefinePlugHardware queries current capabilities and non-mutatingly tries
// deterministic format/channel candidates through the kernel's joint near
// refinement. The returned setup has not been committed.
func (h *Handle) RefinePlugHardware(request PlugHardwareRequest) (PlugTarget, HardwareSetup, error) {
	if err := validatePlugHardwareRequest(request); err != nil {
		return PlugTarget{}, HardwareSetup{}, err
	}
	capabilities, err := h.Capabilities()
	if err != nil {
		return PlugTarget{}, HardwareSetup{}, err
	}
	if !containsAccess(capabilities.Accesses, RWInterleaved) {
		return PlugTarget{}, HardwareSetup{}, fmt.Errorf("%w: RW_INTERLEAVED unavailable", ErrNoPlugTarget)
	}
	formats := orderedIntegerFormats(request.Application.Format, capabilities.Formats)
	channels, err := orderedChannelCandidates(request.Application.Channels, capabilities.Channels)
	if err != nil {
		return PlugTarget{}, HardwareSetup{}, fmt.Errorf("%w: %v", ErrNoPlugTarget, err)
	}
	if len(formats) == 0 {
		return PlugTarget{}, HardwareSetup{}, fmt.Errorf("%w: no supported integer format", ErrNoPlugTarget)
	}
	var last error
	for _, format := range formats {
		for _, channelCount := range channels {
			candidate := HardwareConfig{
				Access: RWInterleaved, Format: format, Channels: channelCount, Rate: request.Application.Rate,
				PeriodFrames: request.PeriodFrames, PeriodCount: request.PeriodCount, BufferFrames: request.BufferFrames,
			}
			setup, refineErr := h.RefineHardwareNear(candidate)
			if refineErr == nil {
				target := PlugTarget{Access: setup.Access, Params: ConversionParams{Format: setup.Format, Channels: setup.Channels, Rate: setup.Rate}}
				return target, setup, nil
			}
			if errors.Is(refineErr, ErrDisconnected) {
				return PlugTarget{}, HardwareSetup{}, refineErr
			}
			last = refineErr
		}
	}
	return PlugTarget{}, HardwareSetup{}, fmt.Errorf("%w: %v", ErrNoPlugTarget, last)
}

func validatePlugHardwareRequest(request PlugHardwareRequest) error {
	if err := validateConversionParams("application", request.Application); err != nil {
		return err
	}
	if request.PeriodFrames <= 0 || request.BufferFrames < request.PeriodFrames || request.PeriodCount < 0 {
		return fmt.Errorf("alsa pcm: invalid plug hardware timing request")
	}
	return nil
}

// ConfigurePlugHardware selects and refines a conversion-friendly hardware
// tuple, then commits that exact tuple. It moves the PCM from OPEN to SETUP on
// success; a failed search does not commit hardware parameters.
func (h *Handle) ConfigurePlugHardware(request PlugHardwareRequest) (PlugTarget, HardwareSetup, error) {
	target, refined, err := h.RefinePlugHardware(request)
	if err != nil {
		return PlugTarget{}, HardwareSetup{}, err
	}
	committed, err := h.ConfigureHardware(refined.HardwareConfig)
	if err != nil {
		return PlugTarget{}, HardwareSetup{}, fmt.Errorf("alsa pcm: commit refined plug target: %w", err)
	}
	target = PlugTarget{
		Access: committed.Access,
		Params: ConversionParams{Format: committed.Format, Channels: committed.Channels, Rate: committed.Rate},
	}
	return target, committed, nil
}

func orderedIntegerFormats(target Format, values []Format) []Format {
	result := make([]Format, 0, len(values))
	seen := make(map[Format]bool, len(values))
	for _, value := range values {
		if _, err := integerLayout(value); err == nil && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := formatDistance(target, result[i]), formatDistance(target, result[j])
		return left < right || left == right && result[i] < result[j]
	})
	return result
}

func orderedChannelCandidates(target int, values Range) ([]int, error) {
	nearest, err := nearestRangeValue(target, values, "channels")
	if err != nil {
		return nil, err
	}
	minimum, _ := nearestRangeValue(1, values, "channels")
	maximum, _ := nearestRangeValue(maximumInt, values, "channels")
	seen := map[int]bool{}
	var result []int
	for _, value := range []int{nearest, 1, 2, minimum, maximum} {
		if value < minimum || value > maximum || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := absInt(result[i]-target), absInt(result[j]-target)
		return left < right || left == right && result[i] < result[j]
	})
	return result, nil
}
