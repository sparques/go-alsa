package control

import (
	"errors"
	"fmt"

	alsaconfig "github.com/sparques/go-alsa/config"
)

// ErrUnsupportedPlugin reports a configured control plugin without a native
// execution implementation.
var ErrUnsupportedPlugin = errors.New("alsa control: unsupported configured plugin")

// UnsupportedPluginError identifies the first unsupported control graph node.
type UnsupportedPluginError struct {
	Plugin   string
	Node     string
	Position alsaconfig.Position
}

func (e *UnsupportedPluginError) Error() string {
	location := ""
	if e.Position.Source != "" {
		location = " at " + e.Position.Source
		if e.Position.Line > 0 {
			location += fmt.Sprintf(":%d:%d", e.Position.Line, e.Position.Column)
		}
	}
	return fmt.Sprintf("%v %q at node %q%s", ErrUnsupportedPlugin, e.Plugin, e.Node, location)
}

func (e *UnsupportedPluginError) Unwrap() error { return ErrUnsupportedPlugin }

// OpenNamed evaluates one configured control graph, traverses routing-only
// empty nodes, and opens its direct type=hw endpoint.
func OpenNamed(evaluator *alsaconfig.Evaluator, name string) (*Handle, error) {
	if evaluator == nil {
		return nil, errors.New("alsa control: named open requires a configuration evaluator")
	}
	graph, err := evaluator.ControlGraph(name)
	if err != nil {
		return nil, err
	}
	selector, err := directGraphSelector(graph.Root)
	if err != nil {
		return nil, err
	}
	return Open(selector)
}

func directGraphSelector(node *alsaconfig.ControlNode) (string, error) {
	for node != nil {
		switch node.Type {
		case "hw":
			definition := alsaconfig.Control{Name: node.Name, Type: node.Type, Position: node.Position, Fields: node.Fields}
			return definition.HardwareSelector()
		case "empty":
			node = node.Child
		default:
			return "", &UnsupportedPluginError{Plugin: node.Type, Node: node.Name, Position: node.Position}
		}
	}
	return "", errors.New("alsa control: configured routing graph has no endpoint")
}
