//go:build linux && (amd64 || arm64)

package pcm

import (
	"testing"

	"github.com/sparques/go-alsa/internal/uapi"
)

func TestStatusFromUAPI(t *testing.T) {
	t.Parallel()
	raw := uapi.PCMStatus{
		State:                  int32(StateRunning),
		TriggerTimestamp:       uapi.Timespec{Seconds: 1, Nanoseconds: 2},
		Timestamp:              uapi.Timespec{Seconds: 3, Nanoseconds: 4},
		ApplicationPointer:     5,
		HardwarePointer:        6,
		Delay:                  7,
		Available:              8,
		MaximumAvailable:       9,
		Overrange:              10,
		SuspendedState:         int32(StatePrepared),
		AudioTimestamp:         uapi.Timespec{Seconds: 11, Nanoseconds: 12},
		DriverTimestamp:        uapi.Timespec{Seconds: 13, Nanoseconds: 14},
		AudioTimestampAccuracy: 15,
	}
	got := statusFromUAPI(raw)
	if got.State != StateRunning || got.ApplicationPointer != 5 || got.DelayFrames != 7 || got.AudioTimestamp.Nanoseconds != 12 || got.AudioTimestampAccuracy != 15 {
		t.Fatalf("status = %#v", got)
	}
}

func TestStateString(t *testing.T) {
	t.Parallel()
	if got := StateDraining.String(); got != "DRAINING" {
		t.Fatalf("StateDraining.String() = %q", got)
	}
	if got := State(99).String(); got != "UNKNOWN" {
		t.Fatalf("State(99).String() = %q", got)
	}
}
