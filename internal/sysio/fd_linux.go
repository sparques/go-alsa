//go:build linux

// Package sysio owns Linux file descriptors used by ALSA handles.
package sysio

import (
	"context"
	"io"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type systemCalls interface {
	Open(path string, flags int, mode uint32) (int, error)
	Close(fd int) error
	Ioctl(fd int, request uintptr, argument unsafe.Pointer) error
	Read(fd int, buffer []byte) (int, error)
}

type linuxSystemCalls struct{}

func (linuxSystemCalls) Open(path string, flags int, mode uint32) (int, error) {
	return syscall.Open(path, flags, mode)
}

func (linuxSystemCalls) Close(fd int) error { return syscall.Close(fd) }

func (linuxSystemCalls) Read(fd int, buffer []byte) (int, error) {
	for {
		count, err := syscall.Read(fd, buffer)
		if err != syscall.EINTR {
			return count, err
		}
	}
}

func (linuxSystemCalls) Ioctl(fd int, request uintptr, argument unsafe.Pointer) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(argument))
		if errno == 0 {
			return nil
		}
		if errno != syscall.EINTR {
			return errno
		}
	}
}

// FD prevents a descriptor from being closed and reused while an operation is
// in flight. ALSA descriptors are opened nonblocking, so operations holding a
// read lock must not sleep indefinitely in the kernel.
type FD struct {
	mu          sync.RWMutex
	number      int
	closed      bool
	sys         systemCalls
	closing     chan struct{}
	closeSignal sync.Once
}

// Open opens path with close-on-exec and returns an owned descriptor.
func Open(path string, flags int) (*FD, error) {
	return openWith(path, flags|syscall.O_CLOEXEC, linuxSystemCalls{})
}

func openWith(path string, flags int, sys systemCalls) (*FD, error) {
	number, err := sys.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	return &FD{number: number, sys: sys, closing: make(chan struct{})}, nil
}

// Ioctl executes one ioctl while preventing concurrent descriptor reuse.
func (f *FD) Ioctl(request uintptr, argument unsafe.Pointer) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return io.ErrClosedPipe
	}
	return f.sys.Ioctl(f.number, request, argument)
}

// Read performs one nonblocking descriptor read while preventing reuse.
func (f *FD) Read(buffer []byte) (int, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return 0, io.ErrClosedPipe
	}
	return f.sys.Read(f.number, buffer)
}

// Close closes the descriptor after any in-flight operation completes. It is
// idempotent. Linux close errors are returned but close is never retried,
// because retrying can close an unrelated reused descriptor.
func (f *FD) Close() error {
	f.closeSignal.Do(func() { close(f.closing) })
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	err := f.sys.Close(f.number)
	f.number = -1
	return err
}

// Wait blocks until the descriptor has one of events, the context is canceled,
// or Close begins. An eventfd wakes poll without periodic timeout latency.
func (f *FD) Wait(ctx context.Context, events int16) (int16, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return 0, io.ErrClosedPipe
	}
	wake, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return 0, err
	}
	defer unix.Close(wake)

	done := make(chan struct{})
	wakerDone := make(chan struct{})
	go func() {
		defer close(wakerDone)
		select {
		case <-ctx.Done():
		case <-f.closing:
		case <-done:
			return
		}
		var value [8]byte
		value[0] = 1
		_, _ = unix.Write(wake, value[:])
	}()

	pollFDs := []unix.PollFd{
		{Fd: int32(f.number), Events: events},
		{Fd: int32(wake), Events: unix.POLLIN},
	}
	for {
		_, err = unix.Ppoll(pollFDs, nil, nil)
		if err == syscall.EINTR {
			continue
		}
		break
	}
	close(done)
	<-wakerDone
	if err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	select {
	case <-f.closing:
		return 0, io.ErrClosedPipe
	default:
	}
	return pollFDs[0].Revents, nil
}
