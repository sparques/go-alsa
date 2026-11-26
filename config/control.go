package config

import (
	"fmt"
	"strings"
)

// Control is one evaluated named ALSA control definition. Unknown fields and
// control plugin types are preserved for explicit compatibility decisions.
type Control struct {
	Name     string
	Type     string
	Position Position
	Fields   map[string]Value
}

// HardwareSelector converts an evaluated type=hw control definition to the
// selector accepted by control.Open.
func (c Control) HardwareSelector() (string, error) {
	if c.Type != "hw" {
		return "", fmt.Errorf("alsa config: control %q has unsupported direct type %q", c.Name, c.Type)
	}
	card, ok := c.Fields["card"]
	if !ok {
		return "", fmt.Errorf("alsa config: control %q has no card field", c.Name)
	}
	text, err := pcmFieldText(card)
	if err != nil {
		return "", fmt.Errorf("alsa config: control %q card: %w", c.Name, err)
	}
	return "hw:CARD=" + text, nil
}

// ControlName evaluates a control invocation with optional positional, named,
// or braced runtime arguments.
func (e *Evaluator) ControlName(invocation string) (Control, error) {
	evaluator, name, err := e.bindInvocation("ctl", "control", invocation)
	if err != nil {
		return Control{}, err
	}
	path := name
	if !strings.HasPrefix(path, "ctl.") {
		path = "ctl." + path
	}
	value, err := evaluator.Reference(path)
	if err != nil {
		return Control{}, err
	}
	if value.Kind != ValueCompound {
		return Control{}, fmt.Errorf("alsa config: control %q resolves to a non-compound value", name)
	}
	fields := make(map[string]Value)
	for _, statement := range value.Entries {
		if statement.Assignment != nil {
			fields[statement.Assignment.Name] = statement.Assignment.Value
		}
	}
	typeValue, ok := fields["type"]
	if !ok || typeValue.Kind != ValueString || typeValue.String == "" {
		return Control{}, fmt.Errorf("alsa config: control %q has no string type", name)
	}
	return Control{Name: strings.TrimPrefix(path, "ctl."), Type: typeValue.String, Position: value.Position, Fields: fields}, nil
}

// Field returns one evaluated direct field.
func (c Control) Field(name string) (Value, bool) {
	value, ok := c.Fields[name]
	return value, ok
}
