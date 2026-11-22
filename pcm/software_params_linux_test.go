//go:build linux && (amd64 || arm64)

package pcm

import "testing"

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
	}
	for _, config := range invalid {
		if err := validateSoftwareConfig(config, hardware); err == nil {
			t.Errorf("validateSoftwareConfig(%#v) succeeded", config)
		}
	}
}
