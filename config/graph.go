package config

import (
	"fmt"
	"strings"
)

const maximumPCMGraphDepth = 64

// PCMGraph is a materialized named PCM plugin graph. It describes configured
// topology only; plugin execution belongs to the PCM graph engine.
type PCMGraph struct {
	Name string
	Root *PCMNode
}

// PCMNode is one configured PCM plugin. Fields preserves every evaluated
// direct field. Slave, Playback, and Capture identify common PCM edges; Edges
// contains the same nodes keyed by their configuration field names.
// Unknown plugin types and fields are retained so callers can reject them
// explicitly instead of silently changing behavior.
type PCMNode struct {
	Name     string
	Type     string
	Position Position
	Fields   map[string]Value
	Slave    *PCMNode
	Playback *PCMNode
	Capture  *PCMNode
	Edges    map[string]*PCMNode
}

// Graph resolves a PCM invocation into a typed plugin graph. Named slave.pcm
// values may themselves include runtime arguments.
func (e *Evaluator) Graph(invocation string) (PCMGraph, error) {
	root, err := e.graphInvocation(invocation, nil, map[string]int{}, 0)
	if err != nil {
		return PCMGraph{}, err
	}
	return PCMGraph{Name: invocation, Root: root}, nil
}

func (e *Evaluator) graphInvocation(invocation string, chain []string, active map[string]int, depth int) (*PCMNode, error) {
	if depth > maximumPCMGraphDepth {
		return nil, fmt.Errorf("alsa config: PCM graph nesting exceeds %d", maximumPCMGraphDepth)
	}
	if index, exists := active[invocation]; exists {
		cycle := append(append([]string(nil), chain[index:]...), invocation)
		return nil, fmt.Errorf("%w: %s", ErrGraphCycle, strings.Join(cycle, " -> "))
	}
	active[invocation] = len(chain)
	chain = append(chain, invocation)
	defer delete(active, invocation)

	definition, err := e.PCMName(invocation)
	if err != nil {
		return nil, fmt.Errorf("alsa config: resolve PCM graph %q: %w", invocation, err)
	}
	return e.graphFields(definition.Name, definition.Position, definition.Fields, chain, active, depth+1)
}

func (e *Evaluator) graphFields(name string, position Position, fields map[string]Value, chain []string, active map[string]int, depth int) (*PCMNode, error) {
	typeValue, ok := fields["type"]
	if !ok || typeValue.Kind != ValueString || typeValue.String == "" {
		return nil, fmt.Errorf("alsa config: PCM graph node %q has no string type", name)
	}
	node := &PCMNode{
		Name:     name,
		Type:     typeValue.String,
		Position: position,
		Fields:   cloneFields(fields),
		Edges:    make(map[string]*PCMNode),
	}
	for _, edgeName := range []string{"slave.pcm", "playback.pcm", "capture.pcm"} {
		value, exists := fields[edgeName]
		if !exists {
			continue
		}
		edge, err := e.graphEdge(name, edgeName, value, chain, active, depth+1)
		if err != nil {
			return nil, err
		}
		node.Edges[edgeName] = edge
		switch edgeName {
		case "slave.pcm":
			node.Slave = edge
		case "playback.pcm":
			node.Playback = edge
		case "capture.pcm":
			node.Capture = edge
		}
	}
	return node, nil
}

func (e *Evaluator) graphEdge(name, edgeName string, value Value, chain []string, active map[string]int, depth int) (*PCMNode, error) {
	switch value.Kind {
	case ValueString:
		if value.String == "" {
			return nil, fmt.Errorf("alsa config: PCM graph node %q has an empty %s", name, edgeName)
		}
		return e.graphInvocation(value.String, chain, active, depth+1)
	case ValueCompound:
		return e.graphCompound(name+"."+edgeName, value, chain, active, depth+1)
	default:
		return nil, fmt.Errorf("alsa config: PCM graph node %q has non-PCM %s", name, edgeName)
	}
}

func (e *Evaluator) graphCompound(name string, value Value, chain []string, active map[string]int, depth int) (*PCMNode, error) {
	if depth > maximumPCMGraphDepth {
		return nil, fmt.Errorf("alsa config: PCM graph nesting exceeds %d", maximumPCMGraphDepth)
	}
	fields := make(map[string]Value)
	for _, statement := range value.Entries {
		if statement.Assignment != nil {
			fields[statement.Assignment.Name] = statement.Assignment.Value
		}
	}
	return e.graphFields(name, value.Position, fields, chain, active, depth+1)
}

func cloneFields(fields map[string]Value) map[string]Value {
	result := make(map[string]Value, len(fields))
	for name, value := range fields {
		result[name] = cloneValue(value)
	}
	return result
}
