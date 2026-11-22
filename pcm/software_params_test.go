package pcm

import "testing"

func TestDefaultSoftwareConfig(t *testing.T) {
	t.Parallel()
	hardware := HardwareSetup{HardwareConfig: HardwareConfig{PeriodFrames: 480, BufferFrames: 5000}}
	playback := DefaultSoftwareConfig(true, hardware)
	if playback.AvailableMinimum != 480 || playback.StartThreshold != 4800 || playback.StopThreshold != 5000 {
		t.Fatalf("playback defaults = %#v", playback)
	}
	capture := DefaultSoftwareConfig(false, hardware)
	if capture.AvailableMinimum != 480 || capture.StartThreshold != 1 || capture.StopThreshold != 5000 {
		t.Fatalf("capture defaults = %#v", capture)
	}
}
