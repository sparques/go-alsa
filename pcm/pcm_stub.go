//go:build !linux || (!amd64 && !arm64)

package pcm

import (
	"context"

	alsa "github.com/sparques/go-alsa"
)

type handleCore struct{}

func OpenHardware(string, alsa.Stream) (*Handle, error) {
	return nil, alsa.ErrUnsupportedPlatform
}

func (*handleCore) close() error { return nil }

func (*Handle) ConfigureHardware(HardwareConfig) (HardwareSetup, error) {
	return HardwareSetup{}, alsa.ErrUnsupportedPlatform
}

func (*Handle) ConfigureSoftware(SoftwareConfig) (SoftwareSetup, error) {
	return SoftwareSetup{}, alsa.ErrUnsupportedPlatform
}

func (*Handle) ConfigureSoftwareDefaults() (SoftwareSetup, error) {
	return SoftwareSetup{}, alsa.ErrUnsupportedPlatform
}

func (*Handle) Prepare() error              { return alsa.ErrUnsupportedPlatform }
func (*Handle) start() error                { return alsa.ErrUnsupportedPlatform }
func (*Handle) drop() error                 { return alsa.ErrUnsupportedPlatform }
func (*Handle) drain(context.Context) error { return alsa.ErrUnsupportedPlatform }
func (*Handle) freeHardware() error         { return alsa.ErrUnsupportedPlatform }
func (*Handle) status() (Status, error)     { return Status{}, alsa.ErrUnsupportedPlatform }
func (*Handle) delay() (int64, error)       { return 0, alsa.ErrUnsupportedPlatform }
func (*Handle) transferInterleaved(context.Context, []int16, bool) (int, error) {
	return 0, alsa.ErrUnsupportedPlatform
}
