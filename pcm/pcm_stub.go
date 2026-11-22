//go:build !linux || (!amd64 && !arm64)

package pcm

import alsa "github.com/sparques/go-alsa"

type handleCore struct{}

func OpenHardware(string, alsa.Stream) (*Handle, error) {
	return nil, alsa.ErrUnsupportedPlatform
}

func (*handleCore) close() error { return nil }
