package pcm

import (
	"context"
	"errors"
	"sync"
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
}

// RecoveryPolicy controls whether transfers report XRUNs or recover and retry.
type RecoveryPolicy uint8

const (
	// RecoveryStrict returns ErrXRUN and leaves the stream requiring Prepare.
	RecoveryStrict RecoveryPolicy = iota
	// RecoveryAutomatic prepares the stream, restarts capture when needed, and
	// retries the interrupted transfer.
	RecoveryAutomatic
)

// Stats counts capture overruns and playback underruns observed by this handle.
type Stats struct {
	Overruns  uint64
	Underruns uint64
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
	return Stats{Overruns: h.overruns, Underruns: h.underruns}
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

// Access identifies how application frames are transferred.
type Access int32

const (
	// RWInterleaved transfers all channel samples in one interleaved buffer.
	RWInterleaved Access = 3
)

// Format identifies the in-memory sample representation.
type Format int32

const (
	// S16LE is signed 16-bit little-endian PCM.
	S16LE Format = 2
)

// HardwareConfig constrains direct hardware parameters. This initial API
// requires exact values; later negotiation APIs will expose near/range policy.
type HardwareConfig struct {
	Access       Access
	Format       Format
	Channels     int
	Rate         int
	PeriodFrames int
	BufferFrames int
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
}

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
}

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
