package config

import "fmt"

const maximumIncludeDepth = 32
const maximumAggregateBytes = 64 << 20

// IncludeResolver maps one include specification to a stable source identity
// and its contents. The caller owns filesystem/search-path policy.
type IncludeResolver func(fromSource, specification string) (source, contents string, err error)

// Load parses input and recursively expands includes in statement order.
// Includes require an injected resolver; no path is opened implicitly.
func Load(source, input string, resolver IncludeResolver) (*Document, error) {
	state := loadState{resolver: resolver, active: map[string]bool{}, bytes: len(input)}
	if state.bytes > maximumAggregateBytes {
		return nil, fmt.Errorf("alsa config: aggregate input exceeds 64 MiB")
	}
	state.active[source] = true
	document, err := Parse(source, input)
	if err != nil {
		return nil, err
	}
	statements, err := state.expand(source, document.Statements, 0)
	if err != nil {
		return nil, err
	}
	document.Statements = statements
	return document, nil
}

type loadState struct {
	resolver IncludeResolver
	active   map[string]bool
	bytes    int
}

func (s *loadState) expand(source string, statements []Statement, depth int) ([]Statement, error) {
	if depth > maximumIncludeDepth {
		return nil, fmt.Errorf("alsa config: include nesting exceeds %d", maximumIncludeDepth)
	}
	result := make([]Statement, 0, len(statements))
	for _, statement := range statements {
		if statement.Include == "" {
			if statement.Assignment != nil {
				value, err := s.expandValue(source, statement.Assignment.Value, depth)
				if err != nil {
					return nil, err
				}
				statement.Assignment.Value = value
			}
			result = append(result, statement)
			continue
		}
		if s.resolver == nil {
			return nil, fmt.Errorf("alsa config: %s:%d:%d: include %q has no resolver", source, statement.Position.Line, statement.Position.Column, statement.Include)
		}
		includedSource, contents, err := s.resolver(source, statement.Include)
		if err != nil {
			return nil, fmt.Errorf("alsa config: %s:%d:%d: include %q: %w", source, statement.Position.Line, statement.Position.Column, statement.Include, err)
		}
		if includedSource == "" {
			return nil, fmt.Errorf("alsa config: include %q resolved to an empty source identity", statement.Include)
		}
		if s.active[includedSource] {
			return nil, fmt.Errorf("alsa config: include cycle from %q to %q", source, includedSource)
		}
		s.bytes += len(contents)
		if s.bytes > maximumAggregateBytes {
			return nil, fmt.Errorf("alsa config: aggregate input exceeds 64 MiB")
		}
		document, err := Parse(includedSource, contents)
		if err != nil {
			return nil, err
		}
		s.active[includedSource] = true
		expanded, err := s.expand(includedSource, document.Statements, depth+1)
		delete(s.active, includedSource)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}
	return result, nil
}

func (s *loadState) expandValue(source string, value Value, depth int) (Value, error) {
	switch value.Kind {
	case ValueCompound:
		entries, err := s.expand(source, value.Entries, depth)
		if err != nil {
			return Value{}, err
		}
		value.Entries = entries
	case ValueArray:
		for index := range value.Items {
			expanded, err := s.expandValue(source, value.Items[index], depth)
			if err != nil {
				return Value{}, err
			}
			value.Items[index] = expanded
		}
	}
	return value, nil
}
