package alsa

import "errors"

var (
	// ErrUnsupportedPlatform reports that the current operating system or
	// architecture has no native ALSA implementation.
	ErrUnsupportedPlatform = errors.New("alsa: unsupported platform")
	// ErrMalformedDevice reports a device node whose name does not follow the
	// Linux ALSA convention expected by the resolver.
	ErrMalformedDevice = errors.New("alsa: malformed device")
)
