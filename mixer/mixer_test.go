package mixer

import (
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
