package control

import (
	"errors"

	alsaconfig "github.com/sparques/go-alsa/config"
)

// OpenNamed evaluates one configured control name and opens it when it
// resolves to a direct type=hw definition. Software/server control plugins
// return an explicit configuration error.
func OpenNamed(evaluator *alsaconfig.Evaluator, name string) (*Handle, error) {
	if evaluator == nil {
		return nil, errors.New("alsa control: named open requires a configuration evaluator")
	}
	definition, err := evaluator.ControlName(name)
	if err != nil {
		return nil, err
	}
	selector, err := definition.HardwareSelector()
	if err != nil {
		return nil, err
	}
	return Open(selector)
}
