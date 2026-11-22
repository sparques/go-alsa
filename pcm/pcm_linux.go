//go:build linux && (amd64 || arm64)

package pcm

import (
	"errors"
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
	direction := streamPlayback
	if stream == alsa.Capture {
		direction = streamCapture
	}
	return &Handle{
		core:   handleCore{fd: fd, protocol: protocol},
		stream: direction,
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

// ConfigureHardware applies exact hardware constraints and moves the stream
// from OPEN to SETUP. It may be called only once before hardware parameters are
// freed or the handle is closed.
func (h *Handle) ConfigureHardware(config HardwareConfig) (HardwareSetup, error) {
	if err := validateHardwareConfig(config); err != nil {
		return HardwareSetup{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	params := anyHardwareParams()
	setMask(&params, uapi.PCMHWParamAccess, uint32(config.Access))
	setMask(&params, uapi.PCMHWParamFormat, uint32(config.Format))
	setMask(&params, uapi.PCMHWParamSubformat, uapi.PCMSubformatStandard)
	setInterval(&params, uapi.PCMHWParamChannels, uint32(config.Channels))
	setInterval(&params, uapi.PCMHWParamRate, uint32(config.Rate))
	setInterval(&params, uapi.PCMHWParamPeriodSize, uint32(config.PeriodFrames))
	setInterval(&params, uapi.PCMHWParamBufferSize, uint32(config.BufferFrames))
	params.RequestMask = ^uint32(0)

	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWParams, unsafe.Pointer(&params)); err != nil {
		return HardwareSetup{}, fmt.Errorf("alsa pcm: configure hardware: %w", err)
	}
	setup := hardwareSetup(params)
	h.hardware = &setup
	h.software = nil
	h.prepared = false
	return setup, nil
}

// ConfigureSoftware applies wake, start, stop, and silence thresholds. Hardware
// parameters must have been configured first.
func (h *Handle) ConfigureSoftware(config SoftwareConfig) (SoftwareSetup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil {
		return SoftwareSetup{}, fmt.Errorf("%w: configure hardware before software", ErrState)
	}
	if err := validateSoftwareConfig(config, *h.hardware); err != nil {
		return SoftwareSetup{}, err
	}
	boundary := ringBoundary(uint64(h.hardware.BufferFrames))
	params := uapi.PCMSWParams{
		PeriodStep:       1,
		AvailableMinimum: uint64(config.AvailableMinimum),
		TransferAlign:    1,
		StartThreshold:   config.StartThreshold,
		StopThreshold:    config.StopThreshold,
		SilenceThreshold: config.SilenceThreshold,
		SilenceSize:      config.SilenceFrames,
		Boundary:         boundary,
		Protocol:         uint32(h.core.protocol),
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlSWParams, unsafe.Pointer(&params)); err != nil {
		return SoftwareSetup{}, fmt.Errorf("alsa pcm: configure software: %w", err)
	}
	setup := SoftwareSetup{SoftwareConfig: config, Boundary: params.Boundary}
	h.software = &setup
	h.prepared = false
	return setup, nil
}

// ConfigureSoftwareDefaults applies the package defaults for this stream.
func (h *Handle) ConfigureSoftwareDefaults() (SoftwareSetup, error) {
	h.mu.Lock()
	if h.hardware == nil {
		h.mu.Unlock()
		return SoftwareSetup{}, fmt.Errorf("%w: configure hardware before software", ErrState)
	}
	config := DefaultSoftwareConfig(h.stream == streamPlayback, *h.hardware)
	h.mu.Unlock()
	return h.ConfigureSoftware(config)
}

// Prepare moves a fully configured PCM into the PREPARED state.
func (h *Handle) Prepare() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil || h.software == nil {
		return fmt.Errorf("%w: configure hardware and software before prepare", ErrState)
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlPrepare, nil); err != nil {
		return fmt.Errorf("alsa pcm: prepare: %w", err)
	}
	h.prepared = true
	return nil
}

func validateSoftwareConfig(config SoftwareConfig, hardware HardwareSetup) error {
	if config.AvailableMinimum <= 0 || uint64(config.AvailableMinimum) > uint64(hardware.BufferFrames) {
		return errors.New("alsa pcm: available minimum must be within the hardware buffer")
	}
	boundary := ringBoundary(uint64(hardware.BufferFrames))
	for _, value := range [...]uint64{config.StartThreshold, config.StopThreshold, config.SilenceThreshold, config.SilenceFrames} {
		if value > boundary {
			return errors.New("alsa pcm: software threshold exceeds the ring boundary")
		}
	}
	return nil
}

func ringBoundary(bufferFrames uint64) uint64 {
	boundary := bufferFrames
	maximum := uint64(^uint(0) >> 1)
	for boundary <= (maximum-bufferFrames)/2 {
		boundary *= 2
	}
	return boundary
}

func validateHardwareConfig(config HardwareConfig) error {
	if config.Access != RWInterleaved {
		return fmt.Errorf("alsa pcm: unsupported access %d", config.Access)
	}
	if config.Format != S16LE {
		return fmt.Errorf("alsa pcm: unsupported format %d", config.Format)
	}
	if config.Channels <= 0 || config.Rate <= 0 || config.PeriodFrames <= 0 || config.BufferFrames <= 0 {
		return errors.New("alsa pcm: channels, rate, period frames, and buffer frames must be positive")
	}
	if config.BufferFrames < config.PeriodFrames {
		return errors.New("alsa pcm: buffer frames must not be smaller than period frames")
	}
	for _, value := range [...]int{config.Channels, config.Rate, config.PeriodFrames, config.BufferFrames} {
		if uint64(value) > uint64(^uint32(0)) {
			return errors.New("alsa pcm: hardware parameter exceeds the kernel 32-bit interval")
		}
	}
	return nil
}

func anyHardwareParams() uapi.PCMHWParams {
	var params uapi.PCMHWParams
	for index := range params.Masks {
		for word := range params.Masks[index].Bits {
			params.Masks[index].Bits[word] = ^uint32(0)
		}
	}
	for index := range params.Intervals {
		params.Intervals[index] = uapi.PCMInterval{Min: 0, Max: ^uint32(0)}
	}
	return params
}

func setMask(p *uapi.PCMHWParams, parameter int, value uint32) {
	index := parameter - uapi.PCMHWParamFirstMask
	for word := range p.Masks[index].Bits {
		p.Masks[index].Bits[word] = 0
	}
	p.Masks[index].Bits[value/32] = uint32(1) << (value % 32)
}

func setInterval(p *uapi.PCMHWParams, parameter int, value uint32) {
	index := parameter - uapi.PCMHWParamFirstInterval
	p.Intervals[index] = uapi.PCMInterval{Min: value, Max: value, Flags: uapi.PCMIntervalInteger}
}

func intervalValue(p uapi.PCMHWParams, parameter int) int {
	return int(p.Intervals[parameter-uapi.PCMHWParamFirstInterval].Min)
}

func hardwareSetup(params uapi.PCMHWParams) HardwareSetup {
	return HardwareSetup{
		HardwareConfig: HardwareConfig{
			Access:       Access(singleMaskValue(params, uapi.PCMHWParamAccess)),
			Format:       Format(singleMaskValue(params, uapi.PCMHWParamFormat)),
			Channels:     intervalValue(params, uapi.PCMHWParamChannels),
			Rate:         intervalValue(params, uapi.PCMHWParamRate),
			PeriodFrames: intervalValue(params, uapi.PCMHWParamPeriodSize),
			BufferFrames: intervalValue(params, uapi.PCMHWParamBufferSize),
		},
		SampleBits: intervalValue(params, uapi.PCMHWParamSampleBits),
		FrameBits:  intervalValue(params, uapi.PCMHWParamFrameBits),
		Periods:    intervalValue(params, uapi.PCMHWParamPeriods),
		MSBits:     int(params.MSBits),
		FIFOFrames: uint64(params.FIFOSize),
	}
}

func singleMaskValue(p uapi.PCMHWParams, parameter int) int {
	mask := p.Masks[parameter-uapi.PCMHWParamFirstMask]
	for wordIndex, word := range mask.Bits {
		for bit := 0; bit < 32; bit++ {
			if word&(uint32(1)<<bit) != 0 {
				return wordIndex*32 + bit
			}
		}
	}
	return -1
}

func cString(value []byte) string {
	for index, character := range value {
		if character == 0 {
			return string(value[:index])
		}
	}
	return string(value)
}
