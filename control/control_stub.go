//go:build !linux || (!amd64 && !arm64)

package control

import alsa "github.com/sparques/go-alsa"

type handleCore struct{}

func Open(string) (*Handle, error)             { return nil, alsa.ErrUnsupportedPlatform }
func (*handleCore) close() error               { return nil }
func (*Handle) Elements() ([]ElementID, error) { return nil, alsa.ErrUnsupportedPlatform }
func (*Handle) Info(ElementID) (ElementInfo, error) {
	return ElementInfo{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) EnumeratedItem(ElementID, uint32) (ElementInfo, error) {
	return ElementInfo{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) Read(ElementID) (ElementValue, error) {
	return ElementValue{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) Write(ElementValue) error { return alsa.ErrUnsupportedPlatform }
