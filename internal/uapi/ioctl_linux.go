//go:build linux

// Package uapi contains checked-in Go representations of the Linux ALSA UAPI.
// It is internal because these layouts are syscall payloads, not public API.
package uapi

const (
	ioctlNRBits   = 8
	ioctlTypeBits = 8
	ioctlSizeBits = 14

	ioctlNRShift   = 0
	ioctlTypeShift = ioctlNRShift + ioctlNRBits
	ioctlSizeShift = ioctlTypeShift + ioctlTypeBits
	ioctlDirShift  = ioctlSizeShift + ioctlSizeBits

	ioctlNone  = 0
	ioctlWrite = 1
	ioctlRead  = 2
)

func ioctl(direction, kind, number, size uintptr) uintptr {
	return direction<<ioctlDirShift |
		kind<<ioctlTypeShift |
		number<<ioctlNRShift |
		size<<ioctlSizeShift
}

func io(kind, number uintptr) uintptr { return ioctl(ioctlNone, kind, number, 0) }

func ior(kind, number, size uintptr) uintptr {
	return ioctl(ioctlRead, kind, number, size)
}

func iow(kind, number, size uintptr) uintptr {
	return ioctl(ioctlWrite, kind, number, size)
}

func iowr(kind, number, size uintptr) uintptr {
	return ioctl(ioctlRead|ioctlWrite, kind, number, size)
}
