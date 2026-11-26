package config

import "fmt"

// Effective applies libasound assignment modes to every definition of one
// canonical path and returns the resulting value.
func (d *Document) Effective(name string) (Value, error) {
	assignments := d.Assignments(name)
	if len(assignments) == 0 {
		return Value{}, fmt.Errorf("alsa config: assignment %q not found", name)
	}
	var result Value
	exists := false
	for _, assignment := range assignments {
		var err error
		result, exists, err = applyValue(result, exists, assignment.Value, assignment.Mode, name)
		if err != nil {
			return Value{}, err
		}
	}
	return result, nil
}

func applyValue(old Value, exists bool, incoming Value, mode AssignmentMode, path string) (Value, bool, error) {
	switch mode {
	case AssignOverride:
		return cloneValue(incoming), true, nil
	case AssignNoOverride:
		if exists {
			return old, true, nil
		}
		return cloneValue(incoming), true, nil
	case AssignMerge:
		if !exists {
			return Value{}, false, fmt.Errorf("alsa config: merge-only assignment %q has no existing node", path)
		}
	case AssignMergeCreate:
		if !exists {
			return cloneValue(incoming), true, nil
		}
	default:
		return Value{}, false, fmt.Errorf("alsa config: assignment %q has invalid mode %d", path, mode)
	}
	merged, err := mergeValues(old, incoming, path)
	return merged, true, err
}

func mergeValues(old, incoming Value, path string) (Value, error) {
	if old.Kind != incoming.Kind {
		return Value{}, fmt.Errorf("alsa config: cannot merge %q values of kinds %d and %d", path, old.Kind, incoming.Kind)
	}
	if old.Kind != ValueCompound {
		return cloneValue(incoming), nil
	}
	result := cloneValue(old)
	for _, statement := range incoming.Entries {
		if statement.Assignment == nil {
			result.Entries = append(result.Entries, cloneStatement(statement))
			continue
		}
		name := statement.Assignment.Name
		index := lastDirectAssignment(result.Entries, name)
		exists := index >= 0
		var current Value
		if exists {
			current = result.Entries[index].Assignment.Value
		}
		value, keep, err := applyValue(current, exists, statement.Assignment.Value, statement.Assignment.Mode, path+"."+name)
		if err != nil {
			return Value{}, err
		}
		if !keep {
			continue
		}
		clone := cloneStatement(statement)
		clone.Assignment.Value = value
		if exists {
			result.Entries[index] = clone
		} else {
			result.Entries = append(result.Entries, clone)
		}
	}
	return result, nil
}

func lastDirectAssignment(statements []Statement, name string) int {
	for index := len(statements) - 1; index >= 0; index-- {
		if statements[index].Assignment != nil && statements[index].Assignment.Name == name {
			return index
		}
	}
	return -1
}

func cloneStatement(statement Statement) Statement {
	if statement.Assignment != nil {
		assignment := *statement.Assignment
		assignment.Path = append([]string(nil), assignment.Path...)
		assignment.Value = cloneValue(assignment.Value)
		statement.Assignment = &assignment
	}
	return statement
}

func cloneValue(value Value) Value {
	value.Items = append([]Value(nil), value.Items...)
	for index := range value.Items {
		value.Items[index] = cloneValue(value.Items[index])
	}
	value.Entries = append([]Statement(nil), value.Entries...)
	for index := range value.Entries {
		value.Entries[index] = cloneStatement(value.Entries[index])
	}
	return value
}
