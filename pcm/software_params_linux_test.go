//go:build linux && (amd64 || arm64)

package pcm

import (
	"testing"

	"github.com/sparques/go-alsa/internal/uapi"
)

func TestRingBoundary(t *testing.T) {
	t.Parallel()
	for _, buffer := range []uint64{32, 4800, 96000} {
		boundary := ringBoundary(buffer)
		if boundary <= buffer {
			t.Errorf("ringBoundary(%d) = %d; want greater than buffer", buffer, boundary)
		}
		if boundary%buffer != 0 {
			t.Errorf("ringBoundary(%d) = %d; want whole multiple", buffer, boundary)
		}
	}
}

func TestValidateSoftwareConfig(t *testing.T) {
	t.Parallel()
	hardware := HardwareSetup{HardwareConfig: HardwareConfig{PeriodFrames: 480, BufferFrames: 4800}}
	valid := DefaultSoftwareConfig(true, hardware)
	if err := validateSoftwareConfig(valid, hardware); err != nil {
		t.Fatal(err)
	}
	invalid := []SoftwareConfig{
		{},
		{AvailableMinimum: 4801},
		{AvailableMinimum: 480, StartThreshold: ringBoundary(4800) + 1},
		{AvailableMinimum: 480, TimestampMode: TimestampMode(2)},
		{AvailableMinimum: 480, TimestampClock: TimestampClock(3)},
	}
	for _, config := range invalid {
		if err := validateSoftwareConfig(config, hardware); err == nil {
			t.Errorf("validateSoftwareConfig(%#v) succeeded", config)
		}
	}
}

func TestSoftwareParameterConstruction(t *testing.T) {
	t.Parallel()
	hardware := HardwareSetup{HardwareConfig: HardwareConfig{PeriodFrames: 480, BufferFrames: 4800}}
	config := SoftwareConfig{
		AvailableMinimum: 480,
		StartThreshold:   960,
		StopThreshold:    4800,
		SilenceThreshold: 24,
		SilenceFrames:    48,
		TimestampMode:    TimestampEnabled,
		TimestampClock:   TimestampMonotonicRaw,
	}
	params := softwareParams(config, hardware, uapi.PCMProtocolVersion)
	if params.TimestampMode != int32(TimestampEnabled) || params.TimestampType != uint32(TimestampMonotonicRaw) {
		t.Fatalf("timestamp parameters = mode %d, type %d", params.TimestampMode, params.TimestampType)
	}
	if params.AvailableMinimum != 480 || params.StartThreshold != 960 || params.StopThreshold != 4800 {
		t.Fatalf("threshold parameters = %#v", params)
	}
	if params.SilenceThreshold != 24 || params.SilenceSize != 48 {
		t.Fatalf("silence parameters = %#v", params)
	}
	if params.Protocol != uapi.PCMProtocolVersion || params.Boundary != ringBoundary(4800) {
		t.Fatalf("protocol/boundary parameters = %#v", params)
	}
}
