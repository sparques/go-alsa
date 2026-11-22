package pcm

import "errors"

var (
	// ErrProtocol reports an incompatible kernel PCM protocol.
	ErrProtocol = errors.New("alsa pcm: incompatible kernel protocol")
	// ErrDirection reports a mismatch between the requested stream direction
	// and the opened kernel PCM node.
	ErrDirection = errors.New("alsa pcm: stream direction mismatch")
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
	core handleCore
	info Info
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
func (h *Handle) Close() error { return h.core.close() }
