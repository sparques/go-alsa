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

func TestPCMHandleIoctlEncoding(t *testing.T) {
	t.Parallel()
	if got := ior(pcmIoctlType, 0x00, unsafe.Sizeof(int32(0))); got != PCMIoctlProtocolVersion {
		t.Fatalf("PCM protocol ioctl = %#x; want %#x", got, PCMIoctlProtocolVersion)
	}
	if got := ior(pcmIoctlType, 0x01, unsafe.Sizeof(PCMInfo{})); got != PCMIoctlInfo {
		t.Fatalf("PCM info ioctl = %#x; want %#x", got, PCMIoctlInfo)
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

func TestPCMHWParamsLayoutAndIoctls(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(PCMHWParams{}); got != 608 {
		t.Fatalf("sizeof(PCMHWParams) = %d; want 608", got)
	}
	if got := unsafe.Offsetof(PCMHWParams{}.Intervals); got != 260 {
		t.Fatalf("offsetof(Intervals) = %d; want 260", got)
	}
	if got := unsafe.Offsetof(PCMHWParams{}.FIFOSize); got != 536 {
		t.Fatalf("offsetof(FIFOSize) = %d; want 536", got)
	}
	if got := iowr(pcmIoctlType, 0x10, unsafe.Sizeof(PCMHWParams{})); got != PCMIoctlHWRefine {
		t.Fatalf("HW_REFINE ioctl = %#x; want %#x", got, PCMIoctlHWRefine)
	}
	if got := iowr(pcmIoctlType, 0x11, unsafe.Sizeof(PCMHWParams{})); got != PCMIoctlHWParams {
		t.Fatalf("HW_PARAMS ioctl = %#x; want %#x", got, PCMIoctlHWParams)
	}
	if got := io(pcmIoctlType, 0x12); got != PCMIoctlHWFree {
		t.Fatalf("HW_FREE ioctl = %#x; want %#x", got, PCMIoctlHWFree)
	}
}

func TestPCMSWParamsLayoutAndIoctls(t *testing.T) {
	t.Parallel()
	if got := unsafe.Sizeof(PCMSWParams{}); got != 136 {
		t.Fatalf("sizeof(PCMSWParams) = %d; want 136", got)
	}
	if got := unsafe.Offsetof(PCMSWParams{}.AvailableMinimum); got != 16 {
		t.Fatalf("offsetof(AvailableMinimum) = %d; want 16", got)
	}
	if got := unsafe.Offsetof(PCMSWParams{}.Protocol); got != 72 {
		t.Fatalf("offsetof(Protocol) = %d; want 72", got)
	}
	if got := iowr(pcmIoctlType, 0x13, unsafe.Sizeof(PCMSWParams{})); got != PCMIoctlSWParams {
		t.Fatalf("SW_PARAMS ioctl = %#x; want %#x", got, PCMIoctlSWParams)
	}
	if got := io(pcmIoctlType, 0x40); got != PCMIoctlPrepare {
		t.Fatalf("PREPARE ioctl = %#x; want %#x", got, PCMIoctlPrepare)
	}
}
