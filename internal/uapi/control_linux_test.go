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

func TestControlElementLayoutsAndIoctls(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(ControlElementID{}); got != 64 {
		t.Fatalf("sizeof(ControlElementID) = %d; want 64", got)
	}
	if got := unsafe.Sizeof(ControlElementList{}); got != 80 {
		t.Fatalf("sizeof(ControlElementList) = %d; want 80", got)
	}
	if got := unsafe.Offsetof(ControlElementList{}.IDs); got != 16 {
		t.Fatalf("offsetof(IDs) = %d; want 16", got)
	}
	if got := unsafe.Sizeof(ControlElementInfo{}); got != 272 {
		t.Fatalf("sizeof(ControlElementInfo) = %d; want 272", got)
	}
	if got := unsafe.Offsetof(ControlElementInfo{}.Value); got != 80 {
		t.Fatalf("offsetof(Value) = %d; want 80", got)
	}
	if got := iowr(controlIoctlType, 0x10, unsafe.Sizeof(ControlElementList{})); got != ControlIoctlElementList {
		t.Fatalf("ELEM_LIST ioctl = %#x; want %#x", got, ControlIoctlElementList)
	}
	if got := iowr(controlIoctlType, 0x11, unsafe.Sizeof(ControlElementInfo{})); got != ControlIoctlElementInfo {
		t.Fatalf("ELEM_INFO ioctl = %#x; want %#x", got, ControlIoctlElementInfo)
	}
	if got := unsafe.Sizeof(ControlElementValue{}); got != 1224 {
		t.Fatalf("sizeof(ControlElementValue) = %d; want 1224", got)
	}
	if got := unsafe.Offsetof(ControlElementValue{}.Value); got != 72 {
		t.Fatalf("offsetof(ControlElementValue.Value) = %d; want 72", got)
	}
	if got := iowr(controlIoctlType, 0x12, unsafe.Sizeof(ControlElementValue{})); got != ControlIoctlElementRead {
		t.Fatalf("ELEM_READ ioctl = %#x; want %#x", got, ControlIoctlElementRead)
	}
	if got := iowr(controlIoctlType, 0x13, unsafe.Sizeof(ControlElementValue{})); got != ControlIoctlElementWrite {
		t.Fatalf("ELEM_WRITE ioctl = %#x; want %#x", got, ControlIoctlElementWrite)
	}
}
