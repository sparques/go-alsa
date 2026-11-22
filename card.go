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
