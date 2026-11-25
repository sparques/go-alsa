// go-amixer inspects and changes simple mixer elements without libasound.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sparques/go-alsa/mixer"
)

type options struct {
	card                string
	name                string
	index               uint
	playbackVolume      string
	captureVolume       string
	playbackSwitch      string
	captureSwitch       string
	enumeratedSelection string
}

func main() {
	var opts options
	flag.StringVar(&opts.card, "card", "0", "numeric card number or stable card ID")
	flag.StringVar(&opts.name, "name", "", "simple or enumerated element name")
	flag.UintVar(&opts.index, "index", 0, "element index")
	flag.StringVar(&opts.playbackVolume, "playback-volume", "", "normalized playback volume(s), comma-separated 0..1")
	flag.StringVar(&opts.captureVolume, "capture-volume", "", "normalized capture volume(s), comma-separated 0..1")
	flag.StringVar(&opts.playbackSwitch, "playback-switch", "", "playback switch value(s): on/off, comma-separated")
	flag.StringVar(&opts.captureSwitch, "capture-switch", "", "capture switch value(s): on/off, comma-separated")
	flag.StringVar(&opts.enumeratedSelection, "enum", "", "enumerated item index/indices, comma-separated")
	flag.Parse()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(opts options) error {
	if actionCount(opts) > 1 {
		return fmt.Errorf("choose only one setter flag")
	}
	if actionCount(opts) != 0 && opts.name == "" {
		return fmt.Errorf("-name is required when setting a value")
	}
	handle, err := mixer.Open(opts.card)
	if err != nil {
		return err
	}
	defer handle.Close()

	elements, err := handle.Elements()
	if err != nil {
		return err
	}
	enumerated, err := handle.EnumeratedElements()
	if err != nil {
		return err
	}
	if opts.name == "" {
		for _, element := range elements {
			if err := printElement(element); err != nil {
				return err
			}
		}
		for _, element := range enumerated {
			if err := printEnumerated(element); err != nil {
				return err
			}
		}
		return nil
	}
	for _, element := range elements {
		if element.Name == opts.name && element.Index == uint32(opts.index) {
			if err := setSimple(element, opts); err != nil {
				return err
			}
			return printElement(element)
		}
	}
	for _, element := range enumerated {
		if element.Name == opts.name && element.Index == uint32(opts.index) {
			if opts.enumeratedSelection != "" {
				values, err := parseUint32s(opts.enumeratedSelection)
				if err != nil {
					return err
				}
				if err := element.SetValues(values); err != nil {
					return err
				}
			} else if actionCount(opts) != 0 {
				return fmt.Errorf("%s is enumerated; use -enum", opts.name)
			}
			return printEnumerated(element)
		}
	}
	return fmt.Errorf("mixer element %q,%d not found", opts.name, opts.index)
}

func actionCount(opts options) int {
	count := 0
	for _, value := range []string{opts.playbackVolume, opts.captureVolume, opts.playbackSwitch, opts.captureSwitch, opts.enumeratedSelection} {
		if value != "" {
			count++
		}
	}
	return count
}

func setSimple(element *mixer.Element, opts options) error {
	switch {
	case opts.playbackVolume != "":
		values, err := parseFloats(opts.playbackVolume)
		if err != nil {
			return err
		}
		return element.SetPlaybackVolumeNormalized(values)
	case opts.captureVolume != "":
		values, err := parseFloats(opts.captureVolume)
		if err != nil {
			return err
		}
		return element.SetCaptureVolumeNormalized(values)
	case opts.playbackSwitch != "":
		values, err := parseSwitches(opts.playbackSwitch)
		if err != nil {
			return err
		}
		return element.SetPlaybackSwitch(values)
	case opts.captureSwitch != "":
		values, err := parseSwitches(opts.captureSwitch)
		if err != nil {
			return err
		}
		return element.SetCaptureSwitch(values)
	case opts.enumeratedSelection != "":
		return fmt.Errorf("%s is not enumerated", element.Name)
	default:
		return nil
	}
}

func printElement(element *mixer.Element) error {
	fmt.Printf("%s,%d", element.Name, element.Index)
	if element.HasPlaybackSwitch() {
		values, err := element.PlaybackSwitch()
		if err != nil {
			return err
		}
		fmt.Printf(" playback-switch=%v", values)
	}
	if element.HasPlaybackVolume() {
		value, err := element.PlaybackVolume()
		if err != nil {
			return err
		}
		fmt.Printf(" playback-volume=%v normalized=%v", value.Values, value.Normalized)
		if value.DB != nil {
			fmt.Printf(" dB=%v", value.DB)
		}
	}
	if element.HasCaptureSwitch() && !element.HasCommonSwitch() {
		values, err := element.CaptureSwitch()
		if err != nil {
			return err
		}
		fmt.Printf(" capture-switch=%v", values)
	}
	if element.HasCaptureVolume() && !element.HasCommonVolume() {
		value, err := element.CaptureVolume()
		if err != nil {
			return err
		}
		fmt.Printf(" capture-volume=%v normalized=%v", value.Values, value.Normalized)
		if value.DB != nil {
			fmt.Printf(" dB=%v", value.DB)
		}
	}
	fmt.Println()
	return nil
}

func printEnumerated(element *mixer.EnumeratedElement) error {
	values, err := element.Values()
	if err != nil {
		return err
	}
	fmt.Printf("%s,%d enum=%v items=%q\n", element.Name, element.Index, values, element.Items())
	return nil
}

func parseFloats(value string) ([]float64, error) {
	parts := strings.Split(value, ",")
	result := make([]float64, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
			return nil, fmt.Errorf("invalid normalized volume %q", part)
		}
		result[index] = parsed
	}
	return result, nil
}

func parseSwitches(value string) ([]bool, error) {
	parts := strings.Split(value, ",")
	result := make([]bool, len(parts))
	for index, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "on", "1", "true":
			result[index] = true
		case "off", "0", "false":
		default:
			return nil, fmt.Errorf("invalid switch value %q", part)
		}
	}
	return result, nil
}

func parseUint32s(value string) ([]uint32, error) {
	parts := strings.Split(value, ",")
	result := make([]uint32, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid enumeration index %q", part)
		}
		result[index] = uint32(parsed)
	}
	return result, nil
}
