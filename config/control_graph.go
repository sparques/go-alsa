package config

import (
	"fmt"
	"strings"
)

// ControlGraph is a materialized named ALSA control plugin graph.
type ControlGraph struct {
	Name string
	Root *ControlNode
}

// ControlNode is one control plugin. Child represents the routing edge used by
// the standard empty control plugin; unknown fields remain in Fields.
type ControlNode struct {
	Name     string
	Type     string
	Position Position
	Fields   map[string]Value
	Child    *ControlNode
}

// ControlGraph resolves a control invocation into a typed plugin graph.
func (e *Evaluator) ControlGraph(invocation string) (ControlGraph, error) {
	root, err := e.controlGraphInvocation(invocation, nil, map[string]int{}, 0)
	if err != nil {
		return ControlGraph{}, err
	}
	return ControlGraph{Name: invocation, Root: root}, nil
}

func (e *Evaluator) controlGraphInvocation(invocation string, chain []string, active map[string]int, depth int) (*ControlNode, error) {
	if depth > maximumPCMGraphDepth {
		return nil, fmt.Errorf("alsa config: control graph nesting exceeds %d", maximumPCMGraphDepth)
	}
	if index, exists := active[invocation]; exists {
		cycle := append(append([]string(nil), chain[index:]...), invocation)
		return nil, fmt.Errorf("%w: %s", ErrControlGraphCycle, strings.Join(cycle, " -> "))
	}
	active[invocation] = len(chain)
	chain = append(chain, invocation)
	defer delete(active, invocation)
	definition, err := e.ControlName(invocation)
	if err != nil {
		return nil, fmt.Errorf("alsa config: resolve control graph %q: %w", invocation, err)
	}
	return e.controlGraphFields(definition.Name, definition.Position, definition.Fields, chain, active, depth+1)
}

func (e *Evaluator) controlGraphFields(name string, position Position, fields map[string]Value, chain []string, active map[string]int, depth int) (*ControlNode, error) {
	typeValue, ok := fields["type"]
	if !ok || typeValue.Kind != ValueString || typeValue.String == "" {
		return nil, fmt.Errorf("alsa config: control graph node %q has no string type", name)
	}
	node := &ControlNode{Name: name, Type: typeValue.String, Position: position, Fields: cloneFields(fields)}
	child, exists := fields["child"]
	if !exists {
		return node, nil
	}
	var err error
	switch child.Kind {
	case ValueString:
		if child.String == "" {
			return nil, fmt.Errorf("alsa config: control graph node %q has an empty child", name)
		}
		node.Child, err = e.controlGraphInvocation(child.String, chain, active, depth+1)
	case ValueCompound:
		childFields := make(map[string]Value)
		for _, statement := range child.Entries {
			if statement.Assignment != nil {
				childFields[statement.Assignment.Name] = statement.Assignment.Value
			}
		}
		node.Child, err = e.controlGraphFields(name+".child", child.Position, childFields, chain, active, depth+1)
	default:
		return nil, fmt.Errorf("alsa config: control graph node %q has a non-control child", name)
	}
	if err != nil {
		return nil, err
	}
	return node, nil
}
