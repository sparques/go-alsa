//go:build linux

// Package sysio owns Linux file descriptors used by ALSA handles.
package sysio

import (
	"io"
	"sync"
	"syscall"
	"unsafe"
)

type systemCalls interface {
	Open(path string, flags int, mode uint32) (int, error)
	Close(fd int) error
	Ioctl(fd int, request uintptr, argument unsafe.Pointer) error
}

type linuxSystemCalls struct{}

func (linuxSystemCalls) Open(path string, flags int, mode uint32) (int, error) {
	return syscall.Open(path, flags, mode)
}

func (linuxSystemCalls) Close(fd int) error { return syscall.Close(fd) }

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
	mu     sync.RWMutex
	number int
	closed bool
	sys    systemCalls
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
	return &FD{number: number, sys: sys}, nil
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

// Close closes the descriptor after any in-flight operation completes. It is
// idempotent. Linux close errors are returned but close is never retried,
// because retrying can close an unrelated reused descriptor.
func (f *FD) Close() error {
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
