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
// direct field. Slave identifies the common slave.pcm edge when present.
// Unknown plugin types and fields are retained so callers can reject them
// explicitly instead of silently changing behavior.
type PCMNode struct {
	Name     string
	Type     string
	Position Position
	Fields   map[string]Value
	Slave    *PCMNode
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
	}
	slave, exists := fields["slave.pcm"]
	if !exists {
		return node, nil
	}
	var err error
	switch slave.Kind {
	case ValueString:
		if slave.String == "" {
			return nil, fmt.Errorf("alsa config: PCM graph node %q has an empty slave.pcm", name)
		}
		node.Slave, err = e.graphInvocation(slave.String, chain, active, depth+1)
	case ValueCompound:
		node.Slave, err = e.graphCompound(name+".slave.pcm", slave, chain, active, depth+1)
	default:
		return nil, fmt.Errorf("alsa config: PCM graph node %q has non-PCM slave.pcm", name)
	}
	if err != nil {
		return nil, err
	}
	return node, nil
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
