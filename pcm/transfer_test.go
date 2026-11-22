package pcm

import (
	"context"
	"errors"
	"testing"
)

func TestTransferRequiresPreparedHandle(t *testing.T) {
	t.Parallel()
	handle := &Handle{}
	if _, err := handle.ReadInterleaved(context.Background(), []int16{0}); !errors.Is(err, ErrState) {
		t.Fatalf("ReadInterleaved error = %v; want ErrState", err)
	}
	if _, err := handle.WriteInterleaved(context.Background(), []int16{0}); !errors.Is(err, ErrState) {
		t.Fatalf("WriteInterleaved error = %v; want ErrState", err)
	}
	if err := handle.Start(); !errors.Is(err, ErrState) {
		t.Fatalf("Start error = %v; want ErrState", err)
	}
}

func TestRecoveryPolicyValidation(t *testing.T) {
	t.Parallel()
	handle := &Handle{}
	if err := handle.SetRecoveryPolicy(RecoveryAutomatic); err != nil {
		t.Fatal(err)
	}
	if err := handle.SetRecoveryPolicy(RecoveryPolicy(99)); err == nil {
		t.Fatal("invalid recovery policy succeeded")
	}
}

func TestNonInterleavedBufferValidation(t *testing.T) {
	t.Parallel()
	handle := &Handle{
		prepared: true,
		hardware: &HardwareSetup{HardwareConfig: HardwareConfig{Access: RWNonInterleaved, Format: S16LE, Channels: 2}},
		stream:   streamPlayback,
	}
	tests := [][][]byte{
		nil,
		{{0, 0}},
		{{0}, {0}},
		{{0, 0}, {0, 0, 0, 0}},
	}
	for _, channels := range tests {
		if _, err := handle.WriteNonInterleavedBytes(context.Background(), channels); err == nil {
			t.Errorf("WriteNonInterleavedBytes(%v) succeeded", channels)
		}
	}
}
