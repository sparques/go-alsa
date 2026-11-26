package pcm

import (
	"errors"
	"testing"

	alsa "github.com/sparques/go-alsa"
)

func TestSelectPlugTargetPrefersExact(t *testing.T) {
	capabilities := Capabilities{
		Accesses: []Access{RWNonInterleaved, RWInterleaved},
		Formats:  []Format{S16BE, U8, S16LE},
		Channels: Range{Minimum: 1, Maximum: 8, Integer: true},
		Rate:     Range{Minimum: 8000, Maximum: 192000, Integer: true},
	}
	application := ConversionParams{Format: U8, Channels: 2, Rate: 48000}
	target, err := SelectPlugTarget(application, capabilities)
	if err != nil || target.Params != application || target.Access != RWInterleaved {
		t.Fatalf("SelectPlugTarget = %#v, %v", target, err)
	}
}

func TestSelectPlugTargetChoosesNearest(t *testing.T) {
	capabilities := Capabilities{
		Accesses: []Access{RWInterleaved},
		Formats:  []Format{S32BE, U16LE, S16LE},
		Channels: Range{Minimum: 2, Maximum: 6, OpenMaximum: true, Integer: true},
		Rate:     Range{Minimum: 44100, Maximum: 96000, Integer: true},
	}
	application := ConversionParams{Format: S24_3LE, Channels: 1, Rate: 192000}
	target, err := SelectPlugTarget(application, capabilities)
	want := ConversionParams{Format: S32BE, Channels: 2, Rate: 96000}
	if err != nil || target.Params != want {
		t.Fatalf("SelectPlugTarget = %#v, %v; want %#v", target, err, want)
	}
}

func TestSelectPlugTargetValidation(t *testing.T) {
	application := ConversionParams{Format: S16LE, Channels: 2, Rate: 48000}
	tests := []Capabilities{
		{Formats: []Format{S16LE}, Channels: Range{Minimum: 1, Maximum: 2}, Rate: Range{Minimum: 48000, Maximum: 48000}},
		{Accesses: []Access{RWInterleaved}, Channels: Range{Minimum: 1, Maximum: 2}, Rate: Range{Minimum: 48000, Maximum: 48000}},
		{Accesses: []Access{RWInterleaved}, Formats: []Format{S16LE}, Channels: Range{Empty: true}, Rate: Range{Minimum: 48000, Maximum: 48000}},
	}
	for index, capabilities := range tests {
		if _, err := SelectPlugTarget(application, capabilities); err == nil {
			t.Errorf("invalid capabilities %d succeeded", index)
		}
	}
}

func TestNamedPlanConverterForCapabilities(t *testing.T) {
	plan := NamedPlan{Stream: alsa.Playback, RequiresPlug: true}
	application := ConversionParams{Format: U8, Channels: 1, Rate: 16000}
	capabilities := Capabilities{
		Accesses: []Access{RWInterleaved}, Formats: []Format{S16LE},
		Channels: Range{Minimum: 2, Maximum: 2}, Rate: Range{Minimum: 48000, Maximum: 48000},
	}
	target, converter, err := plan.ConverterForCapabilities(application, capabilities, nil)
	if err != nil || converter == nil || target.Params != (ConversionParams{Format: S16LE, Channels: 2, Rate: 48000}) {
		t.Fatalf("ConverterForCapabilities = %#v, %#v, %v", target, converter, err)
	}
	plan.RequiresPlug = false
	if _, _, err := plan.ConverterForCapabilities(application, capabilities, nil); !errors.Is(err, ErrConversionRequired) {
		t.Fatalf("direct plan conversion error = %v", err)
	}
}
