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

// ControlElementID matches struct snd_ctl_elem_id.
type ControlElementID struct {
	Number    uint32
	Interface int32
	Device    uint32
	Subdevice uint32
	Name      [44]byte
	Index     uint32
}

// ControlElementList matches struct snd_ctl_elem_list on supported 64-bit
// Linux architectures.
type ControlElementList struct {
	Offset   uint32
	Space    uint32
	Used     uint32
	Count    uint32
	IDs      uintptr
	Reserved [50]byte
	Pad      [6]byte
}

// ControlElementInfo matches struct snd_ctl_elem_info. Value is the 128-byte
// tagged union interpreted according to Type.
type ControlElementInfo struct {
	ID       ControlElementID
	Type     int32
	Access   uint32
	Count    uint32
	Owner    int32
	Value    [128]byte
	Reserved [64]byte
}

// ControlElementValue matches struct snd_ctl_elem_value. Value is the largest
// union arm: 128 native longs on supported 64-bit architectures.
type ControlElementValue struct {
	ID       ControlElementID
	Indirect uint32
	Pad      uint32
	Value    [1024]byte
	Reserved [128]byte
}

// ControlEvent is the element arm of struct snd_ctl_event.
type ControlEvent struct {
	Type int32
	Mask uint32
	ID   ControlElementID
}

const (
	ControlIoctlProtocolVersion = uintptr(0x80045500)
	ControlIoctlCardInfo        = uintptr(0x81785501)
	ControlIoctlElementList     = uintptr(0xc0505510)
	ControlIoctlElementInfo     = uintptr(0xc1105511)
	ControlIoctlElementRead     = uintptr(0xc4c85512)
	ControlIoctlElementWrite    = uintptr(0xc4c85513)
	ControlIoctlElementLock     = uintptr(0x40405514)
	ControlIoctlElementUnlock   = uintptr(0x40405515)
	ControlIoctlSubscribeEvents = uintptr(0xc0045516)
	ControlIoctlTLVRead         = uintptr(0xc008551a)
	ControlIoctlTLVWrite        = uintptr(0xc008551b)
	ControlIoctlTLVCommand      = uintptr(0xc008551c)
)

// Compile-time size checks fail in either direction if the structure changes.
var (
	_ [376 - unsafe.Sizeof(ControlCardInfo{})]byte
	_ [unsafe.Sizeof(ControlCardInfo{}) - 376]byte
	_ [64 - unsafe.Sizeof(ControlElementID{})]byte
	_ [unsafe.Sizeof(ControlElementID{}) - 64]byte
	_ [80 - unsafe.Sizeof(ControlElementList{})]byte
	_ [unsafe.Sizeof(ControlElementList{}) - 80]byte
	_ [272 - unsafe.Sizeof(ControlElementInfo{})]byte
	_ [unsafe.Sizeof(ControlElementInfo{}) - 272]byte
	_ [1224 - unsafe.Sizeof(ControlElementValue{})]byte
	_ [unsafe.Sizeof(ControlElementValue{}) - 1224]byte
	_ [72 - unsafe.Sizeof(ControlEvent{})]byte
	_ [unsafe.Sizeof(ControlEvent{}) - 72]byte
)
