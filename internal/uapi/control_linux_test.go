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

func TestControlCardBytesLayout(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(ControlCardBytes{}); got != 24 {
		t.Fatalf("sizeof(ControlCardBytes) = %d; want 24", got)
	}
	if got := unsafe.Offsetof(ControlCardBytes{}.Data); got != 16 {
		t.Fatalf("offsetof(ControlCardBytes.Data) = %d; want 16", got)
	}
	if got := iowr(controlIoctlType, 0x02, unsafe.Sizeof(ControlCardBytes{})); got != ControlIoctlCardBytes {
		t.Fatalf("CARD_BYTES ioctl = %#x; want %#x", got, ControlIoctlCardBytes)
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

func TestControlLockAndTLVIoctls(t *testing.T) {
	t.Parallel()
	if got := iow(controlIoctlType, 0x14, unsafe.Sizeof(ControlElementID{})); got != ControlIoctlElementLock {
		t.Fatalf("ELEM_LOCK ioctl = %#x; want %#x", got, ControlIoctlElementLock)
	}
	if got := iow(controlIoctlType, 0x15, unsafe.Sizeof(ControlElementID{})); got != ControlIoctlElementUnlock {
		t.Fatalf("ELEM_UNLOCK ioctl = %#x; want %#x", got, ControlIoctlElementUnlock)
	}
	for _, check := range []struct {
		number byte
		want   uintptr
	}{
		{0x1a, ControlIoctlTLVRead}, {0x1b, ControlIoctlTLVWrite}, {0x1c, ControlIoctlTLVCommand},
	} {
		if got := iowr(controlIoctlType, uintptr(check.number), 8); got != check.want {
			t.Errorf("TLV %#x ioctl = %#x; want %#x", check.number, got, check.want)
		}
	}
}

func TestControlUserElementIoctls(t *testing.T) {
	t.Parallel()
	if got := iowr(controlIoctlType, 0x17, unsafe.Sizeof(ControlElementInfo{})); got != ControlIoctlElementAdd {
		t.Fatalf("ELEM_ADD ioctl = %#x; want %#x", got, ControlIoctlElementAdd)
	}
	if got := iowr(controlIoctlType, 0x18, unsafe.Sizeof(ControlElementInfo{})); got != ControlIoctlElementReplace {
		t.Fatalf("ELEM_REPLACE ioctl = %#x; want %#x", got, ControlIoctlElementReplace)
	}
	if got := iowr(controlIoctlType, 0x19, unsafe.Sizeof(ControlElementID{})); got != ControlIoctlElementRemove {
		t.Fatalf("ELEM_REMOVE ioctl = %#x; want %#x", got, ControlIoctlElementRemove)
	}
}

func TestControlEventLayoutAndIoctl(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(ControlEvent{}); got != 72 {
		t.Fatalf("sizeof(ControlEvent) = %d; want 72", got)
	}
	if got := unsafe.Offsetof(ControlEvent{}.ID); got != 8 {
		t.Fatalf("offsetof(ControlEvent.ID) = %d; want 8", got)
	}
	if got := iowr(controlIoctlType, 0x16, unsafe.Sizeof(int32(0))); got != ControlIoctlSubscribeEvents {
		t.Fatalf("SUBSCRIBE_EVENTS ioctl = %#x; want %#x", got, ControlIoctlSubscribeEvents)
	}
}

func TestControlPowerIoctls(t *testing.T) {
	t.Parallel()
	if got := iowr(controlIoctlType, 0xd0, unsafe.Sizeof(int32(0))); got != ControlIoctlPower {
		t.Fatalf("POWER ioctl = %#x; want %#x", got, ControlIoctlPower)
	}
	if got := ior(controlIoctlType, 0xd1, unsafe.Sizeof(int32(0))); got != ControlIoctlPowerState {
		t.Fatalf("POWER_STATE ioctl = %#x; want %#x", got, ControlIoctlPowerState)
	}
}
