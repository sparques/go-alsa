package alsa

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// HardwareAddress is a parsed direct-kernel ALSA PCM name. Card is either a
// decimal kernel card number or a stable kernel card ID. Device is required.
// A Subdevice of -1 asks the kernel to choose an available subdevice and is
// the default produced by ParseHardwareAddress when SUBDEV is omitted.
type HardwareAddress struct {
	Card      string
	Device    int
	Subdevice int
}

// String returns the unambiguous named-parameter form of a hardware address.
func (a HardwareAddress) String() string {
	return fmt.Sprintf("hw:CARD=%s,DEV=%d,SUBDEV=%d", a.Card, a.Device, a.Subdevice)
}

// ParseHardwareAddress parses direct ALSA PCM names in common positional or
// named forms:
//
//	hw:0,0
//	hw:0,0,1
//	hw:CARD=Generic,DEV=0
//	hw:CARD=Generic,DEV=0,SUBDEV=1
//
// Positional and named parameters may not be mixed. Plugin names such as
// default and plughw are deliberately rejected because they do not identify a
// direct kernel PCM.
func ParseHardwareAddress(name string) (HardwareAddress, error) {
	value, ok := strings.CutPrefix(name, "hw:")
	if !ok || value == "" {
		return HardwareAddress{}, malformedAddress(name, "expected hw: prefix and parameters")
	}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return HardwareAddress{}, malformedAddress(name, "empty parameter or surrounding whitespace")
		}
	}

	named := strings.Contains(parts[0], "=")
	if named {
		return parseNamedHardwareAddress(name, parts)
	}
	return parsePositionalHardwareAddress(name, parts)
}

func parsePositionalHardwareAddress(name string, parts []string) (HardwareAddress, error) {
	if len(parts) < 2 || len(parts) > 3 {
		return HardwareAddress{}, malformedAddress(name, "positional form needs card, device, and optional subdevice")
	}
	for _, part := range parts {
		if strings.Contains(part, "=") {
			return HardwareAddress{}, malformedAddress(name, "cannot mix positional and named parameters")
		}
	}
	if err := validateCardSelector(parts[0]); err != nil {
		return HardwareAddress{}, malformedAddress(name, err.Error())
	}
	device, err := nonnegativeInteger(parts[1])
	if err != nil {
		return HardwareAddress{}, malformedAddress(name, "device must be a non-negative integer")
	}
	address := HardwareAddress{Card: parts[0], Device: device, Subdevice: -1}
	if len(parts) == 3 {
		address.Subdevice, err = subdeviceInteger(parts[2])
		if err != nil {
			return HardwareAddress{}, malformedAddress(name, "subdevice must be -1 or a non-negative integer")
		}
	}
	return address, nil
}

func parseNamedHardwareAddress(name string, parts []string) (HardwareAddress, error) {
	values := make(map[string]string, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || value == "" {
			return HardwareAddress{}, malformedAddress(name, "named parameters require non-empty values")
		}
		if key != "CARD" && key != "DEV" && key != "SUBDEV" {
			return HardwareAddress{}, malformedAddress(name, "unknown parameter "+key)
		}
		if _, exists := values[key]; exists {
			return HardwareAddress{}, malformedAddress(name, "duplicate parameter "+key)
		}
		values[key] = value
	}
	if len(values) < 2 || values["CARD"] == "" || values["DEV"] == "" {
		return HardwareAddress{}, malformedAddress(name, "CARD and DEV are required")
	}
	if err := validateCardSelector(values["CARD"]); err != nil {
		return HardwareAddress{}, malformedAddress(name, err.Error())
	}
	device, err := nonnegativeInteger(values["DEV"])
	if err != nil {
		return HardwareAddress{}, malformedAddress(name, "DEV must be a non-negative integer")
	}
	address := HardwareAddress{Card: values["CARD"], Device: device, Subdevice: -1}
	if value, exists := values["SUBDEV"]; exists {
		address.Subdevice, err = subdeviceInteger(value)
		if err != nil {
			return HardwareAddress{}, malformedAddress(name, "SUBDEV must be -1 or a non-negative integer")
		}
	}
	return address, nil
}

func validateCardSelector(value string) error {
	if value == "" {
		return errors.New("card must not be empty")
	}
	if number, err := strconv.Atoi(value); err == nil {
		if number < 0 {
			return errors.New("card number must be non-negative")
		}
		return nil
	}
	for _, character := range value {
		if character > 127 || !(character == '_' || character == '-' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z') {
			return errors.New("card ID contains an unsupported character")
		}
	}
	return nil
}

func subdeviceInteger(value string) (int, error) {
	if value == "-1" {
		return -1, nil
	}
	return nonnegativeInteger(value)
}

func nonnegativeInteger(value string) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, errors.New("not a non-negative integer")
	}
	return number, nil
}

func malformedAddress(name, detail string) error {
	return &DeviceError{Op: "parse", Device: name, Err: fmt.Errorf("%w: %s", ErrMalformedDevice, detail)}
}

// ResolvedPCM binds a parsed address and direction to kernel-reported PCM
// metadata. PCM.Path is the device node to open.
type ResolvedPCM struct {
	Address HardwareAddress
	PCM     PCM
}

// ResolveHardwarePCM resolves a direct hardware name against the currently
// visible kernel cards and PCM devices.
func ResolveHardwarePCM(name string, stream Stream) (ResolvedPCM, error) {
	address, err := ParseHardwareAddress(name)
	if err != nil {
		return ResolvedPCM{}, err
	}
	if stream != Playback && stream != Capture {
		return ResolvedPCM{}, &DeviceError{Op: "resolve", Device: name, Err: ErrDirectionUnavailable}
	}
	cards, err := Cards()
	if err != nil {
		return ResolvedPCM{}, &DeviceError{Op: "enumerate cards for", Device: name, Err: err}
	}
	pcms, err := PCMs()
	if err != nil {
		return ResolvedPCM{}, &DeviceError{Op: "enumerate PCMs for", Device: name, Err: err}
	}
	return resolveHardwarePCM(address, stream, cards, pcms)
}

func resolveHardwarePCM(address HardwareAddress, stream Stream, cards []Card, pcms []PCM) (ResolvedPCM, error) {
	name := address.String()
	cardNumber := -1
	if number, err := strconv.Atoi(address.Card); err == nil {
		for _, card := range cards {
			if card.Number == number {
				cardNumber = number
				break
			}
		}
	} else {
		for _, card := range cards {
			if card.ID == address.Card {
				cardNumber = card.Number
				break
			}
		}
	}
	if cardNumber < 0 {
		return ResolvedPCM{}, &DeviceError{Op: "resolve", Device: name, Err: ErrDeviceNotFound}
	}

	deviceExists := false
	for _, pcm := range pcms {
		if pcm.Card != cardNumber || pcm.Device != address.Device {
			continue
		}
		deviceExists = true
		if pcm.Stream != stream {
			continue
		}
		if address.Subdevice < -1 || address.Subdevice >= pcm.Subdevices {
			return ResolvedPCM{}, &DeviceError{Op: "resolve", Device: name, Err: ErrSubdeviceUnavailable}
		}
		address.Card = strconv.Itoa(cardNumber)
		return ResolvedPCM{Address: address, PCM: pcm}, nil
	}
	if deviceExists {
		return ResolvedPCM{}, &DeviceError{Op: "resolve", Device: name, Err: ErrDirectionUnavailable}
	}
	return ResolvedPCM{}, &DeviceError{Op: "resolve", Device: name, Err: ErrDeviceNotFound}
}
