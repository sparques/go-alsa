//go:build linux

package sysio

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
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
	readData   []byte
	written    []byte
	eventFD    int
	pollStart  chan struct{}
	pollDone   chan struct{}
	pollOnce   sync.Once
}

func (f *fakeSystemCalls) Read(fd int, buffer []byte) (int, error) {
	if fd != f.fd {
		return 0, syscall.EBADF
	}
	if f.readData == nil {
		return 0, syscall.EAGAIN
	}
	return copy(buffer, f.readData), nil
}

func (f *fakeSystemCalls) Write(fd int, buffer []byte) (int, error) {
	if fd == f.eventFD && f.pollDone != nil {
		f.pollOnce.Do(func() { close(f.pollDone) })
		return len(buffer), nil
	}
	if fd != f.fd {
		return 0, syscall.EBADF
	}
	f.written = append(f.written, buffer...)
	return len(buffer), nil
}

func (f *fakeSystemCalls) Eventfd(uint, int) (int, error) { return f.eventFD, nil }

func (f *fakeSystemCalls) Ppoll(fds []unix.PollFd, _ *unix.Timespec, _ *unix.Sigset_t) (int, error) {
	if f.pollStart != nil {
		close(f.pollStart)
	}
	if f.pollDone != nil {
		<-f.pollDone
	}
	fds[1].Revents = unix.POLLIN
	return 1, nil
}

func TestFDWaitCancellation(t *testing.T) {
	t.Parallel()
	pipe := make([]int, 2)
	if err := unix.Pipe2(pipe, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pipe[1])
	fd := &FD{number: pipe[0], sys: linuxSystemCalls{}, closing: make(chan struct{})}
	defer fd.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fd.Wait(ctx, unix.POLLIN); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v; want context.Canceled", err)
	}
}

func TestFDWaitWakesOnClose(t *testing.T) {
	t.Parallel()
	pipe := make([]int, 2)
	if err := unix.Pipe2(pipe, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pipe[1])
	fd := &FD{number: pipe[0], sys: linuxSystemCalls{}, closing: make(chan struct{})}
	waitDone := make(chan error, 1)
	go func() {
		_, err := fd.Wait(context.Background(), unix.POLLIN)
		waitDone <- err
	}()

	closeDone := make(chan error, 1)
	go func() { closeDone <- fd.Close() }()
	select {
	case err := <-waitDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Wait error = %v; want io.ErrClosedPipe", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not wake when Close began")
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}

func (f *fakeSystemCalls) Open(string, int, uint32) (int, error) {
	return f.fd, f.openErr
}

func (f *fakeSystemCalls) Close(fd int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fd == f.eventFD {
		return nil
	}
	if fd != f.fd {
		return syscall.EBADF
	}
	f.closeCount++
	return f.closeErr
}

func TestFDWaitDeterministicCancellation(t *testing.T) {
	t.Parallel()
	sys := &fakeSystemCalls{
		fd: 12, eventFD: 99, pollStart: make(chan struct{}), pollDone: make(chan struct{}),
	}
	fd, err := openWith("ignored", syscall.O_RDWR, sys)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fd.Wait(ctx, unix.POLLIN)
		done <- err
	}()
	<-sys.pollStart
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v; want context.Canceled", err)
	}
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
	if _, err := fd.Read(nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("read after close = %v; want io.ErrClosedPipe", err)
	}
	if _, err := fd.Write(nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after close = %v; want io.ErrClosedPipe", err)
	}
}

func TestFDWithNumberPinsDescriptor(t *testing.T) {
	t.Parallel()
	sys := &fakeSystemCalls{fd: 12}
	fd, err := openWith("ignored", syscall.O_RDWR, sys)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	callbackDone := make(chan error, 1)
	go func() {
		callbackDone <- fd.WithNumber(func(number int) error {
			if number != 12 {
				t.Errorf("number = %d; want 12", number)
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- fd.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned while descriptor callback was active: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-callbackDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := fd.WithNumber(func(int) error { return nil }); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("WithNumber after Close = %v; want io.ErrClosedPipe", err)
	}
}

func TestFDReadWriteSeams(t *testing.T) {
	t.Parallel()
	sys := &fakeSystemCalls{fd: 12, readData: []byte("input")}
	fd, err := openWith("ignored", syscall.O_RDWR, sys)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	buffer := make([]byte, 8)
	count, err := fd.Read(buffer)
	if err != nil || string(buffer[:count]) != "input" {
		t.Fatalf("Read = %q, %v", buffer[:count], err)
	}
	count, err = fd.Write([]byte("output"))
	if err != nil || count != 6 || string(sys.written) != "output" {
		t.Fatalf("Write = %d, %v; captured %q", count, err, sys.written)
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
