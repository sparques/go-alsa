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
