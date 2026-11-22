//go:build linux && (amd64 || arm64)

package pcm

import (
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
