package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maximumEvaluationDepth = 64
const maximumEvaluatedString = 1 << 20

// EvalOptions supplies values that configuration functions may observe.
// A nil LookupEnv exposes no process environment.
type EvalOptions struct {
	Variables map[string]Value
	LookupEnv func(name string) (value string, ok bool)
	// DataDir is returned by the pure datadir function. An empty value leaves
	// that function unavailable rather than consulting the host implicitly.
	DataDir string
	// CardNumber resolves a stable ALSA card ID for functions such as
	// card_inum. Numeric card selectors do not require this callback.
	CardNumber func(card string) (number int64, err error)
}

// Evaluator evaluates pure configuration values against one loaded document.
type Evaluator struct {
	document *Document
	options  EvalOptions
}

func NewEvaluator(document *Document, options EvalOptions) *Evaluator {
	variables := make(map[string]Value, len(options.Variables))
	for name, value := range options.Variables {
		variables[name] = value
	}
	options.Variables = variables
	return &Evaluator{document: document, options: options}
}

// Reference evaluates the last assignment to one canonical path.
func (e *Evaluator) Reference(name string) (Value, error) {
	return e.reference(name, map[string]bool{}, 0)
}

// Value evaluates one parsed value without executing external hooks.
func (e *Evaluator) Value(value Value) (Value, error) {
	return e.value(value, map[string]bool{}, 0)
}

func (e *Evaluator) reference(name string, active map[string]bool, depth int) (Value, error) {
	if e == nil || e.document == nil {
		return Value{}, fmt.Errorf("alsa config: evaluator has no document")
	}
	if depth > maximumEvaluationDepth {
		return Value{}, fmt.Errorf("alsa config: evaluation nesting exceeds %d", maximumEvaluationDepth)
	}
	if active[name] {
		return Value{}, fmt.Errorf("alsa config: evaluation cycle at %q", name)
	}
	active[name] = true
	defer delete(active, name)
	value, err := e.document.Effective(name)
	if err != nil {
		return Value{}, err
	}
	return e.value(value, active, depth+1)
}

func (e *Evaluator) value(value Value, active map[string]bool, depth int) (Value, error) {
	if depth > maximumEvaluationDepth {
		return Value{}, fmt.Errorf("alsa config: evaluation nesting exceeds %d", maximumEvaluationDepth)
	}
	switch value.Kind {
	case ValueString:
		if len(value.String) > maximumEvaluatedString {
			return Value{}, fmt.Errorf("alsa config: evaluated string exceeds 1 MiB")
		}
		if strings.HasPrefix(value.String, "$") && len(value.String) > 1 && !strings.HasPrefix(value.String, "${") {
			variable, ok := e.options.Variables[value.String[1:]]
			if !ok {
				return Value{}, fmt.Errorf("alsa config: variable %q not provided", value.String[1:])
			}
			return e.value(variable, active, depth+1)
		}
		if looksLikePath(value.String) {
			if _, ok := e.document.Lookup(value.String); ok {
				return e.reference(value.String, active, depth+1)
			}
		}
		return value, nil
	case ValueArray:
		items := make([]Value, len(value.Items))
		for index := range value.Items {
			item, err := e.value(value.Items[index], active, depth+1)
			if err != nil {
				return Value{}, err
			}
			items[index] = item
		}
		value.Items = items
		return value, nil
	case ValueCompound:
		function, ok := compoundString(value, "@func")
		if ok {
			return e.function(function, value, active, depth+1)
		}
		entries := make([]Statement, len(value.Entries))
		copy(entries, value.Entries)
		for index := range entries {
			if entries[index].Assignment == nil {
				continue
			}
			assignment := *entries[index].Assignment
			evaluated, err := e.value(assignment.Value, active, depth+1)
			if err != nil {
				return Value{}, err
			}
			assignment.Value = evaluated
			entries[index].Assignment = &assignment
		}
		value.Entries = entries
		return value, nil
	default:
		return value, nil
	}
}

func (e *Evaluator) function(name string, compound Value, active map[string]bool, depth int) (Value, error) {
	switch name {
	case "refer":
		nameValue, ok := compoundValue(compound, "name")
		if !ok {
			return Value{}, fmt.Errorf("alsa config: refer requires a name")
		}
		target, err := e.functionString(nameValue, active, depth+1)
		if err != nil {
			return Value{}, fmt.Errorf("alsa config: refer name: %w", err)
		}
		if target == "" {
			return Value{}, fmt.Errorf("alsa config: refer requires a non-empty name")
		}
		result, err := e.reference(target, active, depth+1)
		if err == nil || !errors.Is(err, ErrNotFound) {
			return result, err
		}
		fallback, exists := compoundValue(compound, "default")
		if !exists {
			return Value{}, err
		}
		return e.value(fallback, active, depth+1)
	case "concat":
		stringsValue, ok := compoundValue(compound, "strings")
		if !ok || stringsValue.Kind != ValueArray {
			return Value{}, fmt.Errorf("alsa config: concat requires a strings array")
		}
		var builder strings.Builder
		for _, item := range stringsValue.Items {
			evaluated, err := e.value(item, active, depth+1)
			if err != nil {
				return Value{}, err
			}
			text, err := scalarText(evaluated)
			if err != nil {
				return Value{}, fmt.Errorf("alsa config: concat: %w", err)
			}
			if builder.Len()+len(text) > maximumEvaluatedString {
				return Value{}, fmt.Errorf("alsa config: evaluated string exceeds 1 MiB")
			}
			builder.WriteString(text)
		}
		return Value{Kind: ValueString, Position: compound.Position, String: builder.String()}, nil
	case "getenv", "igetenv":
		return e.getenv(name == "igetenv", compound, active, depth+1)
	case "card_inum":
		card, ok := compoundValue(compound, "card")
		if !ok {
			return Value{}, fmt.Errorf("alsa config: card_inum requires a card")
		}
		evaluated, err := e.value(card, active, depth+1)
		if err != nil {
			return Value{}, fmt.Errorf("alsa config: card_inum card: %w", err)
		}
		selector, err := scalarText(evaluated)
		if err != nil || selector == "" {
			return Value{}, fmt.Errorf("alsa config: card_inum card is not a selector")
		}
		number, parseErr := strconv.ParseInt(selector, 10, 64)
		if parseErr != nil {
			if e.options.CardNumber == nil {
				return Value{}, fmt.Errorf("alsa config: card_inum cannot resolve card %q", selector)
			}
			number, err = e.options.CardNumber(selector)
			if err != nil {
				return Value{}, fmt.Errorf("alsa config: card_inum resolve card %q: %w", selector, err)
			}
		}
		if number < 0 {
			return Value{}, fmt.Errorf("alsa config: card_inum resolved negative card %d", number)
		}
		return Value{Kind: ValueInteger, Position: compound.Position, Integer: number}, nil
	case "datadir":
		if e.options.DataDir == "" {
			return Value{}, fmt.Errorf("alsa config: datadir was not provided")
		}
		if len(e.options.DataDir) > maximumEvaluatedString {
			return Value{}, fmt.Errorf("alsa config: datadir exceeds 1 MiB")
		}
		return Value{Kind: ValueString, Position: compound.Position, String: e.options.DataDir}, nil
	default:
		return Value{}, fmt.Errorf("alsa config: unsupported function %q", name)
	}
}

func (e *Evaluator) functionString(value Value, active map[string]bool, depth int) (string, error) {
	// A literal dotted string in a function argument is a path name, not the
	// value stored at that path. Variables and structured functions still need
	// evaluation before conversion to text.
	if value.Kind == ValueString && !strings.HasPrefix(value.String, "$") {
		return value.String, nil
	}
	evaluated, err := e.value(value, active, depth+1)
	if err != nil {
		return "", err
	}
	return scalarText(evaluated)
}

func (e *Evaluator) getenv(integer bool, compound Value, active map[string]bool, depth int) (Value, error) {
	vars, ok := compoundValue(compound, "vars")
	if !ok || vars.Kind != ValueArray {
		return Value{}, fmt.Errorf("alsa config: getenv requires a vars array")
	}
	if e.options.LookupEnv != nil {
		for _, item := range vars.Items {
			name, err := scalarText(item)
			if err != nil {
				return Value{}, fmt.Errorf("alsa config: getenv variable: %w", err)
			}
			if text, found := e.options.LookupEnv(name); found {
				return environmentValue(text, integer, compound.Position)
			}
		}
	}
	if fallback, ok := compoundValue(compound, "default"); ok {
		return e.value(fallback, active, depth+1)
	}
	return Value{}, fmt.Errorf("alsa config: no requested environment variable is set and no default exists")
}

func environmentValue(text string, integer bool, position Position) (Value, error) {
	if len(text) > maximumEvaluatedString {
		return Value{}, fmt.Errorf("alsa config: environment value exceeds 1 MiB")
	}
	if !integer {
		return Value{Kind: ValueString, Position: position, String: text}, nil
	}
	value, err := strconv.ParseInt(text, 0, 64)
	if err != nil {
		return Value{}, fmt.Errorf("alsa config: environment value %q is not an integer", text)
	}
	return Value{Kind: ValueInteger, Position: position, Integer: value}, nil
}

func compoundValue(compound Value, name string) (Value, bool) {
	for index := len(compound.Entries) - 1; index >= 0; index-- {
		assignment := compound.Entries[index].Assignment
		if assignment != nil && assignment.Name == name {
			return assignment.Value, true
		}
	}
	return Value{}, false
}

func compoundString(compound Value, name string) (string, bool) {
	value, ok := compoundValue(compound, name)
	return value.String, ok && value.Kind == ValueString
}

func scalarText(value Value) (string, error) {
	switch value.Kind {
	case ValueString:
		return value.String, nil
	case ValueInteger:
		return strconv.FormatInt(value.Integer, 10), nil
	case ValueReal:
		return strconv.FormatFloat(value.Real, 'g', -1, 64), nil
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean), nil
	default:
		return "", fmt.Errorf("value is not scalar")
	}
}
