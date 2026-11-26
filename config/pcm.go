package config

import (
	"fmt"
	"strconv"
	"strings"
)

// PCM is one evaluated named PCM definition. Fields contains the last direct
// assignment to each field; nested compounds remain structured Values.
type PCM struct {
	Name     string
	Type     string
	Position Position
	Fields   map[string]Value
}

// HardwareSelector converts an evaluated type=hw definition into the direct
// selector accepted by the root ALSA resolver and pcm.OpenHardware.
func (p PCM) HardwareSelector() (string, error) {
	if p.Type != "hw" {
		return "", fmt.Errorf("alsa config: PCM %q has unsupported direct type %q", p.Name, p.Type)
	}
	card, ok := p.Fields["card"]
	if !ok {
		return "", fmt.Errorf("alsa config: PCM %q has no card field", p.Name)
	}
	cardText, err := pcmFieldText(card)
	if err != nil {
		return "", fmt.Errorf("alsa config: PCM %q card: %w", p.Name, err)
	}
	device := int64(0)
	if value, exists := p.Fields["device"]; exists {
		device, err = pcmFieldInteger(value)
		if err != nil || device < 0 {
			return "", fmt.Errorf("alsa config: PCM %q has invalid device", p.Name)
		}
	}
	selector := fmt.Sprintf("hw:CARD=%s,DEV=%d", cardText, device)
	if value, exists := p.Fields["subdevice"]; exists {
		subdevice, subErr := pcmFieldInteger(value)
		if subErr != nil || subdevice < -1 {
			return "", fmt.Errorf("alsa config: PCM %q has invalid subdevice", p.Name)
		}
		selector += fmt.Sprintf(",SUBDEV=%d", subdevice)
	}
	return selector, nil
}

func pcmFieldText(value Value) (string, error) {
	switch value.Kind {
	case ValueString:
		if value.String == "" || strings.ContainsAny(value.String, ",= \t\r\n") {
			return "", fmt.Errorf("invalid string %q", value.String)
		}
		return value.String, nil
	case ValueInteger:
		if value.Integer < 0 {
			return "", fmt.Errorf("negative value")
		}
		return strconv.FormatInt(value.Integer, 10), nil
	}
	return "", fmt.Errorf("value is not a card identifier")
}

func pcmFieldInteger(value Value) (int64, error) {
	if value.Kind != ValueInteger {
		return 0, fmt.Errorf("value is not an integer")
	}
	return value.Integer, nil
}

// PCM resolves and evaluates a named PCM definition. Name may be either
// "default" or its canonical "pcm.default" path.
func (e *Evaluator) PCM(name string) (PCM, error) {
	path := name
	if !strings.HasPrefix(path, "pcm.") {
		path = "pcm." + path
	}
	value, err := e.Reference(path)
	if err != nil {
		return PCM{}, err
	}
	if value.Kind != ValueCompound {
		return PCM{}, fmt.Errorf("alsa config: PCM %q resolves to a non-compound value", name)
	}
	fields := make(map[string]Value)
	for _, statement := range value.Entries {
		if statement.Assignment == nil {
			continue
		}
		fields[statement.Assignment.Name] = statement.Assignment.Value
	}
	typeValue, ok := fields["type"]
	if !ok || typeValue.Kind != ValueString || typeValue.String == "" {
		return PCM{}, fmt.Errorf("alsa config: PCM %q has no string type", name)
	}
	return PCM{Name: strings.TrimPrefix(path, "pcm."), Type: typeValue.String, Position: value.Position, Fields: fields}, nil
}

// Field returns one evaluated direct field.
func (p PCM) Field(name string) (Value, bool) {
	value, ok := p.Fields[name]
	return value, ok
}
