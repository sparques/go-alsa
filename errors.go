package alsa

import (
	"errors"
	"io/fs"
	"syscall"
)

var (
	// ErrUnsupportedPlatform reports that the current operating system or
	// architecture has no native ALSA implementation.
	ErrUnsupportedPlatform = errors.New("alsa: unsupported platform")
	// ErrMalformedDevice reports a device node whose name does not follow the
	// Linux ALSA convention expected by the resolver.
	ErrMalformedDevice = errors.New("alsa: malformed device")
	// ErrDeviceNotFound reports that no ALSA card or PCM matches a selector.
	ErrDeviceNotFound = errors.New("alsa: device not found")
	// ErrPermission reports that the process cannot access an ALSA device.
	ErrPermission = errors.New("alsa: permission denied")
	// ErrDeviceBusy reports that an ALSA device or subdevice is already in use.
	ErrDeviceBusy = errors.New("alsa: device busy")
	// ErrDisconnected reports hardware removal or a disconnected device.
	ErrDisconnected = errors.New("alsa: device disconnected")
	// ErrDirectionUnavailable reports that a PCM exists, but not in the
	// requested playback or capture direction.
	ErrDirectionUnavailable = errors.New("alsa: PCM direction unavailable")
	// ErrSubdeviceUnavailable reports a subdevice outside the range exposed by
	// the selected PCM direction.
	ErrSubdeviceUnavailable = errors.New("alsa: PCM subdevice unavailable")
)

// DeviceError adds the operation and device selector to an ALSA error while
// preserving its cause for errors.Is and errors.As.
type DeviceError struct {
	Op     string
	Device string
	Err    error
}

func (e *DeviceError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Op == "" {
		return "alsa: " + e.Device + ": " + e.Err.Error()
	}
	return "alsa: " + e.Op + " " + e.Device + ": " + e.Err.Error()
}

func (e *DeviceError) Unwrap() error { return e.Err }

// Is classifies common device-node failures while Unwrap continues to expose
// the original errno. Callers can therefore test both ErrPermission and
// syscall.EACCES, for example.
func (e *DeviceError) Is(target error) bool {
	if e == nil || e.Err == nil {
		return false
	}
	switch target {
	case ErrPermission:
		return errors.Is(e.Err, fs.ErrPermission) || errors.Is(e.Err, syscall.EPERM)
	case ErrDeviceBusy:
		return errors.Is(e.Err, syscall.EBUSY)
	case ErrDeviceNotFound:
		return errors.Is(e.Err, fs.ErrNotExist) ||
			errors.Is(e.Err, syscall.ENODEV) || errors.Is(e.Err, syscall.ENXIO)
	}
	return false
}
