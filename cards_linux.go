//go:build linux && (amd64 || arm64)

package alsa

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sparques/go-alsa/internal/device"
)

const controlDevicePattern = "/dev/snd/controlC*"

// Cards returns ALSA cards ordered by kernel card number. A machine with no
// ALSA control devices returns an empty slice and no error.
func Cards() ([]Card, error) {
	paths, err := filepath.Glob(controlDevicePattern)
	if err != nil {
		return nil, fmt.Errorf("alsa: enumerate control devices: %w", err)
	}

	cards := make([]Card, 0, len(paths))
	for _, path := range paths {
		number, err := controlCardNumber(path)
		if err != nil {
			return nil, err
		}
		info, err := device.ReadCardInfo(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// Hot-unplug between Glob and Open is not an enumeration failure.
				continue
			}
			return nil, fmt.Errorf("alsa: inspect card %d (%s): %w", number, path, err)
		}
		cards = append(cards, Card{
			Number:     int(info.Card),
			ID:         cString(info.ID[:]),
			Driver:     cString(info.Driver[:]),
			Name:       cString(info.Name[:]),
			LongName:   cString(info.LongName[:]),
			MixerName:  cString(info.MixerName[:]),
			Components: cString(info.Components[:]),
			Path:       path,
		})
	}
	slices.SortFunc(cards, func(a, b Card) int { return a.Number - b.Number })
	return cards, nil
}

// PCMs returns the PCM directions exposed by all ALSA cards, ordered by card,
// device, and then playback before capture. A machine with no ALSA cards
// returns an empty slice and no error.
func PCMs() ([]PCM, error) {
	paths, err := filepath.Glob(controlDevicePattern)
	if err != nil {
		return nil, fmt.Errorf("alsa: enumerate control devices: %w", err)
	}

	var pcms []PCM
	for _, path := range paths {
		number, err := controlCardNumber(path)
		if err != nil {
			return nil, err
		}
		items, err := device.ReadPCMs(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("alsa: enumerate PCMs for card %d (%s): %w", number, path, err)
		}
		for _, item := range items {
			stream := Playback
			suffix := "p"
			if item.Stream == device.StreamCapture {
				stream = Capture
				suffix = "c"
			}
			pcms = append(pcms, PCM{
				Card:                int(item.Card),
				Device:              int(item.Device),
				Stream:              stream,
				ID:                  cString(item.ID[:]),
				Name:                cString(item.Name[:]),
				SubdeviceName:       cString(item.Subname[:]),
				Subdevices:          int(item.SubdevicesCount),
				AvailableSubdevices: int(item.SubdevicesAvailable),
				Path:                fmt.Sprintf("/dev/snd/pcmC%dD%d%s", item.Card, item.Device, suffix),
			})
		}
	}
	slices.SortFunc(pcms, func(a, b PCM) int {
		if difference := a.Card - b.Card; difference != 0 {
			return difference
		}
		if difference := a.Device - b.Device; difference != 0 {
			return difference
		}
		return int(a.Stream) - int(b.Stream)
	})
	return pcms, nil
}

func controlCardNumber(path string) (int, error) {
	name := filepath.Base(path)
	value, ok := strings.CutPrefix(name, "controlC")
	if !ok || value == "" {
		return 0, fmt.Errorf("%w: ALSA control path %q", ErrMalformedDevice, path)
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("%w: ALSA control path %q", ErrMalformedDevice, path)
	}
	return number, nil
}

func cString(value []byte) string {
	if index := slices.Index(value, byte(0)); index >= 0 {
		value = value[:index]
	}
	return string(value)
}
