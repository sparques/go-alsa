//go:build linux && (amd64 || arm64)

package uapi

import "unsafe"

const (
	ControlProtocolVersion = 2<<16 | 0<<8 | 10
	controlIoctlType       = uintptr('U')
)

// ControlCardInfo matches struct snd_ctl_card_info from Linux UAPI
// sound/asound.h. The layout contains only fixed-width fields.
type ControlCardInfo struct {
	Card       int32
	Pad        int32
	ID         [16]byte
	Driver     [16]byte
	Name       [32]byte
	LongName   [80]byte
	Reserved   [16]byte
	MixerName  [80]byte
	Components [128]byte
}

const (
	ControlIoctlProtocolVersion = uintptr(0x80045500)
	ControlIoctlCardInfo        = uintptr(0x81785501)
)

// Compile-time size checks fail in either direction if the structure changes.
var (
	_ [376 - unsafe.Sizeof(ControlCardInfo{})]byte
	_ [unsafe.Sizeof(ControlCardInfo{}) - 376]byte
)
