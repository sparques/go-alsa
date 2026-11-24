package alsa

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestDeviceErrorClassifiesErrnoAndPreservesCause(t *testing.T) {
	tests := []struct {
		name       string
		errno      error
		classified error
	}{
		{name: "access", errno: syscall.EACCES, classified: ErrPermission},
		{name: "operation not permitted", errno: syscall.EPERM, classified: ErrPermission},
		{name: "busy", errno: syscall.EBUSY, classified: ErrDeviceBusy},
		{name: "absent path", errno: syscall.ENOENT, classified: ErrDeviceNotFound},
		{name: "absent device", errno: syscall.ENODEV, classified: ErrDeviceNotFound},
		{name: "no such device address", errno: syscall.ENXIO, classified: ErrDeviceNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cause := fmt.Errorf("open failed: %w", test.errno)
			err := &DeviceError{Op: "open", Device: "hw:3,0", Err: cause}
			if !errors.Is(err, test.classified) {
				t.Fatalf("errors.Is(%v, %v) = false", err, test.classified)
			}
			if !errors.Is(err, test.errno) {
				t.Fatalf("errors.Is(%v, %v) = false; original errno was lost", err, test.errno)
			}
			var deviceError *DeviceError
			if !errors.As(err, &deviceError) || deviceError.Device != "hw:3,0" {
				t.Fatalf("errors.As(%v) = %#v", err, deviceError)
			}
		})
	}
}

func TestDeviceErrorDoesNotMisclassifyErrno(t *testing.T) {
	err := &DeviceError{Op: "open", Device: "hw:0", Err: syscall.EINVAL}
	for _, target := range []error{ErrPermission, ErrDeviceBusy, ErrDeviceNotFound, ErrDisconnected} {
		if errors.Is(err, target) {
			t.Fatalf("errors.Is(%v, %v) = true", err, target)
		}
	}
}
