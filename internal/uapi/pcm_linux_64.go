//go:build linux && (amd64 || arm64)

package uapi

import "unsafe"

const (
	PCMProtocolVersion = 2<<16 | 0<<8 | 18

	PCMStreamPlayback = int32(0)
	PCMStreamCapture  = int32(1)
)

// PCMInfo matches struct snd_pcm_info from Linux UAPI sound/asound.h.
type PCMInfo struct {
	Device              uint32
	Subdevice           uint32
	Stream              int32
	Card                int32
	ID                  [64]byte
	Name                [80]byte
	Subname             [32]byte
	DeviceClass         int32
	DeviceSubclass      int32
	SubdevicesCount     uint32
	SubdevicesAvailable uint32
	Pad                 [16]byte
	Reserved            [64]byte
}

const (
	ControlIoctlPCMNextDevice = uintptr(0x80045530)
	ControlIoctlPCMInfo       = uintptr(0xc1205531)
)

var (
	_ [288 - unsafe.Sizeof(PCMInfo{})]byte
	_ [unsafe.Sizeof(PCMInfo{}) - 288]byte
)
