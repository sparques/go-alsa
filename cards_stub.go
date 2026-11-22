//go:build !linux || (!amd64 && !arm64)

package alsa

// Cards is unavailable outside the currently supported Linux architectures.
func Cards() ([]Card, error) { return nil, ErrUnsupportedPlatform }

// PCMs is unavailable outside the currently supported Linux architectures.
func PCMs() ([]PCM, error) { return nil, ErrUnsupportedPlatform }
