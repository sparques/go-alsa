package pcm

import (
	"errors"
	"fmt"

	alsa "github.com/sparques/go-alsa"
	alsaconfig "github.com/sparques/go-alsa/config"
)

// ErrConversionRequired reports differing application/hardware parameters on
// a named graph that contains no plug conversion node.
var ErrConversionRequired = errors.New("alsa pcm: named graph does not permit conversion")

// PlannedPlugin records one configured node retained in a named open plan.
type PlannedPlugin struct {
	Type     string
	Name     string
	Position alsaconfig.Position
}

// NamedPlan is a direction-selected route from a configured name to direct
// hardware. Plug nodes are recorded but not instantiated until negotiated
// hardware parameters are available.
type NamedPlan struct {
	Name             string
	Stream           alsa.Stream
	HardwareSelector string
	Plugins          []PlannedPlugin
	RequiresPlug     bool
}

// PlanNamed resolves a configured graph for one stream. It accepts routing-
// only empty/asym nodes, records plug nodes, and rejects every other executable
// plugin with ErrUnsupportedPlugin.
func PlanNamed(evaluator *alsaconfig.Evaluator, name string, stream alsa.Stream) (NamedPlan, error) {
	if evaluator == nil {
		return NamedPlan{}, errors.New("alsa pcm: named plan requires a configuration evaluator")
	}
	if stream != alsa.Playback && stream != alsa.Capture {
		return NamedPlan{}, errors.New("alsa pcm: invalid named plan stream")
	}
	graph, err := evaluator.Graph(name)
	if err != nil {
		return NamedPlan{}, err
	}
	plan := NamedPlan{Name: name, Stream: stream}
	node := graph.Root
	for node != nil {
		plan.Plugins = append(plan.Plugins, PlannedPlugin{Type: node.Type, Name: node.Name, Position: node.Position})
		switch node.Type {
		case "empty":
			node = node.Slave
		case "asym":
			if stream == alsa.Playback {
				node = node.Playback
			} else {
				node = node.Capture
			}
		case "plug":
			plan.RequiresPlug = true
			node = node.Slave
		case "hw":
			definition := alsaconfig.PCM{Name: node.Name, Type: node.Type, Position: node.Position, Fields: node.Fields}
			selector, selectorErr := definition.HardwareSelector()
			if selectorErr != nil {
				return NamedPlan{}, selectorErr
			}
			plan.HardwareSelector = selector
			return plan, nil
		default:
			return NamedPlan{}, &UnsupportedPluginError{Plugin: node.Type, Node: node.Name, Position: node.Position}
		}
	}
	return NamedPlan{}, fmt.Errorf("alsa pcm: named graph %q has no endpoint for %s", name, stream)
}

// Converter builds the planned plug stage after hardware negotiation. A direct
// graph returns nil when parameters already match and ErrConversionRequired
// when they differ. Matrix is optional and follows PlugConfig semantics.
func (p NamedPlan) Converter(application, hardware ConversionParams, matrix []int32) (*PlugConverter, error) {
	if !p.RequiresPlug {
		if application != hardware {
			return nil, ErrConversionRequired
		}
		return nil, nil
	}
	return NewPlugConverter(PlugConfig{Application: application, Hardware: hardware, Matrix: matrix}, p.Stream)
}
