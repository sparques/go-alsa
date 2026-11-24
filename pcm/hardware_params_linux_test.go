//go:build linux && (amd64 || arm64)

package pcm

import (
	"syscall"
	"testing"

	"github.com/sparques/go-alsa/internal/uapi"
)

func TestHardwareParameterConstruction(t *testing.T) {
	t.Parallel()
	params := anyHardwareParams()
	setMask(&params, uapi.PCMHWParamAccess, uint32(RWInterleaved))
	setMask(&params, uapi.PCMHWParamFormat, uint32(S16LE))
	setInterval(&params, uapi.PCMHWParamChannels, 2)
	setInterval(&params, uapi.PCMHWParamRate, 48000)

	if got := singleMaskValue(params, uapi.PCMHWParamAccess); got != int(RWInterleaved) {
		t.Fatalf("access = %d; want %d", got, RWInterleaved)
	}
	if got := singleMaskValue(params, uapi.PCMHWParamFormat); got != int(S16LE) {
		t.Fatalf("format = %d; want %d", got, S16LE)
	}
	if got := intervalValue(params, uapi.PCMHWParamChannels); got != 2 {
		t.Fatalf("channels = %d; want 2", got)
	}
	interval := params.Intervals[uapi.PCMHWParamRate-uapi.PCMHWParamFirstInterval]
	if interval.Min != 48000 || interval.Max != 48000 || interval.Flags != uapi.PCMIntervalInteger {
		t.Fatalf("rate interval = %#v", interval)
	}
}

func TestIntegerBounds(t *testing.T) {
	t.Parallel()
	minimum, maximum, ok := integerBounds(uapi.PCMInterval{
		Min: 10, Max: 20, Flags: uapi.PCMIntervalInteger | uapi.PCMIntervalOpenMin | uapi.PCMIntervalOpenMax,
	})
	if !ok || minimum != 11 || maximum != 19 {
		t.Fatalf("integerBounds = %d, %d, %t", minimum, maximum, ok)
	}
	if _, _, ok := integerBounds(uapi.PCMInterval{Flags: uapi.PCMIntervalEmpty}); ok {
		t.Fatal("empty interval reported values")
	}
}

func TestNearestCandidate(t *testing.T) {
	t.Parallel()
	lower := nearCandidate{value: 44100}
	upper := nearCandidate{value: 48000}
	chosen, err := nearestCandidate(47000, lower, upper)
	if err != nil || chosen.value != 48000 {
		t.Fatalf("nearestCandidate = %d, %v; want 48000", chosen.value, err)
	}
	chosen, err = nearestCandidate(46050, lower, upper)
	if err != nil || chosen.value != 44100 {
		t.Fatalf("tie nearestCandidate = %d, %v; want lower 44100", chosen.value, err)
	}
	lower.err = syscall.EINVAL
	chosen, err = nearestCandidate(44100, lower, upper)
	if err != nil || chosen.value != 48000 {
		t.Fatalf("one-sided nearestCandidate = %d, %v", chosen.value, err)
	}
}

func TestValidateHardwareConfig(t *testing.T) {
	t.Parallel()
	valid := HardwareConfig{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800}
	if err := validateHardwareConfig(valid); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	tests := []HardwareConfig{
		{},
		{Access: 99, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800},
		{Access: RWInterleaved, Format: 99, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800},
		{Access: RWInterleaved, Format: S16LE, Channels: 0, Rate: 48000, PeriodFrames: 480, BufferFrames: 4800},
		{Access: RWInterleaved, Format: S16LE, Channels: 2, Rate: 48000, PeriodFrames: 480, BufferFrames: 100},
	}
	for _, config := range tests {
		if err := validateHardwareConfig(config); err == nil {
			t.Errorf("validateHardwareConfig(%#v) succeeded", config)
		}
	}
}

func TestHardwareCapabilities(t *testing.T) {
	t.Parallel()
	setup := HardwareSetup{InfoFlags: hardwareInfoPause | hardwareInfoResume}
	if !setup.CanPause() || !setup.CanResume() {
		t.Fatalf("capabilities not reported: %#v", setup)
	}
	if (HardwareSetup{}).CanPause() || (HardwareSetup{}).CanResume() {
		t.Fatal("zero setup reported capabilities")
	}
}

func TestCapabilitiesFromParams(t *testing.T) {
	t.Parallel()
	params := anyHardwareParams()
	setMask(&params, uapi.PCMHWParamAccess, uint32(RWInterleaved))
	setMask(&params, uapi.PCMHWParamFormat, uint32(S16LE))
	params.Intervals[uapi.PCMHWParamChannels-uapi.PCMHWParamFirstInterval] = uapi.PCMInterval{Min: 1, Max: 2, Flags: uapi.PCMIntervalInteger}
	params.Intervals[uapi.PCMHWParamRate-uapi.PCMHWParamFirstInterval] = uapi.PCMInterval{Min: 44100, Max: 48000, Flags: uapi.PCMIntervalOpenMin}

	got := capabilitiesFromParams(params)
	if len(got.Accesses) != 1 || got.Accesses[0] != RWInterleaved {
		t.Fatalf("accesses = %#v", got.Accesses)
	}
	if len(got.Formats) != 1 || got.Formats[0] != S16LE {
		t.Fatalf("formats = %#v", got.Formats)
	}
	if got.Channels.Minimum != 1 || got.Channels.Maximum != 2 || !got.Channels.Integer {
		t.Fatalf("channels = %#v", got.Channels)
	}
	if got.Rate.Minimum != 44100 || got.Rate.Maximum != 48000 || !got.Rate.OpenMinimum {
		t.Fatalf("rate = %#v", got.Rate)
	}
}

func TestAccessAndFormatStrings(t *testing.T) {
	t.Parallel()
	if RWInterleaved.String() != "RW_INTERLEAVED" || Access(99).String() != "UNKNOWN" {
		t.Fatal("unexpected access string")
	}
	if S16LE.String() != "S16_LE" || Format(99).String() != "UNKNOWN" {
		t.Fatal("unexpected format string")
	}
}

func TestFormatPhysicalBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		format Format
		bytes  int
	}{
		{S8, 1}, {S16LE, 2}, {S24_3LE, 3}, {S24LE, 4}, {S32LE, 4},
	}
	for _, test := range tests {
		got, ok := test.format.PhysicalBytes()
		if !ok || got != test.bytes {
			t.Errorf("%s PhysicalBytes = %d, %t; want %d, true", test.format, got, ok, test.bytes)
		}
	}
	if _, ok := Format(99).PhysicalBytes(); ok {
		t.Fatal("unknown format has a physical width")
	}
}
