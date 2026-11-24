package mixer

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sparques/go-alsa/control"
)

type Handle struct{ control *control.Handle }

func Open(selector string) (*Handle, error) {
	handle, err := control.Open(selector)
	if err != nil {
		return nil, err
	}
	return &Handle{control: handle}, nil
}

func (h *Handle) Close() error { return h.control.Close() }

type binding struct {
	id   control.ElementID
	info control.ElementInfo
}

// Element groups conventional simple controls sharing a base name and index.
type Element struct {
	Name  string
	Index uint32
	h     *Handle

	playbackSwitch *binding
	playbackVolume *binding
	captureSwitch  *binding
	captureVolume  *binding
}

func (e *Element) HasPlaybackSwitch() bool { return e.playbackSwitch != nil }
func (e *Element) HasPlaybackVolume() bool { return e.playbackVolume != nil }
func (e *Element) HasCaptureSwitch() bool  { return e.captureSwitch != nil }
func (e *Element) HasCaptureVolume() bool  { return e.captureVolume != nil }

// Elements discovers standard simple-element suffixes on MIXER controls.
func (h *Handle) Elements() ([]*Element, error) {
	ids, err := h.control.Elements()
	if err != nil {
		return nil, err
	}
	type key struct {
		name  string
		index uint32
	}
	byKey := make(map[key]*Element)
	for _, id := range ids {
		if id.Interface != control.InterfaceMixer {
			continue
		}
		base, kind, ok := splitSimpleName(id.Name)
		if !ok {
			continue
		}
		info, err := h.control.Info(id)
		if err != nil {
			return nil, err
		}
		itemKey := key{name: base, index: id.Index}
		element := byKey[itemKey]
		if element == nil {
			element = &Element{Name: base, Index: id.Index, h: h}
			byKey[itemKey] = element
		}
		value := &binding{id: id, info: info}
		switch kind {
		case "playback_switch":
			if info.Type == control.TypeBoolean {
				element.playbackSwitch = value
			}
		case "playback_volume":
			if info.Type == control.TypeInteger || info.Type == control.TypeInteger64 {
				element.playbackVolume = value
			}
		case "capture_switch":
			if info.Type == control.TypeBoolean {
				element.captureSwitch = value
			}
		case "capture_volume":
			if info.Type == control.TypeInteger || info.Type == control.TypeInteger64 {
				element.captureVolume = value
			}
		}
	}
	result := make([]*Element, 0, len(byKey))
	for _, element := range byKey {
		if element.playbackSwitch != nil || element.playbackVolume != nil || element.captureSwitch != nil || element.captureVolume != nil {
			result = append(result, element)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Index < result[j].Index
	})
	return result, nil
}

func splitSimpleName(name string) (string, string, bool) {
	suffixes := []struct{ suffix, kind string }{
		{" Playback Switch", "playback_switch"},
		{" Playback Volume", "playback_volume"},
		{" Capture Switch", "capture_switch"},
		{" Capture Volume", "capture_volume"},
	}
	for _, suffix := range suffixes {
		if base, ok := strings.CutSuffix(name, suffix.suffix); ok && base != "" {
			return base, suffix.kind, true
		}
	}
	return "", "", false
}

type Volume struct {
	Values  []int64
	Minimum int64
	Maximum int64
	Step    int64
	DB      []int32
	// Normalized contains Values mapped linearly from the raw range to 0..1.
	Normalized []float64
}

func (e *Element) PlaybackSwitch() ([]bool, error) { return e.readSwitch(e.playbackSwitch) }
func (e *Element) CaptureSwitch() ([]bool, error)  { return e.readSwitch(e.captureSwitch) }
func (e *Element) SetPlaybackSwitch(values []bool) error {
	return e.writeSwitch(e.playbackSwitch, values)
}
func (e *Element) SetCaptureSwitch(values []bool) error {
	return e.writeSwitch(e.captureSwitch, values)
}
func (e *Element) PlaybackVolume() (Volume, error) { return e.readVolume(e.playbackVolume) }
func (e *Element) CaptureVolume() (Volume, error)  { return e.readVolume(e.captureVolume) }
func (e *Element) SetPlaybackVolume(values []int64) error {
	return e.writeVolume(e.playbackVolume, values)
}
func (e *Element) SetCaptureVolume(values []int64) error {
	return e.writeVolume(e.captureVolume, values)
}

// SetPlaybackVolumeNormalized sets each playback channel on a linear raw-value
// scale from 0 (minimum) through 1 (maximum), rounded to the nearest valid step.
func (e *Element) SetPlaybackVolumeNormalized(values []float64) error {
	return e.writeNormalizedVolume(e.playbackVolume, values)
}

// SetCaptureVolumeNormalized sets each capture channel on a linear raw-value
// scale from 0 (minimum) through 1 (maximum), rounded to the nearest valid step.
func (e *Element) SetCaptureVolumeNormalized(values []float64) error {
	return e.writeNormalizedVolume(e.captureVolume, values)
}

func (e *Element) readSwitch(item *binding) ([]bool, error) {
	if item == nil {
		return nil, fmt.Errorf("alsa mixer: %s has no requested switch", e.Name)
	}
	value, err := e.h.control.Read(item.id)
	if err != nil {
		return nil, err
	}
	return value.Booleans, nil
}

func (e *Element) writeSwitch(item *binding, values []bool) error {
	if item == nil {
		return fmt.Errorf("alsa mixer: %s has no requested switch", e.Name)
	}
	return e.h.control.Write(control.ElementValue{ID: item.id, Type: control.TypeBoolean, Booleans: append([]bool(nil), values...)})
}

func (e *Element) readVolume(item *binding) (Volume, error) {
	if item == nil {
		return Volume{}, fmt.Errorf("alsa mixer: %s has no requested volume", e.Name)
	}
	value, err := e.h.control.Read(item.id)
	if err != nil {
		return Volume{}, err
	}
	result := Volume{Values: value.Integers, Minimum: item.info.Integer.Minimum, Maximum: item.info.Integer.Maximum, Step: item.info.Integer.Step}
	result.Normalized = make([]float64, len(result.Values))
	for index, raw := range result.Values {
		result.Normalized[index] = normalizeRaw(raw, item.info.Integer)
	}
	if item.info.Access.TLVReadable() {
		tlv, err := e.h.control.ReadDecodedTLV(item.id, 4096)
		if err != nil {
			return Volume{}, err
		}
		result.DB = make([]int32, len(result.Values))
		for index, raw := range result.Values {
			db, ok := rawToDB(raw, item.info.Integer, tlv)
			if !ok {
				result.DB = nil
				break
			}
			result.DB[index] = db
		}
	}
	return result, nil
}

func (e *Element) writeVolume(item *binding, values []int64) error {
	if item == nil {
		return fmt.Errorf("alsa mixer: %s has no requested volume", e.Name)
	}
	return e.h.control.Write(control.ElementValue{ID: item.id, Type: item.info.Type, Integers: append([]int64(nil), values...)})
}

func (e *Element) writeNormalizedVolume(item *binding, values []float64) error {
	if item == nil {
		return fmt.Errorf("alsa mixer: %s has no requested volume", e.Name)
	}
	raw := make([]int64, len(values))
	for index, value := range values {
		converted, err := denormalizeRaw(value, item.info.Integer)
		if err != nil {
			return fmt.Errorf("alsa mixer: normalized channel %d: %w", index, err)
		}
		raw[index] = converted
	}
	return e.writeVolume(item, raw)
}

func normalizeRaw(raw int64, integer control.IntegerRange) float64 {
	if integer.Maximum <= integer.Minimum {
		return 0
	}
	value := (float64(raw) - float64(integer.Minimum)) / (float64(integer.Maximum) - float64(integer.Minimum))
	return math.Max(0, math.Min(1, value))
}

func denormalizeRaw(value float64, integer control.IntegerRange) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0, fmt.Errorf("value %v is outside 0..1", value)
	}
	if integer.Maximum < integer.Minimum {
		return 0, fmt.Errorf("invalid integer range %+v", integer)
	}
	if integer.Maximum == integer.Minimum {
		return integer.Minimum, nil
	}
	step := integer.Step
	if step <= 0 {
		step = 1
	}
	span := float64(integer.Maximum) - float64(integer.Minimum)
	steps := math.Round(value * (span / float64(step)))
	raw := integer.Minimum + int64(steps)*step
	if raw > integer.Maximum {
		raw = integer.Maximum
	}
	return raw, nil
}

func rawToDB(raw int64, integer control.IntegerRange, tlv control.TLV) (int32, bool) {
	if tlv.DB != nil {
		switch tlv.Type {
		case control.TLVDBScale:
			return tlv.DB.Minimum + int32((raw-integer.Minimum)*int64(tlv.DB.Step)), true
		case control.TLVDBMinMax, control.TLVDBMinMaxMute:
			span := integer.Maximum - integer.Minimum
			if span <= 0 {
				return tlv.DB.Minimum, true
			}
			return tlv.DB.Minimum + int32((raw-integer.Minimum)*int64(tlv.DB.Maximum-tlv.DB.Minimum)/span), true
		}
	}
	if tlv.Type == control.TLVDBRange {
		for _, item := range tlv.DBRanges {
			if raw >= int64(item.MinimumValue) && raw <= int64(item.MaximumValue) {
				return rawToDB(raw, control.IntegerRange{Minimum: int64(item.MinimumValue), Maximum: int64(item.MaximumValue)}, item.Curve)
			}
		}
	}
	return 0, false
}
