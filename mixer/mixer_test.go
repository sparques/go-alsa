package mixer

import (
	"math"
	"testing"

	"github.com/sparques/go-alsa/control"
)

func TestSplitSimpleName(t *testing.T) {
	tests := []struct {
		name string
		base string
		kind string
		ok   bool
	}{
		{"Speaker Playback Volume", "Speaker", "playback_volume", true},
		{"Mic Capture Switch", "Mic", "capture_switch", true},
		{" Playback Volume", "", "", false},
		{"Auto Gain Control", "", "", false},
	}
	for _, test := range tests {
		base, kind, ok := splitSimpleName(test.name)
		if base != test.base || kind != test.kind || ok != test.ok {
			t.Errorf("splitSimpleName(%q) = %q, %q, %v; want %q, %q, %v",
				test.name, base, kind, ok, test.base, test.kind, test.ok)
		}
	}
}

func TestClassifyEvent(t *testing.T) {
	raw := control.Event{
		Mask: control.EventValue,
		ID:   control.ElementID{Interface: control.InterfaceMixer, Name: "Speaker Playback Volume", Index: 2},
	}
	got := classifyEvent(raw)
	if got.Raw != raw || !got.Recognized || got.Name != "Speaker" || got.Index != 2 ||
		got.Direction != DirectionPlayback || got.Kind != EventVolume {
		t.Fatalf("classifyEvent() = %+v", got)
	}

	unknownRaw := control.Event{ID: control.ElementID{Interface: control.InterfaceCard, Name: "Speaker Playback Volume"}}
	unknown := classifyEvent(unknownRaw)
	if unknown.Raw != unknownRaw || unknown.Recognized || unknown.Direction != DirectionUnknown || unknown.Kind != EventUnknown {
		t.Fatalf("classifyEvent(unknown) = %+v", unknown)
	}
}

func TestNormalizeRaw(t *testing.T) {
	integer := control.IntegerRange{Minimum: -10, Maximum: 30, Step: 4}
	for _, test := range []struct {
		raw  int64
		want float64
	}{{-10, 0}, {10, 0.5}, {30, 1}} {
		if got := normalizeRaw(test.raw, integer); math.Abs(got-test.want) > 1e-12 {
			t.Errorf("normalizeRaw(%d) = %v; want %v", test.raw, got, test.want)
		}
	}
}

func TestDenormalizeRaw(t *testing.T) {
	integer := control.IntegerRange{Minimum: 0, Maximum: 10, Step: 2}
	for _, test := range []struct {
		value float64
		want  int64
	}{{0, 0}, {0.24, 2}, {0.5, 6}, {1, 10}} {
		got, err := denormalizeRaw(test.value, integer)
		if err != nil {
			t.Fatalf("denormalizeRaw(%v): %v", test.value, err)
		}
		if got != test.want {
			t.Errorf("denormalizeRaw(%v) = %d; want %d", test.value, got, test.want)
		}
	}
	for _, value := range []float64{-0.01, 1.01, math.NaN(), math.Inf(1)} {
		if _, err := denormalizeRaw(value, integer); err == nil {
			t.Errorf("denormalizeRaw(%v) unexpectedly succeeded", value)
		}
	}
}

func TestRawToDB(t *testing.T) {
	tests := []struct {
		name    string
		raw     int64
		integer control.IntegerRange
		tlv     control.TLV
		want    int32
		ok      bool
	}{
		{
			name:    "scale",
			raw:     10,
			integer: control.IntegerRange{Minimum: 0, Maximum: 20},
			tlv:     control.TLV{Type: control.TLVDBScale, DB: &control.DB{Minimum: -3000, Step: 50}},
			want:    -2500,
			ok:      true,
		},
		{
			name:    "min max midpoint",
			raw:     50,
			integer: control.IntegerRange{Minimum: 0, Maximum: 100},
			tlv:     control.TLV{Type: control.TLVDBMinMax, DB: &control.DB{Minimum: -6000, Maximum: 0}},
			want:    -3000,
			ok:      true,
		},
		{
			name:    "range",
			raw:     75,
			integer: control.IntegerRange{Minimum: 0, Maximum: 100},
			tlv: control.TLV{Type: control.TLVDBRange, DBRanges: []control.DBRange{
				{MinimumValue: 0, MaximumValue: 49, Curve: control.TLV{Type: control.TLVDBScale, DB: &control.DB{Minimum: -6000, Step: 100}}},
				{MinimumValue: 50, MaximumValue: 100, Curve: control.TLV{Type: control.TLVDBMinMax, DB: &control.DB{Minimum: -1000, Maximum: 0}}},
			}},
			want: -500,
			ok:   true,
		},
		{
			name:    "unsupported linear",
			raw:     1,
			integer: control.IntegerRange{Minimum: 0, Maximum: 2},
			tlv:     control.TLV{Type: control.TLVDBLinear, DB: &control.DB{Minimum: -6000, Maximum: 0, Linear: true}},
			ok:      false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := rawToDB(test.raw, test.integer, test.tlv)
			if got != test.want || ok != test.ok {
				t.Fatalf("rawToDB() = %d, %v; want %d, %v", got, ok, test.want, test.ok)
			}
		})
	}
}
