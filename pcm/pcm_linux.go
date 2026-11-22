//go:build linux && (amd64 || arm64)

package pcm

import (
	"fmt"
	"syscall"
	"unsafe"

	alsa "github.com/sparques/go-alsa"
	"github.com/sparques/go-alsa/internal/sysio"
	"github.com/sparques/go-alsa/internal/uapi"
)

type handleCore struct {
	fd       *sysio.FD
	protocol int32
}

// OpenHardware resolves and opens a direct kernel PCM in nonblocking mode.
// Hardware parameters are not configured yet; the returned handle is in the
// kernel OPEN state.
func OpenHardware(name string, stream alsa.Stream) (*Handle, error) {
	resolved, err := alsa.ResolveHardwarePCM(name, stream)
	if err != nil {
		return nil, err
	}
	fd, err := sysio.Open(resolved.PCM.Path, syscall.O_RDWR|syscall.O_NONBLOCK)
	if err != nil {
		return nil, &alsa.DeviceError{Op: "open", Device: name, Err: err}
	}
	failed := true
	defer func() {
		if failed {
			fd.Close()
		}
	}()

	var protocol int32
	if err := fd.Ioctl(uapi.PCMIoctlProtocolVersion, unsafe.Pointer(&protocol)); err != nil {
		return nil, &alsa.DeviceError{Op: "query PCM protocol for", Device: name, Err: err}
	}
	if protocol>>16 != uapi.PCMProtocolVersion>>16 {
		return nil, &alsa.DeviceError{Op: "query PCM protocol for", Device: name, Err: fmt.Errorf("%w: kernel %#x, supported major %d", ErrProtocol, protocol, uapi.PCMProtocolVersion>>16)}
	}

	info := uapi.PCMInfo{Subdevice: uint32(resolved.Address.Subdevice)}
	if err := fd.Ioctl(uapi.PCMIoctlInfo, unsafe.Pointer(&info)); err != nil {
		return nil, &alsa.DeviceError{Op: "query opened PCM info for", Device: name, Err: err}
	}
	wantStream := uapi.PCMStreamPlayback
	if stream == alsa.Capture {
		wantStream = uapi.PCMStreamCapture
	}
	if info.Stream != wantStream {
		return nil, &alsa.DeviceError{Op: "open", Device: name, Err: ErrDirection}
	}

	failed = false
	return &Handle{
		core: handleCore{fd: fd, protocol: protocol},
		info: Info{
			Card:                int(info.Card),
			Device:              int(info.Device),
			Subdevice:           int(info.Subdevice),
			ID:                  cString(info.ID[:]),
			Name:                cString(info.Name[:]),
			SubdeviceName:       cString(info.Subname[:]),
			Subdevices:          int(info.SubdevicesCount),
			AvailableSubdevices: int(info.SubdevicesAvailable),
		},
	}, nil
}

func (h *handleCore) close() error {
	if h.fd == nil {
		return nil
	}
	return h.fd.Close()
}

func cString(value []byte) string {
	for index, character := range value {
		if character == 0 {
			return string(value[:index])
		}
	}
	return string(value)
}
