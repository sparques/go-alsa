//go:build linux && (amd64 || arm64)

package device

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/sparques/go-alsa/internal/uapi"
)

// ReadCardInfo opens a kernel ALSA control device and retrieves its card
// metadata. The descriptor is never retained beyond this call.
func ReadCardInfo(path string) (uapi.ControlCardInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return uapi.ControlCardInfo{}, err
	}
	defer file.Close()

	var version int32
	if err := ioctl(file.Fd(), uapi.ControlIoctlProtocolVersion, unsafe.Pointer(&version)); err != nil {
		return uapi.ControlCardInfo{}, fmt.Errorf("query control protocol version: %w", err)
	}
	if version>>16 != uapi.ControlProtocolVersion>>16 {
		return uapi.ControlCardInfo{}, fmt.Errorf("unsupported control protocol %#x (supported major %#x)", version, uapi.ControlProtocolVersion>>16)
	}

	var info uapi.ControlCardInfo
	if err := ioctl(file.Fd(), uapi.ControlIoctlCardInfo, unsafe.Pointer(&info)); err != nil {
		return uapi.ControlCardInfo{}, fmt.Errorf("query card info: %w", err)
	}
	return info, nil
}

func ioctl(fd, request uintptr, argument unsafe.Pointer) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(argument))
		if errno == 0 {
			return nil
		}
		if errno != syscall.EINTR {
			return errno
		}
	}
}
