//go:build linux && (amd64 || arm64)

package device

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/sparques/go-alsa/internal/uapi"
)

const (
	StreamPlayback = uapi.PCMStreamPlayback
	StreamCapture  = uapi.PCMStreamCapture
)

// ReadCardInfo opens a kernel ALSA control device and retrieves its card
// metadata. The descriptor is never retained beyond this call.
func ReadCardInfo(path string) (uapi.ControlCardInfo, error) {
	file, version, err := openControl(path)
	if err != nil {
		return uapi.ControlCardInfo{}, err
	}
	defer file.Close()
	_ = version

	var info uapi.ControlCardInfo
	if err := ioctl(file.Fd(), uapi.ControlIoctlCardInfo, unsafe.Pointer(&info)); err != nil {
		return uapi.ControlCardInfo{}, fmt.Errorf("query card info: %w", err)
	}
	return info, nil
}

// ReadPCMs enumerates both stream directions through a card control device.
func ReadPCMs(path string) ([]uapi.PCMInfo, error) {
	file, _, err := openControl(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	device := int32(-1)
	var result []uapi.PCMInfo
	for {
		if err := ioctl(file.Fd(), uapi.ControlIoctlPCMNextDevice, unsafe.Pointer(&device)); err != nil {
			return nil, fmt.Errorf("query next PCM device: %w", err)
		}
		if device < 0 {
			return result, nil
		}
		for _, stream := range [...]int32{uapi.PCMStreamPlayback, uapi.PCMStreamCapture} {
			info := uapi.PCMInfo{Device: uint32(device), Stream: stream}
			err := ioctl(file.Fd(), uapi.ControlIoctlPCMInfo, unsafe.Pointer(&info))
			if errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.ENOENT) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("query PCM %d stream %d: %w", device, stream, err)
			}
			result = append(result, info)
		}
	}
}

func openControl(path string) (*os.File, int32, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	var version int32
	if err := ioctl(file.Fd(), uapi.ControlIoctlProtocolVersion, unsafe.Pointer(&version)); err != nil {
		file.Close()
		return nil, 0, fmt.Errorf("query control protocol version: %w", err)
	}
	if version>>16 != uapi.ControlProtocolVersion>>16 {
		file.Close()
		return nil, 0, fmt.Errorf("unsupported control protocol %#x (supported major %#x)", version, uapi.ControlProtocolVersion>>16)
	}
	return file, version, nil
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
