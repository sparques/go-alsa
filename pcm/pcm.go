package pcm

import (
	"context"
	"errors"
	"sync"

	alsa "github.com/sparques/go-alsa"
)

var (
	// ErrProtocol reports an incompatible kernel PCM protocol.
	ErrProtocol = errors.New("alsa pcm: incompatible kernel protocol")
	// ErrDirection reports a mismatch between the requested stream direction
	// and the opened kernel PCM node.
	ErrDirection = errors.New("alsa pcm: stream direction mismatch")
	// ErrState reports an operation attempted in the wrong PCM lifecycle state.
	ErrState = errors.New("alsa pcm: invalid state")
	// ErrXRUN reports a playback underrun or capture overrun. Call Prepare to
	// recover before transferring more frames.
	ErrXRUN = errors.New("alsa pcm: xrun")
	// ErrSuspended reports a stream suspended by the system. Resume may recover
	// it; Prepare is the fallback when the driver cannot resume in place.
	ErrSuspended = errors.New("alsa pcm: suspended")
	// ErrDisconnected reports hardware removal or a disconnected PCM stream.
	ErrDisconnected = alsa.ErrDisconnected
)

// Info is immutable metadata returned by the kernel for an opened PCM stream.
type Info struct {
	Card                int
	Device              int
	Subdevice           int
	ID                  string
	Name                string
	SubdeviceName       string
	Subdevices          int
	AvailableSubdevices int
}

// Handle owns one direct kernel ALSA PCM stream.
type Handle struct {
	mu        sync.Mutex
	ioMu      sync.Mutex
	core      handleCore
	info      Info
	stream    streamDirection
	hardware  *HardwareSetup
	software  *SoftwareSetup
	prepared  bool
	recovery  RecoveryPolicy
	overruns  uint64
	underruns uint64
	suspends  uint64
}

// RecoveryPolicy controls whether transfers report XRUN/suspend failures or
// recover and retry.
type RecoveryPolicy uint8

const (
	// RecoveryStrict returns ErrXRUN or ErrSuspended and leaves recovery to the
	// caller.
	RecoveryStrict RecoveryPolicy = iota
	// RecoveryAutomatic prepares after XRUN, attempts resume after suspend,
	// restarts capture when needed, and retries the interrupted transfer.
	RecoveryAutomatic
)

// Stats counts transfer disruptions observed by this handle.
type Stats struct {
	Overruns    uint64
	Underruns   uint64
	Suspensions uint64
}

// SetRecoveryPolicy changes transfer recovery behavior. Strict is the default.
func (h *Handle) SetRecoveryPolicy(policy RecoveryPolicy) error {
	if policy != RecoveryStrict && policy != RecoveryAutomatic {
		return errors.New("alsa pcm: invalid recovery policy")
	}
	h.mu.Lock()
	h.recovery = policy
	h.mu.Unlock()
	return nil
}

// Stats returns a consistent snapshot of XRUN counters.
func (h *Handle) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Stats{Overruns: h.overruns, Underruns: h.underruns, Suspensions: h.suspends}
}

// Info returns metadata captured when the handle was opened.
func (h *Handle) Info() Info { return h.info }

// ProtocolVersion returns the kernel PCM protocol version as major, minor,
// and micro components.
func (h *Handle) ProtocolVersion() (major, minor, micro int) {
	version := h.core.protocol
	return int(version >> 16), int(version >> 8 & 0xff), int(version & 0xff)
}

// Close releases the PCM descriptor. It is safe to call more than once.
func (h *Handle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.core.close()
}

// Start explicitly starts a prepared stream. Capture applications commonly
// call Start before ReadInterleaved; playback normally starts automatically at
// its configured threshold.
func (h *Handle) Start() error { return h.start() }

// Drop stops the stream immediately and discards queued frames. Call Prepare
// before the next transfer.
func (h *Handle) Drop() error { return h.drop() }

// Drain waits until queued playback frames have reached the device. Kernel
// state is consulted to distinguish completion, XRUN, and disconnection.
func (h *Handle) Drain(ctx context.Context) error { return h.drain(ctx) }

// FreeHardware releases negotiated hardware resources and returns the stream
// to OPEN state. It is safe when parameters have already been freed.
func (h *Handle) FreeHardware() error { return h.freeHardware() }

// ReadInterleaved captures whole S16_LE frames into samples. The sample count
// must be divisible by the configured channel count.
func (h *Handle) ReadInterleaved(ctx context.Context, samples []int16) (int, error) {
	return h.transferInterleaved(ctx, samples, true)
}

// WriteInterleaved plays whole S16_LE frames from samples. The sample count
// must be divisible by the configured channel count.
func (h *Handle) WriteInterleaved(ctx context.Context, samples []int16) (int, error) {
	return h.transferInterleaved(ctx, samples, false)
}

// ReadInterleavedBytes captures whole frames in the configured physical sample
// representation. The buffer length must be divisible by physical frame size.
func (h *Handle) ReadInterleavedBytes(ctx context.Context, buffer []byte) (int, error) {
	return h.transferInterleavedBytes(ctx, buffer, true)
}

// WriteInterleavedBytes plays whole frames in the configured physical sample
// representation. The buffer length must be divisible by physical frame size.
func (h *Handle) WriteInterleavedBytes(ctx context.Context, buffer []byte) (int, error) {
	return h.transferInterleavedBytes(ctx, buffer, false)
}

// ReadNonInterleavedBytes captures frames into one physical-sample byte buffer
// per configured channel. Every channel buffer must have the same length.
func (h *Handle) ReadNonInterleavedBytes(ctx context.Context, channels [][]byte) (int, error) {
	return h.transferNonInterleavedBytes(ctx, channels, true)
}

// WriteNonInterleavedBytes plays frames from one physical-sample byte buffer
// per configured channel. Every channel buffer must have the same length.
func (h *Handle) WriteNonInterleavedBytes(ctx context.Context, channels [][]byte) (int, error) {
	return h.transferNonInterleavedBytes(ctx, channels, false)
}

// Access identifies how application frames are transferred.
type Access int32

const (
	MMAPInterleaved    Access = 0
	MMAPNonInterleaved Access = 1
	MMAPComplex        Access = 2
	// RWInterleaved transfers all channel samples in one interleaved buffer.
	RWInterleaved    Access = 3
	RWNonInterleaved Access = 4
)

func (a Access) String() string {
	switch a {
	case MMAPInterleaved:
		return "MMAP_INTERLEAVED"
	case MMAPNonInterleaved:
		return "MMAP_NONINTERLEAVED"
	case MMAPComplex:
		return "MMAP_COMPLEX"
	case RWInterleaved:
		return "RW_INTERLEAVED"
	case RWNonInterleaved:
		return "RW_NONINTERLEAVED"
	default:
		return "UNKNOWN"
	}
}

// Format identifies the in-memory sample representation.
type Format int32

const (
	S8 Format = 0
	U8 Format = 1
	// S16LE is signed 16-bit little-endian PCM.
	S16LE   Format = 2
	S16BE   Format = 3
	U16LE   Format = 4
	U16BE   Format = 5
	S24LE   Format = 6
	S24BE   Format = 7
	U24LE   Format = 8
	U24BE   Format = 9
	S32LE   Format = 10
	S32BE   Format = 11
	U32LE   Format = 12
	U32BE   Format = 13
	S24_3LE Format = 32
	S24_3BE Format = 33
	U24_3LE Format = 34
	U24_3BE Format = 35
)

func (f Format) String() string {
	names := map[Format]string{
		S8: "S8", U8: "U8", S16LE: "S16_LE", S16BE: "S16_BE",
		U16LE: "U16_LE", U16BE: "U16_BE", S24LE: "S24_LE", S24BE: "S24_BE",
		U24LE: "U24_LE", U24BE: "U24_BE", S32LE: "S32_LE", S32BE: "S32_BE",
		U32LE: "U32_LE", U32BE: "U32_BE", S24_3LE: "S24_3LE", S24_3BE: "S24_3BE",
		U24_3LE: "U24_3LE", U24_3BE: "U24_3BE",
	}
	if name, ok := names[f]; ok {
		return name
	}
	return "UNKNOWN"
}

// PhysicalBytes returns the number of bytes occupied by one sample in memory.
// ALSA's S24_LE/U24_LE formats use four-byte containers; the *_3 variants are
// the packed three-byte representations.
func (f Format) PhysicalBytes() (int, bool) {
	switch f {
	case S8, U8:
		return 1, true
	case S16LE, S16BE, U16LE, U16BE:
		return 2, true
	case S24_3LE, S24_3BE, U24_3LE, U24_3BE:
		return 3, true
	case S24LE, S24BE, U24LE, U24BE, S32LE, S32BE, U32LE, U32BE:
		return 4, true
	default:
		return 0, false
	}
}

// Range describes one kernel hardware-parameter interval. OpenMinimum and
// OpenMaximum indicate excluded bounds.
type Range struct {
	Minimum     uint32
	Maximum     uint32
	OpenMinimum bool
	OpenMaximum bool
	Integer     bool
	Empty       bool
}

// Capabilities contains unconstrained hardware possibilities returned by
// SNDRV_PCM_IOCTL_HW_REFINE.
type Capabilities struct {
	Accesses     []Access
	Formats      []Format
	SampleBits   Range
	FrameBits    Range
	Channels     Range
	Rate         Range
	PeriodTime   Range
	PeriodFrames Range
	PeriodBytes  Range
	Periods      Range
	BufferTime   Range
	BufferFrames Range
	BufferBytes  Range
	TickTime     Range
}

// Capabilities queries unconstrained hardware access, format, and interval
// ranges without changing the PCM state.
func (h *Handle) Capabilities() (Capabilities, error) { return h.capabilities() }

// HardwareConfig describes requested direct hardware parameters.
type HardwareConfig struct {
	Access       Access
	Format       Format
	Channels     int
	Rate         int
	PeriodFrames int
	BufferFrames int
}

// ConfigureHardwareNear keeps access, format, and channel count exact while
// selecting the nearest jointly supported rate, period size, and buffer size.
// The returned setup contains the values accepted by the kernel.
func (h *Handle) ConfigureHardwareNear(config HardwareConfig) (HardwareSetup, error) {
	return h.configureHardwareNear(config)
}

// HardwareSetup contains the parameters accepted by the kernel and additional
// stream properties reported by SNDRV_PCM_IOCTL_HW_PARAMS.
type HardwareSetup struct {
	HardwareConfig
	SampleBits int
	FrameBits  int
	Periods    int
	MSBits     int
	FIFOFrames uint64
	InfoFlags  uint32
}

// CanPause reports native hardware pause support.
func (s HardwareSetup) CanPause() bool { return s.InfoFlags&hardwareInfoPause != 0 }

// CanResume reports native hardware suspend/resume support.
func (s HardwareSetup) CanResume() bool { return s.InfoFlags&hardwareInfoResume != 0 }

const (
	hardwareInfoResume = uint32(0x00000400)
	hardwareInfoPause  = uint32(0x00000800)
)

// State is the kernel PCM lifecycle state.
type State int32

const (
	StateOpen State = iota
	StateSetup
	StatePrepared
	StateRunning
	StateXRUN
	StateDraining
	StatePaused
	StateSuspended
	StateDisconnected
)

func (s State) String() string {
	names := [...]string{"OPEN", "SETUP", "PREPARED", "RUNNING", "XRUN", "DRAINING", "PAUSED", "SUSPENDED", "DISCONNECTED"}
	if s < 0 || int(s) >= len(names) {
		return "UNKNOWN"
	}
	return names[s]
}

// Timestamp is an unmodified kernel timespec. Its clock domain depends on the
// configured timestamp mode and type.
type Timestamp struct {
	Seconds     int64
	Nanoseconds int64
}

// Status is a snapshot returned by SNDRV_PCM_IOCTL_STATUS.
type Status struct {
	State                  State
	TriggerTimestamp       Timestamp
	Timestamp              Timestamp
	ApplicationPointer     uint64
	HardwarePointer        uint64
	DelayFrames            int64
	AvailableFrames        uint64
	MaximumAvailableFrames uint64
	OverrangeCount         uint64
	SuspendedState         State
	AudioTimestamp         Timestamp
	DriverTimestamp        Timestamp
	AudioTimestampAccuracy uint32
}

// Status returns a kernel state snapshot.
func (h *Handle) Status() (Status, error) { return h.status() }

// Delay returns frames queued for playback or frames captured but not yet read.
func (h *Handle) Delay() (int64, error) { return h.delay() }

// Available returns the number of frames that can be transferred without
// blocking. For playback this is writable space; for capture it is readable
// data.
func (h *Handle) Available() (uint64, error) { return h.available() }

// Wait blocks until a transfer can make progress, the stream enters an error
// state, the context is canceled, or the handle is closed. It uses POLLOUT for
// playback and POLLIN for capture.
func (h *Handle) Wait(ctx context.Context) error { return h.wait(ctx) }

// Reset resets application and hardware ring pointers without discarding the
// negotiated parameters.
func (h *Handle) Reset() error { return h.reset() }

// Pause pauses or releases a running stream when HardwareSetup.CanPause is
// true.
func (h *Handle) Pause(paused bool) error { return h.pause(paused) }

// Resume resumes a stream suspended by the system when
// HardwareSetup.CanResume is true. This is distinct from releasing Pause.
func (h *Handle) Resume() error { return h.resume() }

// Rewind moves the application pointer backward by up to frames and returns
// the actual movement reported by the kernel.
func (h *Handle) Rewind(frames uint64) (uint64, error) { return h.rewind(frames) }

// Forward moves the application pointer forward by up to frames and returns
// the actual movement reported by the kernel.
func (h *Handle) Forward(frames uint64) (uint64, error) { return h.forward(frames) }

// SyncHardware synchronizes the userspace-visible hardware pointer.
func (h *Handle) SyncHardware() error { return h.syncHardware() }

// Link synchronizes this stream's start, stop, pause, and resume operations
// with peer. Both handles must have the same direction and configured hardware
// parameters. The kernel additionally requires compatible hardware.
func (h *Handle) Link(peer *Handle) error { return h.link(peer) }

// Unlink removes this stream from its kernel synchronization group.
func (h *Handle) Unlink() error { return h.unlink() }

type streamDirection uint8

const (
	streamPlayback streamDirection = iota
	streamCapture
)

// SoftwareConfig controls when the kernel wakes the application and starts or
// stops the stream. Frame counts are relative to the negotiated hardware ring.
type SoftwareConfig struct {
	AvailableMinimum int
	StartThreshold   uint64
	StopThreshold    uint64
	SilenceThreshold uint64
	SilenceFrames    uint64
	TimestampMode    TimestampMode
	TimestampClock   TimestampClock
}

// TimestampMode controls whether Status timestamps are updated by the kernel.
type TimestampMode int32

const (
	TimestampDisabled TimestampMode = iota
	TimestampEnabled
)

// TimestampClock selects the clock used for kernel PCM status timestamps.
// Realtime is the legacy ALSA default. Monotonic clocks avoid wall-clock
// adjustments and are generally preferable for measuring stream timing.
type TimestampClock uint32

const (
	TimestampRealtime TimestampClock = iota
	TimestampMonotonic
	TimestampMonotonicRaw
)

// SoftwareSetup is the software-parameter set accepted by the kernel.
type SoftwareSetup struct {
	SoftwareConfig
	Boundary uint64
}

// DefaultSoftwareConfig returns low-surprise thresholds modeled on libasound's
// direct PCM setup: wake once per period, start capture immediately, start
// playback once the largest whole-period portion of the buffer is filled, and
// enter XRUN when the full ring is exhausted.
func DefaultSoftwareConfig(streamPlaybackDirection bool, hardware HardwareSetup) SoftwareConfig {
	start := uint64(1)
	if streamPlaybackDirection {
		periods := hardware.BufferFrames / hardware.PeriodFrames
		start = uint64(periods * hardware.PeriodFrames)
	}
	return SoftwareConfig{
		AvailableMinimum: hardware.PeriodFrames,
		StartThreshold:   start,
		StopThreshold:    uint64(hardware.BufferFrames),
	}
}
