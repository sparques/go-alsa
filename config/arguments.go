package config

import (
	"fmt"
	"strconv"
	"strings"
)

// PCMName evaluates a PCM invocation with optional runtime arguments, such as
// hw:0,1, hw:CARD=0,DEV=1, or hw:{CARD 0 DEV 1}.
func (e *Evaluator) PCMName(invocation string) (PCM, error) {
	evaluator, name, err := e.bindInvocation("pcm", "PCM", invocation)
	if err != nil {
		return PCM{}, err
	}
	return evaluator.PCM(name)
}

func (e *Evaluator) bindInvocation(namespace, label, invocation string) (*Evaluator, string, error) {
	return e.bindPath(namespace+".", label, invocation)
}

func (e *Evaluator) bindPath(prefix, label, invocation string) (*Evaluator, string, error) {
	name, arguments, hasArguments, err := parseInvocation(invocation, label)
	if err != nil {
		return nil, "", err
	}
	path := name
	if prefix != "" && !strings.HasPrefix(path, prefix) {
		path = prefix + path
	}
	template, err := e.document.Effective(path)
	if err != nil {
		return nil, "", err
	}
	if template.Kind != ValueCompound {
		return nil, "", fmt.Errorf("alsa config: %s %q is not a compound template", label, name)
	}
	order, specs, err := argumentDefinitions(template)
	if err != nil {
		if !hasArguments && !hasArgumentList(template) {
			return e, name, nil
		}
		return nil, "", fmt.Errorf("alsa config: %s %q arguments: %w", label, name, err)
	}
	if len(arguments.positional) > len(order) {
		return nil, "", fmt.Errorf("alsa config: %s %q received %d positional arguments, accepts %d", label, name, len(arguments.positional), len(order))
	}
	provided := make(map[string]Value, len(arguments.named)+len(arguments.positional))
	for index, value := range arguments.positional {
		provided[order[index]] = value
	}
	for key, value := range arguments.named {
		if _, ok := specs[key]; !ok {
			return nil, "", fmt.Errorf("alsa config: %s %q has no argument %q", label, name, key)
		}
		if _, duplicate := provided[key]; duplicate {
			return nil, "", fmt.Errorf("alsa config: %s %q argument %q supplied twice", label, name, key)
		}
		provided[key] = value
	}
	options := e.options
	options.Variables = make(map[string]Value, len(e.options.Variables)+len(order))
	for key, value := range e.options.Variables {
		options.Variables[key] = value
	}
	argumentEvaluator := NewEvaluator(e.document, options)
	for _, key := range order {
		value, ok := provided[key]
		if !ok && !hasArguments {
			value, ok = e.options.Variables[key]
		}
		if !ok {
			fallback, exists := compoundValue(specs[key], "default")
			if !exists {
				return nil, "", fmt.Errorf("alsa config: %s %q argument %q is required", label, name, key)
			}
			value, err = argumentEvaluator.Value(fallback)
			if err != nil {
				return nil, "", fmt.Errorf("alsa config: %s %q default %q: %w", label, name, key, err)
			}
		}
		value, err = coerceArgument(value, specs[key])
		if err != nil {
			return nil, "", fmt.Errorf("alsa config: %s %q argument %q: %w", label, name, key, err)
		}
		argumentEvaluator.options.Variables[key] = value
	}
	return argumentEvaluator, name, nil
}

func hasArgumentList(template Value) bool {
	_, ok := compoundValue(template, "@args")
	return ok
}

type invocationArguments struct {
	positional []Value
	named      map[string]Value
}

func parsePCMInvocation(invocation string) (string, invocationArguments, bool, error) {
	return parseInvocation(invocation, "PCM")
}

func parseInvocation(invocation, label string) (string, invocationArguments, bool, error) {
	name, raw, found := strings.Cut(invocation, ":")
	if name == "" {
		return "", invocationArguments{}, false, fmt.Errorf("alsa config: empty %s name", label)
	}
	if !found {
		return name, invocationArguments{}, false, nil
	}
	if raw == "" {
		return "", invocationArguments{}, false, fmt.Errorf("alsa config: %s %q has an empty argument list", label, name)
	}
	result := invocationArguments{named: map[string]Value{}}
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		value, err := parseInvocationValue(raw)
		if err != nil {
			return "", invocationArguments{}, false, fmt.Errorf("alsa config: %s %q invalid braced arguments: %w", label, name, err)
		}
		if value.Kind != ValueCompound {
			return "", invocationArguments{}, false, fmt.Errorf("alsa config: %s %q braced arguments are not a compound", label, name)
		}
		for _, statement := range value.Entries {
			if statement.Assignment == nil {
				continue
			}
			key := statement.Assignment.Name
			if _, duplicate := result.named[key]; duplicate {
				return "", invocationArguments{}, false, fmt.Errorf("alsa config: argument %q supplied twice", key)
			}
			result.named[key] = statement.Assignment.Value
		}
		return name, result, true, nil
	}
	parts, err := splitInvocationArguments(raw)
	if err != nil {
		return "", invocationArguments{}, false, err
	}
	for _, part := range parts {
		key, text, named := strings.Cut(part, "=")
		if named {
			key = strings.TrimSpace(key)
			if key == "" {
				return "", invocationArguments{}, false, fmt.Errorf("alsa config: empty named argument")
			}
			if _, duplicate := result.named[key]; duplicate {
				return "", invocationArguments{}, false, fmt.Errorf("alsa config: argument %q supplied twice", key)
			}
			value, err := parseInvocationValue(text)
			if err != nil {
				return "", invocationArguments{}, false, err
			}
			result.named[key] = value
			continue
		}
		value, err := parseInvocationValue(part)
		if err != nil {
			return "", invocationArguments{}, false, err
		}
		result.positional = append(result.positional, value)
	}
	return name, result, true, nil
}

func splitInvocationArguments(input string) ([]string, error) {
	var result []string
	start, depth := 0, 0
	var quote byte
	escaped := false
	for index := 0; index < len(input); index++ {
		character := input[index]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("alsa config: unbalanced PCM arguments")
			}
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(input[start:index])
				if part == "" {
					return nil, fmt.Errorf("alsa config: empty PCM argument")
				}
				result = append(result, part)
				start = index + 1
			}
		}
	}
	if quote != 0 || depth != 0 {
		return nil, fmt.Errorf("alsa config: unterminated PCM argument")
	}
	part := strings.TrimSpace(input[start:])
	if part == "" {
		return nil, fmt.Errorf("alsa config: empty PCM argument")
	}
	return append(result, part), nil
}

func parseInvocationValue(input string) (Value, error) {
	document, err := Parse("<pcm-arguments>", "value "+strings.TrimSpace(input))
	if err != nil {
		return Value{}, err
	}
	if len(document.Statements) != 1 || document.Statements[0].Assignment == nil {
		return Value{}, fmt.Errorf("alsa config: invalid PCM argument %q", input)
	}
	return document.Statements[0].Assignment.Value, nil
}

func argumentDefinitions(template Value) ([]string, map[string]Value, error) {
	args, ok := compoundValue(template, "@args")
	if !ok || args.Kind != ValueArray {
		return nil, nil, fmt.Errorf("template has no @args array")
	}
	order := make([]string, len(args.Items))
	specs := make(map[string]Value, len(order))
	for index, item := range args.Items {
		if item.Kind != ValueString || item.String == "" {
			return nil, nil, fmt.Errorf("@args item %d is not a name", index)
		}
		order[index] = item.String
		value, ok := compoundValue(template, "@args."+item.String)
		if !ok || value.Kind != ValueCompound {
			return nil, nil, fmt.Errorf("argument %q has no definition", item.String)
		}
		specs[item.String] = value
	}
	return order, specs, nil
}

func coerceArgument(value, specification Value) (Value, error) {
	typeName, ok := compoundString(specification, "type")
	if !ok {
		return Value{}, fmt.Errorf("definition has no type")
	}
	switch typeName {
	case "string":
		text, err := scalarText(value)
		if err != nil {
			return Value{}, err
		}
		value.Kind, value.String = ValueString, text
		return value, nil
	case "integer":
		if value.Kind == ValueInteger {
			return value, nil
		}
		if value.Kind == ValueString {
			integer, err := strconv.ParseInt(value.String, 0, 64)
			if err == nil {
				value.Kind, value.Integer = ValueInteger, integer
				return value, nil
			}
		}
		return Value{}, fmt.Errorf("value is not an integer")
	default:
		return Value{}, fmt.Errorf("unsupported argument type %q", typeName)
	}
}
