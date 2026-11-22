//go:build linux && (amd64 || arm64)

package uapi

import (
	"testing"
	"unsafe"
)

func TestPCMDiscoveryIoctlEncoding(t *testing.T) {
	t.Parallel()
	if got := ior(controlIoctlType, 0x30, unsafe.Sizeof(int32(0))); got != ControlIoctlPCMNextDevice {
		t.Fatalf("next PCM ioctl = %#x; want %#x", got, ControlIoctlPCMNextDevice)
	}
	if got := iowr(controlIoctlType, 0x31, unsafe.Sizeof(PCMInfo{})); got != ControlIoctlPCMInfo {
		t.Fatalf("PCM info ioctl = %#x; want %#x", got, ControlIoctlPCMInfo)
	}
}

func TestPCMInfoLayout(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(PCMInfo{}); got != 288 {
		t.Fatalf("sizeof(PCMInfo) = %d; want 288", got)
	}
	if got := unsafe.Offsetof(PCMInfo{}.SubdevicesCount); got != 200 {
		t.Fatalf("offsetof(SubdevicesCount) = %d; want 200", got)
	}
}
