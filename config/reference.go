package config

import (
	"fmt"
	"strings"
)

const maximumReferenceDepth = 64

// ResolveReference resolves one path through direct path-valued assignments
// and explicit {@func refer; name path} compounds. Other functions and
// variable interpolation are deliberately not executed.
func (d *Document) ResolveReference(name string) (Value, error) {
	return d.resolveReference(name, map[string]bool{}, 0)
}

func (d *Document) resolveReference(name string, active map[string]bool, depth int) (Value, error) {
	if depth > maximumReferenceDepth {
		return Value{}, fmt.Errorf("alsa config: reference nesting exceeds %d", maximumReferenceDepth)
	}
	if active[name] {
		return Value{}, fmt.Errorf("alsa config: reference cycle at %q", name)
	}
	active[name] = true
	defer delete(active, name)
	value, err := d.Effective(name)
	if err != nil {
		return Value{}, err
	}
	if target, ok := referTarget(value); ok {
		return d.resolveReference(target, active, depth+1)
	}
	if value.Kind == ValueString && looksLikePath(value.String) {
		if _, exists := d.Lookup(value.String); exists {
			return d.resolveReference(value.String, active, depth+1)
		}
	}
	return value, nil
}

func referTarget(value Value) (string, bool) {
	if value.Kind != ValueCompound {
		return "", false
	}
	var function, name string
	for _, statement := range value.Entries {
		if statement.Assignment == nil || statement.Assignment.Value.Kind != ValueString {
			continue
		}
		switch statement.Assignment.Name {
		case "@func":
			function = statement.Assignment.Value.String
		case "name":
			name = statement.Assignment.Value.String
		}
	}
	return name, function == "refer" && name != ""
}

func looksLikePath(value string) bool {
	return strings.Contains(value, ".") && !strings.ContainsAny(value, "/: \t\r\n")
}
