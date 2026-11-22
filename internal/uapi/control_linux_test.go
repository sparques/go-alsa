//go:build linux && (amd64 || arm64)

package uapi

import (
	"testing"
	"unsafe"
)

func TestControlIoctlEncoding(t *testing.T) {
	t.Parallel()

	if got := ior(controlIoctlType, 0x00, unsafe.Sizeof(int32(0))); got != ControlIoctlProtocolVersion {
		t.Fatalf("protocol ioctl = %#x; want %#x", got, ControlIoctlProtocolVersion)
	}
	if got := ior(controlIoctlType, 0x01, unsafe.Sizeof(ControlCardInfo{})); got != ControlIoctlCardInfo {
		t.Fatalf("card info ioctl = %#x; want %#x", got, ControlIoctlCardInfo)
	}
}

func TestControlCardInfoLayout(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(ControlCardInfo{}); got != 376 {
		t.Fatalf("sizeof(ControlCardInfo) = %d; want 376", got)
	}
	if got := unsafe.Offsetof(ControlCardInfo{}.Components); got != 248 {
		t.Fatalf("offsetof(Components) = %d; want 248", got)
	}
}
