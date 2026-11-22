package alsa

// Card describes one ALSA card reported by a kernel control device.
type Card struct {
	Number     int
	ID         string
	Driver     string
	Name       string
	LongName   string
	MixerName  string
	Components string
	Path       string
}

// Stream identifies the direction of a PCM stream.
type Stream uint8

const (
	Playback Stream = iota
	Capture
)

func (s Stream) String() string {
	switch s {
	case Playback:
		return "playback"
	case Capture:
		return "capture"
	default:
		return "unknown"
	}
}

// PCM describes one direction of a kernel ALSA PCM device. A full-duplex
// device is represented by separate playback and capture entries because the
// kernel may report different subdevices and availability for each direction.
type PCM struct {
	Card                int
	Device              int
	Stream              Stream
	ID                  string
	Name                string
	SubdeviceName       string
	Subdevices          int
	AvailableSubdevices int
	Path                string
}
