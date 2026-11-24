//go:build linux && (amd64 || arm64)

package pcm

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/sparques/go-alsa/internal/uapi"
)

func TestStrictSuspendRecovery(t *testing.T) {
	t.Parallel()
	handle := &Handle{prepared: true, recovery: RecoveryStrict}
	retry, err := handle.recoverTransfer(context.Background(), syscall.ESTRPIPE, false)
	if retry || !errors.Is(err, ErrSuspended) || !errors.Is(err, syscall.ESTRPIPE) {
		t.Fatalf("recoverTransfer = retry %t, error %v", retry, err)
	}
	if handle.prepared {
		t.Fatal("suspended handle remained marked prepared")
	}
	if got := handle.Stats().Suspensions; got != 1 {
		t.Fatalf("suspensions = %d; want 1", got)
	}
}

func TestPollStateErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state State
		want  error
	}{
		{StateXRUN, ErrXRUN},
		{StateSuspended, ErrSuspended},
		{StateDisconnected, ErrDisconnected},
	}
	for _, test := range tests {
		if err := pollStateError(test.state, 8); !errors.Is(err, test.want) {
			t.Errorf("pollStateError(%s) = %v; want %v", test.state, err, test.want)
		}
	}
	if err := pollStateError(StateSuspended, 8); !errors.Is(err, syscall.ESTRPIPE) {
		t.Fatalf("suspended error = %v; want ESTRPIPE", err)
	}
	if err := pollStateError(StateRunning, 8); err == nil {
		t.Fatal("running stream POLLERR was ignored")
	}
}

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
