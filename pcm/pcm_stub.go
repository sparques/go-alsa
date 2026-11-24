//go:build !linux || (!amd64 && !arm64)

package pcm

import (
	"context"

	alsa "github.com/sparques/go-alsa"
)

type handleCore struct{ protocol uint32 }

func OpenHardware(string, alsa.Stream) (*Handle, error) {
	return nil, alsa.ErrUnsupportedPlatform
}

func (*handleCore) close() error { return nil }

func (*Handle) ConfigureHardware(HardwareConfig) (HardwareSetup, error) {
	return HardwareSetup{}, alsa.ErrUnsupportedPlatform
}

func (*Handle) configureHardwareNear(HardwareConfig) (HardwareSetup, error) {
	return HardwareSetup{}, alsa.ErrUnsupportedPlatform
}

func (*Handle) capabilities() (Capabilities, error) {
	return Capabilities{}, alsa.ErrUnsupportedPlatform
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
func (*Handle) available() (uint64, error)  { return 0, alsa.ErrUnsupportedPlatform }
func (*Handle) hardwareTimestamp() (uint64, Timestamp, error) {
	return 0, Timestamp{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) wait(context.Context) error     { return alsa.ErrUnsupportedPlatform }
func (*Handle) reset() error                   { return alsa.ErrUnsupportedPlatform }
func (*Handle) pause(bool) error               { return alsa.ErrUnsupportedPlatform }
func (*Handle) resume() error                  { return alsa.ErrUnsupportedPlatform }
func (*Handle) rewind(uint64) (uint64, error)  { return 0, alsa.ErrUnsupportedPlatform }
func (*Handle) forward(uint64) (uint64, error) { return 0, alsa.ErrUnsupportedPlatform }
func (*Handle) syncHardware() error            { return alsa.ErrUnsupportedPlatform }
func (*Handle) link(*Handle) error             { return alsa.ErrUnsupportedPlatform }
func (*Handle) unlink() error                  { return alsa.ErrUnsupportedPlatform }
func (*Handle) transferInterleaved(context.Context, []int16, bool) (int, error) {
	return 0, alsa.ErrUnsupportedPlatform
}
func (*Handle) transferInterleavedBytes(context.Context, []byte, bool) (int, error) {
	return 0, alsa.ErrUnsupportedPlatform
}
func (*Handle) transferNonInterleavedBytes(context.Context, [][]byte, bool) (int, error) {
	return 0, alsa.ErrUnsupportedPlatform
}
