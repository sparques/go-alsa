package mixer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/sparques/go-alsa/control"
)

type Handle struct {
	control *control.Handle
	cacheMu sync.Mutex

	elementsCached     bool
	elementsCache      []*Element
	enumerationsCached bool
	enumerationsCache  []*EnumeratedElement
}

func Open(selector string) (*Handle, error) {
	handle, err := control.Open(selector)
	if err != nil {
		return nil, err
	}
	return &Handle{control: handle}, nil
}

func (h *Handle) Close() error {
	h.Refresh()
	return h.control.Close()
}

// Refresh discards cached mixer topology and capability metadata. Values are
// always read directly from the kernel and are never cached.
func (h *Handle) Refresh() {
	h.cacheMu.Lock()
	h.elementsCached = false
	h.elementsCache = nil
	h.enumerationsCached = false
	h.enumerationsCache = nil
	h.cacheMu.Unlock()
}

// EventKind identifies the conventional role of a changed mixer control.
type EventKind uint8

const (
	EventUnknown EventKind = iota
	EventSwitch
	EventVolume
)

// Direction identifies whether a conventional mixer control is used for
// playback or capture.
type Direction uint8

const (
	DirectionUnknown Direction = iota
	DirectionPlayback
	DirectionCapture
)

// Event reports a raw control event and, when recognized, its simple-element
// identity. Unknown controls retain Raw and have Recognized set to false.
type Event struct {
	Raw        control.Event
	Name       string
	Index      uint32
	Direction  Direction
	Kind       EventKind
	Recognized bool
}

// Subscribe enables or disables kernel mixer-control events.
func (h *Handle) Subscribe(enabled bool) error { return h.control.Subscribe(enabled) }

// NextEvent waits for the next control event and classifies conventional
// playback/capture switch and volume controls.
func (h *Handle) NextEvent(ctx context.Context) (Event, error) {
	raw, err := h.control.NextEvent(ctx)
	if err != nil {
		return Event{}, err
	}
	if raw.Mask == control.EventRemove || raw.Mask&(control.EventInfo|control.EventAdd|control.EventTLV) != 0 {
		h.Refresh()
	}
	return classifyEvent(raw), nil
}

func classifyEvent(raw control.Event) Event {
	event := Event{Raw: raw, Index: raw.ID.Index}
	if raw.ID.Interface != control.InterfaceMixer {
		return event
	}
	base, kind, ok := splitSimpleName(raw.ID.Name)
	if !ok {
		return event
	}
	event.Name = base
	event.Recognized = true
	switch kind {
	case "playback_switch":
		event.Direction, event.Kind = DirectionPlayback, EventSwitch
	case "playback_volume":
		event.Direction, event.Kind = DirectionPlayback, EventVolume
	case "capture_switch":
		event.Direction, event.Kind = DirectionCapture, EventSwitch
	case "capture_volume":
		event.Direction, event.Kind = DirectionCapture, EventVolume
	}
	return event
}

type binding struct {
	id   control.ElementID
	info control.ElementInfo
}

// EnumeratedElement is a raw ALSA MIXER enumeration. Its name is deliberately
// preserved because vendor controls do not follow reliable direction suffixes.
type EnumeratedElement struct {
	Name  string
	Index uint32
	h     *Handle
	item  binding
	items []string
}

// Items returns the kernel-reported item names in selection-index order.
func (e *EnumeratedElement) Items() []string { return append([]string(nil), e.items...) }

// Writable reports whether the enumeration is active and writable.
func (e *EnumeratedElement) Writable() bool {
	return e.item.info.Access.Writable() && !e.item.info.Access.Inactive()
}

// Values returns one selected item index per kernel-reported channel.
func (e *EnumeratedElement) Values() ([]uint32, error) {
	value, err := e.h.control.Read(e.item.id)
	if err != nil {
		return nil, err
	}
	return append([]uint32(nil), value.Enumerated...), nil
}

// SetValues selects one item index per kernel-reported channel. The raw
// control layer validates channel count, item bounds, activity and access.
func (e *EnumeratedElement) SetValues(values []uint32) error {
	return e.h.control.Write(control.ElementValue{
		ID: e.item.id, Type: control.TypeEnumerated, Enumerated: append([]uint32(nil), values...),
	})
}

// EnumeratedElements returns every enumerated MIXER control without inferring
// playback/capture direction from vendor-specific names.
func (h *Handle) EnumeratedElements() ([]*EnumeratedElement, error) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.enumerationsCached {
		return cloneEnumeratedElements(h.enumerationsCache), nil
	}
	ids, err := h.control.Elements()
	if err != nil {
		return nil, err
	}
	var result []*EnumeratedElement
	for _, id := range ids {
		if id.Interface != control.InterfaceMixer {
			continue
		}
		info, err := h.control.Info(id)
		if err != nil {
			return nil, err
		}
		if info.Type != control.TypeEnumerated {
			continue
		}
		item := &EnumeratedElement{Name: id.Name, Index: id.Index, h: h, item: binding{id: id, info: info}}
		item.items = make([]string, info.Enumerated.Items)
		for index := range item.items {
			itemInfo, err := h.control.EnumeratedItem(id, uint32(index))
			if err != nil {
				return nil, err
			}
			item.items[index] = itemInfo.Enumerated.ItemName
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Index < result[j].Index
	})
	h.enumerationsCache = result
	h.enumerationsCached = true
	return cloneEnumeratedElements(result), nil
}

func cloneEnumeratedElements(elements []*EnumeratedElement) []*EnumeratedElement {
	result := make([]*EnumeratedElement, len(elements))
	for index, element := range elements {
		clone := *element
		clone.items = append([]string(nil), element.items...)
		result[index] = &clone
	}
	return result
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
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.elementsCached {
		return cloneElements(h.elementsCache), nil
	}
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
	h.elementsCache = result
	h.elementsCached = true
	return cloneElements(result), nil
}

func cloneElements(elements []*Element) []*Element {
	result := make([]*Element, len(elements))
	for index, element := range elements {
		clone := *element
		clone.playbackSwitch = cloneBinding(element.playbackSwitch)
		clone.playbackVolume = cloneBinding(element.playbackVolume)
		clone.captureSwitch = cloneBinding(element.captureSwitch)
		clone.captureVolume = cloneBinding(element.captureVolume)
		result[index] = &clone
	}
	return result
}

func cloneBinding(item *binding) *binding {
	if item == nil {
		return nil
	}
	clone := *item
	return &clone
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
			if tlv.DB.Mute && raw <= integer.Minimum {
				return control.DBGainMute, true
			}
			return tlv.DB.Minimum + int32((raw-integer.Minimum)*int64(tlv.DB.Step)), true
		case control.TLVDBMinMax, control.TLVDBMinMaxMute:
			span := integer.Maximum - integer.Minimum
			if raw <= integer.Minimum || span <= 0 {
				if tlv.Type == control.TLVDBMinMaxMute {
					return control.DBGainMute, true
				}
				return tlv.DB.Minimum, true
			}
			if raw >= integer.Maximum {
				return tlv.DB.Maximum, true
			}
			return tlv.DB.Minimum + int32((raw-integer.Minimum)*int64(tlv.DB.Maximum-tlv.DB.Minimum)/span), true
		case control.TLVDBLinear:
			if raw <= integer.Minimum || integer.Maximum <= integer.Minimum {
				return tlv.DB.Minimum, true
			}
			if raw >= integer.Maximum {
				return tlv.DB.Maximum, true
			}
			value := float64(raw-integer.Minimum) / float64(integer.Maximum-integer.Minimum)
			if tlv.DB.Minimum <= control.DBGainMute {
				return int32(2000*math.Log10(value)) + tlv.DB.Maximum, true
			}
			linearMinimum := math.Pow(10, float64(tlv.DB.Minimum)/2000)
			linearMaximum := math.Pow(10, float64(tlv.DB.Maximum)/2000)
			amplitude := (linearMaximum-linearMinimum)*value + linearMinimum
			return int32(2000 * math.Log10(amplitude)), true
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
