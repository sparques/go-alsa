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

func TestOperationErrorClassifiesDisconnection(t *testing.T) {
	t.Parallel()
	for _, errno := range []error{syscall.ENODEV, syscall.ENXIO} {
		err := operationError("status", errno)
		if !errors.Is(err, ErrDisconnected) || !errors.Is(err, errno) {
			t.Errorf("operationError(%v) = %v; want ErrDisconnected and original errno", errno, err)
		}
	}
	err := operationError("status", syscall.EINVAL)
	if errors.Is(err, ErrDisconnected) || !errors.Is(err, syscall.EINVAL) {
		t.Fatalf("operationError(EINVAL) = %v", err)
	}
}

func TestLinkValidation(t *testing.T) {
	t.Parallel()
	playback := &Handle{stream: streamPlayback}
	if err := playback.Link(nil); err == nil {
		t.Fatal("Link(nil) succeeded")
	}
	if err := playback.Link(playback); err == nil {
		t.Fatal("self Link succeeded")
	}
	capture := &Handle{stream: streamCapture}
	if err := playback.Link(capture); !errors.Is(err, ErrDirection) {
		t.Fatalf("opposite-direction Link = %v; want ErrDirection", err)
	}
	peer := &Handle{stream: streamPlayback}
	if err := playback.Link(peer); !errors.Is(err, ErrState) {
		t.Fatalf("unconfigured Link = %v; want ErrState", err)
	}
}

func TestLifecycleRequiresHardwareConfiguration(t *testing.T) {
	t.Parallel()
	handle := &Handle{}
	tests := []struct {
		name string
		call func() error
	}{
		{"reset", handle.Reset},
		{"pause", func() error { return handle.Pause(true) }},
		{"resume", handle.Resume},
		{"rewind", func() error { _, err := handle.Rewind(1); return err }},
		{"forward", func() error { _, err := handle.Forward(1); return err }},
		{"sync", handle.SyncHardware},
	}
	for _, test := range tests {
		if err := test.call(); !errors.Is(err, ErrState) {
			t.Errorf("%s error = %v; want ErrState", test.name, err)
		}
	}
}

func TestPauseResumeCapabilities(t *testing.T) {
	t.Parallel()
	handle := &Handle{hardware: &HardwareSetup{}, prepared: true}
	if err := handle.Pause(true); err == nil {
		t.Fatal("Pause succeeded without hardware support")
	}
	if err := handle.Resume(); err == nil {
		t.Fatal("Resume succeeded without hardware support")
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
	if err := pollStateError(StateXRUN, 8); !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("xrun error = %v; want EPIPE", err)
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
