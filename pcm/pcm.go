package pcm

import (
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
	mu       sync.Mutex
	core     handleCore
	info     Info
	stream   streamDirection
	hardware *HardwareSetup
	software *SoftwareSetup
	prepared bool
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
