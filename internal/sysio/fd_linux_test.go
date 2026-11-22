//go:build linux

package sysio

import (
	"errors"
	"io"
	"sync"
	"syscall"
	"testing"
	"unsafe"
)

type fakeSystemCalls struct {
	mu         sync.Mutex
	fd         int
	openErr    error
	ioctlErr   error
	closeErr   error
	closeCount int
	ioctlStart chan struct{}
	ioctlDone  chan struct{}
}

func (f *fakeSystemCalls) Open(string, int, uint32) (int, error) {
	return f.fd, f.openErr
}

func (f *fakeSystemCalls) Close(fd int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fd != f.fd {
		return syscall.EBADF
	}
	f.closeCount++
	return f.closeErr
}

func (f *fakeSystemCalls) Ioctl(fd int, _ uintptr, _ unsafe.Pointer) error {
	if f.ioctlStart != nil {
		close(f.ioctlStart)
		<-f.ioctlDone
	}
	if fd != f.fd {
		return syscall.EBADF
	}
	return f.ioctlErr
}

func TestFDCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	sys := &fakeSystemCalls{fd: 12}
	fd, err := openWith("ignored", syscall.O_RDWR, sys)
	if err != nil {
		t.Fatal(err)
	}
	if err := fd.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fd.Close(); err != nil {
		t.Fatal(err)
	}
	if sys.closeCount != 1 {
		t.Fatalf("close count = %d; want 1", sys.closeCount)
	}
	if err := fd.Ioctl(0, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("ioctl after close = %v; want io.ErrClosedPipe", err)
	}
}

func TestFDCloseWaitsForOperation(t *testing.T) {
	t.Parallel()
	sys := &fakeSystemCalls{fd: 12, ioctlStart: make(chan struct{}), ioctlDone: make(chan struct{})}
	fd, err := openWith("ignored", syscall.O_RDWR, sys)
	if err != nil {
		t.Fatal(err)
	}

	operationDone := make(chan error, 1)
	go func() { operationDone <- fd.Ioctl(1, nil) }()
	<-sys.ioctlStart
	closeDone := make(chan error, 1)
	go func() { closeDone <- fd.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before in-flight ioctl completed: %v", err)
	default:
	}
	close(sys.ioctlDone)
	if err := <-operationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}
