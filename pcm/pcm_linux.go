//go:build linux && (amd64 || arm64)

package pcm

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	alsa "github.com/sparques/go-alsa"
	"github.com/sparques/go-alsa/internal/sysio"
	"github.com/sparques/go-alsa/internal/uapi"
	"golang.org/x/sys/unix"
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
		return HardwareSetup{}, operationError("configure hardware", err)
	}
	setup := hardwareSetup(params)
	h.hardware = &setup
	h.software = nil
	h.prepared = false
	return setup, nil
}

func (h *Handle) configureHardwareNear(config HardwareConfig) (HardwareSetup, error) {
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
	params.RequestMask = ^uint32(0)
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWRefine, unsafe.Pointer(&params)); err != nil {
		return HardwareSetup{}, operationError("refine fixed hardware parameters", err)
	}
	requests := [...]struct {
		parameter int
		value     uint32
		name      string
	}{
		{uapi.PCMHWParamRate, uint32(config.Rate), "rate"},
		{uapi.PCMHWParamPeriodSize, uint32(config.PeriodFrames), "period size"},
		{uapi.PCMHWParamBufferSize, uint32(config.BufferFrames), "buffer size"},
	}
	for _, request := range requests {
		if err := h.refineNear(&params, request.parameter, request.value); err != nil {
			return HardwareSetup{}, operationError(fmt.Sprintf("negotiate %s near %d", request.name, request.value), err)
		}
	}
	params.RequestMask = ^uint32(0)
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWParams, unsafe.Pointer(&params)); err != nil {
		return HardwareSetup{}, operationError("configure negotiated hardware", err)
	}
	setup := hardwareSetup(params)
	h.hardware = &setup
	h.software = nil
	h.prepared = false
	return setup, nil
}

type nearCandidate struct {
	params uapi.PCMHWParams
	value  uint32
	err    error
}

func (h *Handle) refineNear(params *uapi.PCMHWParams, parameter int, target uint32) error {
	base := params.Intervals[parameter-uapi.PCMHWParamFirstInterval]
	minimum, maximum, ok := integerBounds(base)
	if !ok {
		return errors.New("hardware parameter has no available values")
	}
	lower := nearCandidate{params: *params}
	if target < minimum {
		lower.err = syscall.EINVAL
	} else {
		setIntervalRange(&lower.params, parameter, minimum, min(target, maximum))
		lower.params.RequestMask = ^uint32(0)
		lower.err = h.core.fd.Ioctl(uapi.PCMIoctlHWRefine, unsafe.Pointer(&lower.params))
		if lower.err == nil {
			_, lower.value, ok = integerBounds(lower.params.Intervals[parameter-uapi.PCMHWParamFirstInterval])
			if !ok {
				lower.err = syscall.EINVAL
			}
		}
	}
	upper := nearCandidate{params: *params}
	if target > maximum {
		upper.err = syscall.EINVAL
	} else {
		setIntervalRange(&upper.params, parameter, max(target, minimum), maximum)
		upper.params.RequestMask = ^uint32(0)
		upper.err = h.core.fd.Ioctl(uapi.PCMIoctlHWRefine, unsafe.Pointer(&upper.params))
		if upper.err == nil {
			upper.value, _, ok = integerBounds(upper.params.Intervals[parameter-uapi.PCMHWParamFirstInterval])
			if !ok {
				upper.err = syscall.EINVAL
			}
		}
	}
	chosen, err := nearestCandidate(target, lower, upper)
	if err != nil {
		return err
	}
	*params = chosen.params
	setInterval(params, parameter, chosen.value)
	params.RequestMask = ^uint32(0)
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWRefine, unsafe.Pointer(params)); err != nil {
		return err
	}
	return nil
}

func nearestCandidate(target uint32, lower, upper nearCandidate) (nearCandidate, error) {
	if lower.err != nil && upper.err != nil {
		return nearCandidate{}, fmt.Errorf("no value near %d: lower: %v; upper: %w", target, lower.err, upper.err)
	}
	if lower.err != nil {
		return upper, nil
	}
	if upper.err != nil {
		return lower, nil
	}
	lowerDistance := target - lower.value
	upperDistance := upper.value - target
	if lowerDistance <= upperDistance {
		return lower, nil
	}
	return upper, nil
}

func integerBounds(interval uapi.PCMInterval) (uint32, uint32, bool) {
	if interval.Flags&uapi.PCMIntervalEmpty != 0 {
		return 0, 0, false
	}
	minimum, maximum := interval.Min, interval.Max
	if interval.Flags&uapi.PCMIntervalOpenMin != 0 {
		if minimum == ^uint32(0) {
			return 0, 0, false
		}
		minimum++
	}
	if interval.Flags&uapi.PCMIntervalOpenMax != 0 {
		if maximum == 0 {
			return 0, 0, false
		}
		maximum--
	}
	return minimum, maximum, minimum <= maximum
}

// ConfigureSoftware applies wake, start, stop, silence, and timestamp policy.
// Hardware parameters must have been configured first.
func (h *Handle) ConfigureSoftware(config SoftwareConfig) (SoftwareSetup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil {
		return SoftwareSetup{}, fmt.Errorf("%w: configure hardware before software", ErrState)
	}
	if err := validateSoftwareConfig(config, *h.hardware); err != nil {
		return SoftwareSetup{}, err
	}
	params := softwareParams(config, *h.hardware, uint32(h.core.protocol))
	if err := h.core.fd.Ioctl(uapi.PCMIoctlSWParams, unsafe.Pointer(&params)); err != nil {
		return SoftwareSetup{}, operationError("configure software", err)
	}
	accepted := SoftwareConfig{
		AvailableMinimum: int(params.AvailableMinimum),
		StartThreshold:   params.StartThreshold,
		StopThreshold:    params.StopThreshold,
		SilenceThreshold: params.SilenceThreshold,
		SilenceFrames:    params.SilenceSize,
		TimestampMode:    TimestampMode(params.TimestampMode),
		TimestampClock:   TimestampClock(params.TimestampType),
	}
	setup := SoftwareSetup{SoftwareConfig: accepted, Boundary: params.Boundary}
	h.software = &setup
	h.prepared = false
	return setup, nil
}

func softwareParams(config SoftwareConfig, hardware HardwareSetup, protocol uint32) uapi.PCMSWParams {
	return uapi.PCMSWParams{
		TimestampMode:    int32(config.TimestampMode),
		PeriodStep:       1,
		AvailableMinimum: uint64(config.AvailableMinimum),
		TransferAlign:    1,
		StartThreshold:   config.StartThreshold,
		StopThreshold:    config.StopThreshold,
		SilenceThreshold: config.SilenceThreshold,
		SilenceSize:      config.SilenceFrames,
		Boundary:         ringBoundary(uint64(hardware.BufferFrames)),
		Protocol:         protocol,
		TimestampType:    uint32(config.TimestampClock),
	}
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
		return operationError("prepare", err)
	}
	h.prepared = true
	return nil
}

func (h *Handle) start() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.prepared {
		return fmt.Errorf("%w: prepare before start", ErrState)
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlStart, nil); err != nil {
		return operationError("start", err)
	}
	return nil
}

func (h *Handle) drop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil {
		return nil
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlDrop, nil); err != nil {
		return operationError("drop", err)
	}
	h.prepared = false
	return nil
}

func (h *Handle) status() (Status, error) {
	var raw uapi.PCMStatus
	if err := h.core.fd.Ioctl(uapi.PCMIoctlStatus, unsafe.Pointer(&raw)); err != nil {
		return Status{}, operationError("status", err)
	}
	return statusFromUAPI(raw), nil
}

func (h *Handle) delay() (int64, error) {
	var frames int64
	if err := h.core.fd.Ioctl(uapi.PCMIoctlDelay, unsafe.Pointer(&frames)); err != nil {
		return 0, operationError("delay", err)
	}
	return frames, nil
}

func (h *Handle) available() (uint64, error) {
	status, err := h.status()
	if err != nil {
		return 0, err
	}
	return status.AvailableFrames, nil
}

func (h *Handle) hardwareTimestamp() (uint64, Timestamp, error) {
	status, err := h.status()
	if err != nil {
		return 0, Timestamp{}, err
	}
	return status.AvailableFrames, status.Timestamp, nil
}

func (h *Handle) wait(ctx context.Context) error {
	h.mu.Lock()
	if h.hardware == nil {
		h.mu.Unlock()
		return fmt.Errorf("%w: configure hardware before waiting", ErrState)
	}
	events := int16(unix.POLLOUT)
	if h.stream == streamCapture {
		events = unix.POLLIN
	}
	h.mu.Unlock()

	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	revents, err := h.core.fd.Wait(ctx, events)
	if err != nil {
		return err
	}
	if err := h.pollError(revents); err != nil {
		return err
	}
	if revents&events == 0 {
		return fmt.Errorf("alsa pcm: poll returned unexpected events %#x", revents)
	}
	return nil
}

func (h *Handle) pollError(revents int16) error {
	if revents&(unix.POLLHUP|unix.POLLNVAL) != 0 {
		return ErrDisconnected
	}
	if revents&unix.POLLERR == 0 {
		return nil
	}
	status, err := h.status()
	if err != nil {
		return operationError("poll error", err)
	}
	return pollStateError(status.State, revents)
}

func pollStateError(state State, revents int16) error {
	switch state {
	case StateXRUN:
		return fmt.Errorf("%w: %w", ErrXRUN, syscall.EPIPE)
	case StateSuspended:
		return fmt.Errorf("%w: %w", ErrSuspended, syscall.ESTRPIPE)
	case StateDisconnected:
		return ErrDisconnected
	default:
		return fmt.Errorf("alsa pcm: poll reported stream error %#x in state %s", revents, state)
	}
}

func statusFromUAPI(raw uapi.PCMStatus) Status {
	return Status{
		State:                  State(raw.State),
		TriggerTimestamp:       Timestamp{Seconds: raw.TriggerTimestamp.Seconds, Nanoseconds: raw.TriggerTimestamp.Nanoseconds},
		Timestamp:              Timestamp{Seconds: raw.Timestamp.Seconds, Nanoseconds: raw.Timestamp.Nanoseconds},
		ApplicationPointer:     raw.ApplicationPointer,
		HardwarePointer:        raw.HardwarePointer,
		DelayFrames:            raw.Delay,
		AvailableFrames:        raw.Available,
		MaximumAvailableFrames: raw.MaximumAvailable,
		OverrangeCount:         raw.Overrange,
		SuspendedState:         State(raw.SuspendedState),
		AudioTimestamp:         Timestamp{Seconds: raw.AudioTimestamp.Seconds, Nanoseconds: raw.AudioTimestamp.Nanoseconds},
		DriverTimestamp:        Timestamp{Seconds: raw.DriverTimestamp.Seconds, Nanoseconds: raw.DriverTimestamp.Nanoseconds},
		AudioTimestampAccuracy: raw.AudioTimestampAccuracy,
	}
}

func (h *Handle) drain(ctx context.Context) error {
	h.mu.Lock()
	if h.stream != streamPlayback {
		h.mu.Unlock()
		return ErrDirection
	}
	if !h.prepared {
		h.mu.Unlock()
		return fmt.Errorf("%w: prepare before drain", ErrState)
	}
	h.mu.Unlock()

	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := h.core.fd.Ioctl(uapi.PCMIoctlDrain, nil)
		if err == nil {
			h.markUnprepared()
			return nil
		}
		if retry, recoveryErr := h.recoverTransfer(ctx, err, false); retry || recoveryErr != nil {
			if recoveryErr != nil {
				return recoveryErr
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) {
			return operationError("drain", err)
		}

		status, err := h.status()
		if err != nil {
			return err
		}
		switch status.State {
		case StateSetup:
			h.markUnprepared()
			return nil
		case StateXRUN, StateSuspended:
			stateErr := pollStateError(status.State, unix.POLLERR)
			retry, recoveryErr := h.recoverTransfer(ctx, stateErr, false)
			if recoveryErr != nil {
				return recoveryErr
			}
			if retry {
				continue
			}
			return stateErr
		case StateDisconnected:
			h.markUnprepared()
			return ErrDisconnected
		case StateDraining, StateRunning:
		default:
			return fmt.Errorf("%w: cannot drain state %s", ErrState, status.State)
		}

		revents, err := h.core.fd.Wait(ctx, unix.POLLOUT)
		if err != nil {
			return err
		}
		if pollErr := h.pollError(revents); pollErr != nil {
			retry, recoveryErr := h.recoverTransfer(ctx, pollErr, false)
			if recoveryErr != nil {
				return recoveryErr
			}
			if !retry {
				return pollErr
			}
		}
	}
}

func (h *Handle) markUnprepared() {
	h.mu.Lock()
	h.prepared = false
	h.mu.Unlock()
}

func (h *Handle) freeHardware() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil {
		return nil
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWFree, nil); err != nil {
		return operationError("free hardware parameters", err)
	}
	h.hardware = nil
	h.software = nil
	h.prepared = false
	return nil
}

func (h *Handle) transferInterleaved(ctx context.Context, samples []int16, read bool) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	h.mu.Lock()
	if h.hardware == nil {
		h.mu.Unlock()
		return 0, fmt.Errorf("%w: configure hardware before transfer", ErrState)
	}
	if h.hardware.Format != S16LE {
		h.mu.Unlock()
		return 0, errors.New("alsa pcm: []int16 transfer requires S16_LE hardware format")
	}
	h.mu.Unlock()
	frames, err := h.transferBuffer(ctx, uintptr(unsafe.Pointer(unsafe.SliceData(samples))), len(samples)*2, read)
	runtime.KeepAlive(samples)
	return frames, err
}

func (h *Handle) transferInterleavedBytes(ctx context.Context, buffer []byte, read bool) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	frames, err := h.transferBuffer(ctx, uintptr(unsafe.Pointer(unsafe.SliceData(buffer))), len(buffer), read)
	runtime.KeepAlive(buffer)
	return frames, err
}

func (h *Handle) transferBuffer(ctx context.Context, buffer uintptr, byteLength int, read bool) (int, error) {
	h.mu.Lock()
	if !h.prepared || h.hardware == nil {
		h.mu.Unlock()
		return 0, fmt.Errorf("%w: prepare before transfer", ErrState)
	}
	channels := h.hardware.Channels
	bytesPerSample, supportedFormat := h.hardware.Format.PhysicalBytes()
	access := h.hardware.Access
	stream := h.stream
	h.mu.Unlock()
	if !supportedFormat {
		return 0, errors.New("alsa pcm: configured format has no byte-transfer representation")
	}
	if access != RWInterleaved {
		return 0, errors.New("alsa pcm: interleaved transfer requires RW_INTERLEAVED access")
	}
	frameBytes := channels * bytesPerSample
	if byteLength%frameBytes != 0 {
		return 0, errors.New("alsa pcm: byte buffer does not contain whole frames")
	}
	if read && stream != streamCapture || !read && stream != streamPlayback {
		return 0, ErrDirection
	}

	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	request := uapi.PCMIoctlWriteInterleaved
	events := int16(unix.POLLOUT)
	if read {
		request = uapi.PCMIoctlReadInterleaved
		events = unix.POLLIN
	}
	transfer := uapi.PCMXferI{
		Buffer: buffer,
		Frames: uint64(byteLength / frameBytes),
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		err := h.core.fd.Ioctl(request, unsafe.Pointer(&transfer))
		if err == nil {
			if transfer.Result < 0 || uint64(transfer.Result) > transfer.Frames {
				return 0, fmt.Errorf("alsa pcm: kernel returned invalid frame count %d", transfer.Result)
			}
			return int(transfer.Result), nil
		}
		if retry, recoveryErr := h.recoverTransfer(ctx, err, read); retry || recoveryErr != nil {
			if recoveryErr != nil {
				return 0, recoveryErr
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) {
			return 0, operationError("transfer", err)
		}
		revents, err := h.core.fd.Wait(ctx, events)
		if err != nil {
			return 0, err
		}
		if pollErr := h.pollError(revents); pollErr != nil {
			retry, recoveryErr := h.recoverTransfer(ctx, pollErr, read)
			if recoveryErr != nil {
				return 0, recoveryErr
			}
			if !retry {
				return 0, pollErr
			}
		}
	}
}

func (h *Handle) transferNonInterleavedBytes(ctx context.Context, channels [][]byte, read bool) (int, error) {
	if len(channels) == 0 {
		return 0, errors.New("alsa pcm: non-interleaved transfer needs channel buffers")
	}
	h.mu.Lock()
	if !h.prepared || h.hardware == nil {
		h.mu.Unlock()
		return 0, fmt.Errorf("%w: prepare before transfer", ErrState)
	}
	wantChannels := h.hardware.Channels
	bytesPerSample, supportedFormat := h.hardware.Format.PhysicalBytes()
	access := h.hardware.Access
	stream := h.stream
	h.mu.Unlock()
	if access != RWNonInterleaved {
		return 0, errors.New("alsa pcm: non-interleaved transfer requires RW_NONINTERLEAVED access")
	}
	if !supportedFormat {
		return 0, errors.New("alsa pcm: configured format has no byte-transfer representation")
	}
	if len(channels) != wantChannels {
		return 0, fmt.Errorf("alsa pcm: got %d channel buffers, need %d", len(channels), wantChannels)
	}
	byteLength := len(channels[0])
	if byteLength == 0 {
		return 0, nil
	}
	if byteLength%bytesPerSample != 0 {
		return 0, errors.New("alsa pcm: channel buffer does not contain whole samples")
	}
	pointers := make([]uintptr, len(channels))
	for index, channel := range channels {
		if len(channel) != byteLength {
			return 0, errors.New("alsa pcm: non-interleaved channel buffers have different lengths")
		}
		pointers[index] = uintptr(unsafe.Pointer(unsafe.SliceData(channel)))
	}
	if read && stream != streamCapture || !read && stream != streamPlayback {
		return 0, ErrDirection
	}

	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	request := uapi.PCMIoctlWriteNonInterleaved
	events := int16(unix.POLLOUT)
	if read {
		request = uapi.PCMIoctlReadNonInterleaved
		events = unix.POLLIN
	}
	transfer := uapi.PCMXferN{
		Buffers: uintptr(unsafe.Pointer(unsafe.SliceData(pointers))),
		Frames:  uint64(byteLength / bytesPerSample),
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		err := h.core.fd.Ioctl(request, unsafe.Pointer(&transfer))
		runtime.KeepAlive(channels)
		runtime.KeepAlive(pointers)
		if err == nil {
			if transfer.Result < 0 || uint64(transfer.Result) > transfer.Frames {
				return 0, fmt.Errorf("alsa pcm: kernel returned invalid frame count %d", transfer.Result)
			}
			return int(transfer.Result), nil
		}
		if retry, recoveryErr := h.recoverTransfer(ctx, err, read); retry || recoveryErr != nil {
			if recoveryErr != nil {
				return 0, recoveryErr
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) {
			return 0, operationError("transfer", err)
		}
		revents, err := h.core.fd.Wait(ctx, events)
		if err != nil {
			return 0, err
		}
		if pollErr := h.pollError(revents); pollErr != nil {
			retry, recoveryErr := h.recoverTransfer(ctx, pollErr, read)
			if recoveryErr != nil {
				return 0, recoveryErr
			}
			if !retry {
				return 0, pollErr
			}
		}
	}
}

func (h *Handle) recoverXRUN(capture bool) error {
	return h.recoverPrepare(capture, "xrun")
}

func (h *Handle) recoverTransfer(ctx context.Context, transferErr error, capture bool) (bool, error) {
	if !errors.Is(transferErr, syscall.EPIPE) && !errors.Is(transferErr, syscall.ESTRPIPE) {
		return false, nil
	}
	h.mu.Lock()
	policy := h.recovery
	h.prepared = false
	if errors.Is(transferErr, syscall.EPIPE) {
		if capture {
			h.overruns++
		} else {
			h.underruns++
		}
	} else {
		h.suspends++
	}
	h.mu.Unlock()

	if errors.Is(transferErr, syscall.EPIPE) {
		if policy == RecoveryStrict {
			return false, fmt.Errorf("%w: %w", ErrXRUN, transferErr)
		}
		return true, h.recoverXRUN(capture)
	}
	if policy == RecoveryStrict {
		return false, fmt.Errorf("%w: %w", ErrSuspended, transferErr)
	}
	return true, h.recoverSuspend(ctx, capture)
}

func (h *Handle) recoverSuspend(ctx context.Context, capture bool) error {
	for {
		err := h.core.fd.Ioctl(uapi.PCMIoctlResume, nil)
		if err == nil {
			h.mu.Lock()
			h.prepared = true
			h.mu.Unlock()
			return nil
		}
		if !errors.Is(err, syscall.EAGAIN) {
			return h.recoverPrepare(capture, "suspend")
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (h *Handle) recoverPrepare(capture bool, reason string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil || h.software == nil {
		return fmt.Errorf("%w: cannot recover an unconfigured stream", ErrState)
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlPrepare, nil); err != nil {
		return operationError(fmt.Sprintf("recover %s: prepare", reason), err)
	}
	h.prepared = true
	if capture {
		if err := h.core.fd.Ioctl(uapi.PCMIoctlStart, nil); err != nil {
			h.prepared = false
			return operationError(fmt.Sprintf("recover %s: start capture", reason), err)
		}
	}
	return nil
}

func validateSoftwareConfig(config SoftwareConfig, hardware HardwareSetup) error {
	if config.TimestampMode != TimestampDisabled && config.TimestampMode != TimestampEnabled {
		return errors.New("alsa pcm: invalid timestamp mode")
	}
	if config.TimestampClock > TimestampMonotonicRaw {
		return errors.New("alsa pcm: invalid timestamp clock")
	}
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
	if config.Access != RWInterleaved && config.Access != RWNonInterleaved {
		return fmt.Errorf("alsa pcm: unsupported access %d", config.Access)
	}
	if _, ok := config.Format.PhysicalBytes(); !ok {
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

func (h *Handle) capabilities() (Capabilities, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	params := anyHardwareParams()
	params.RequestMask = ^uint32(0)
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWRefine, unsafe.Pointer(&params)); err != nil {
		return Capabilities{}, operationError("refine hardware capabilities", err)
	}
	return capabilitiesFromParams(params), nil
}

func capabilitiesFromParams(params uapi.PCMHWParams) Capabilities {
	return Capabilities{
		Accesses:     accessValues(params.Masks[uapi.PCMHWParamAccess-uapi.PCMHWParamFirstMask]),
		Formats:      formatValues(params.Masks[uapi.PCMHWParamFormat-uapi.PCMHWParamFirstMask]),
		SampleBits:   rangeValue(params, uapi.PCMHWParamSampleBits),
		FrameBits:    rangeValue(params, uapi.PCMHWParamFrameBits),
		Channels:     rangeValue(params, uapi.PCMHWParamChannels),
		Rate:         rangeValue(params, uapi.PCMHWParamRate),
		PeriodTime:   rangeValue(params, uapi.PCMHWParamPeriodTime),
		PeriodFrames: rangeValue(params, uapi.PCMHWParamPeriodSize),
		PeriodBytes:  rangeValue(params, uapi.PCMHWParamPeriodBytes),
		Periods:      rangeValue(params, uapi.PCMHWParamPeriods),
		BufferTime:   rangeValue(params, uapi.PCMHWParamBufferTime),
		BufferFrames: rangeValue(params, uapi.PCMHWParamBufferSize),
		BufferBytes:  rangeValue(params, uapi.PCMHWParamBufferBytes),
		TickTime:     rangeValue(params, uapi.PCMHWParamTickTime),
	}
}

func accessValues(mask uapi.PCMMask) []Access {
	var values []Access
	for _, value := range maskValues(mask) {
		if value <= int(RWNonInterleaved) {
			values = append(values, Access(value))
		}
	}
	return values
}

func formatValues(mask uapi.PCMMask) []Format {
	values := maskValues(mask)
	formats := make([]Format, len(values))
	for index, value := range values {
		formats[index] = Format(value)
	}
	return formats
}

func maskValues(mask uapi.PCMMask) []int {
	var values []int
	for wordIndex, word := range mask.Bits {
		for bit := 0; bit < 32; bit++ {
			if word&(uint32(1)<<bit) != 0 {
				values = append(values, wordIndex*32+bit)
			}
		}
	}
	return values
}

func rangeValue(params uapi.PCMHWParams, parameter int) Range {
	interval := params.Intervals[parameter-uapi.PCMHWParamFirstInterval]
	return Range{
		Minimum: interval.Min, Maximum: interval.Max,
		OpenMinimum: interval.Flags&uapi.PCMIntervalOpenMin != 0,
		OpenMaximum: interval.Flags&uapi.PCMIntervalOpenMax != 0,
		Integer:     interval.Flags&uapi.PCMIntervalInteger != 0,
		Empty:       interval.Flags&uapi.PCMIntervalEmpty != 0,
	}
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

func setIntervalRange(p *uapi.PCMHWParams, parameter int, minimum, maximum uint32) {
	index := parameter - uapi.PCMHWParamFirstInterval
	p.Intervals[index] = uapi.PCMInterval{Min: minimum, Max: maximum, Flags: uapi.PCMIntervalInteger}
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
		InfoFlags:  params.Info,
	}
}

func (h *Handle) reset() error {
	if _, err := h.requireHardware("reset"); err != nil {
		return err
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	if err := h.core.fd.Ioctl(uapi.PCMIoctlReset, nil); err != nil {
		return operationError("reset", err)
	}
	return nil
}

func (h *Handle) pause(paused bool) error {
	setup, err := h.requireHardware("pause")
	if err != nil {
		return err
	}
	if !setup.CanPause() {
		return errors.New("alsa pcm: hardware does not support pause")
	}
	h.mu.Lock()
	prepared := h.prepared
	h.mu.Unlock()
	if !prepared {
		return fmt.Errorf("%w: prepare before pause", ErrState)
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	value := int32(0)
	if paused {
		value = 1
	}
	if err := h.core.fd.Ioctl(uapi.PCMIoctlPause, unsafe.Pointer(&value)); err != nil {
		return operationError("pause", err)
	}
	return nil
}

func (h *Handle) resume() error {
	setup, err := h.requireHardware("resume")
	if err != nil {
		return err
	}
	if !setup.CanResume() {
		return errors.New("alsa pcm: hardware does not support resume")
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	if err := h.core.fd.Ioctl(uapi.PCMIoctlResume, nil); err != nil {
		return operationError("resume", err)
	}
	return nil
}

func (h *Handle) rewind(frames uint64) (uint64, error) {
	return h.moveApplicationPointer(uapi.PCMIoctlRewind, frames, "rewind")
}

func (h *Handle) forward(frames uint64) (uint64, error) {
	return h.moveApplicationPointer(uapi.PCMIoctlForward, frames, "forward")
}

func (h *Handle) moveApplicationPointer(request uintptr, frames uint64, operation string) (uint64, error) {
	if _, err := h.requireHardware(operation); err != nil {
		return 0, err
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	actual := frames
	if err := h.core.fd.Ioctl(request, unsafe.Pointer(&actual)); err != nil {
		return 0, operationError(operation, err)
	}
	return actual, nil
}

func (h *Handle) syncHardware() error {
	if _, err := h.requireHardware("synchronize hardware pointer"); err != nil {
		return err
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	if err := h.core.fd.Ioctl(uapi.PCMIoctlHWSync, nil); err != nil {
		return operationError("synchronize hardware pointer", err)
	}
	return nil
}

func (h *Handle) requireHardware(operation string) (HardwareSetup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hardware == nil {
		return HardwareSetup{}, fmt.Errorf("%w: configure hardware before %s", ErrState, operation)
	}
	return *h.hardware, nil
}

func (h *Handle) link(peer *Handle) error {
	if peer == nil {
		return errors.New("alsa pcm: cannot link a nil peer")
	}
	if peer == h {
		return errors.New("alsa pcm: cannot link a stream to itself")
	}
	h.mu.Lock()
	direction := h.stream
	configured := h.hardware != nil
	h.mu.Unlock()
	peer.mu.Lock()
	peerDirection := peer.stream
	peerConfigured := peer.hardware != nil
	peer.mu.Unlock()
	if direction != peerDirection {
		return ErrDirection
	}
	if !configured || !peerConfigured {
		return fmt.Errorf("%w: configure both streams before linking", ErrState)
	}
	return peer.core.fd.WithNumber(func(number int) error {
		peerFD := int32(number)
		if err := h.core.fd.Ioctl(uapi.PCMIoctlLink, unsafe.Pointer(&peerFD)); err != nil {
			return operationError("link", err)
		}
		return nil
	})
}

func (h *Handle) unlink() error {
	if err := h.core.fd.Ioctl(uapi.PCMIoctlUnlink, nil); err != nil {
		return operationError("unlink", err)
	}
	return nil
}

func operationError(operation string, err error) error {
	if errors.Is(err, syscall.ENODEV) || errors.Is(err, syscall.ENXIO) {
		return fmt.Errorf("alsa pcm: %s: %w: %w", operation, ErrDisconnected, err)
	}
	return fmt.Errorf("alsa pcm: %s: %w", operation, err)
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
