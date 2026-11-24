//go:build !linux || (!amd64 && !arm64)

package control

import (
	"context"

	alsa "github.com/sparques/go-alsa"
)

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
func (*Handle) Write(ElementValue) error                 { return alsa.ErrUnsupportedPlatform }
func (*Handle) Lock(ElementID) error                     { return alsa.ErrUnsupportedPlatform }
func (*Handle) Unlock(ElementID) error                   { return alsa.ErrUnsupportedPlatform }
func (*Handle) ReadTLV(ElementID, int) ([]uint32, error) { return nil, alsa.ErrUnsupportedPlatform }
func (*Handle) ReadDecodedTLV(ElementID, int) (TLV, error) {
	return TLV{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) WriteTLV(ElementID, []uint32) error       { return alsa.ErrUnsupportedPlatform }
func (*Handle) CommandTLV(ElementID, []uint32) error     { return alsa.ErrUnsupportedPlatform }
func (*Handle) subscribe(bool) error                     { return alsa.ErrUnsupportedPlatform }
func (*Handle) nextEvent(context.Context) (Event, error) { return Event{}, alsa.ErrUnsupportedPlatform }
func (*Handle) powerState() (PowerState, error)          { return 0, alsa.ErrUnsupportedPlatform }
func (*Handle) setPowerState(PowerState) error           { return alsa.ErrUnsupportedPlatform }
func (*Handle) addOrReplaceUserElement(UserElementSpec, bool) (ElementID, error) {
	return ElementID{}, alsa.ErrUnsupportedPlatform
}
func (*Handle) removeUserElement(ElementID) error { return alsa.ErrUnsupportedPlatform }
