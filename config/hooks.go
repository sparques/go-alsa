package config

import (
	"fmt"
	"strconv"
	"strings"
)

// HookCard is caller-supplied metadata used by load_for_all_cards. Driver is
// the ALSA configuration basename, such as "USB-Audio".
type HookCard struct {
	Number int64
	Driver string
}

// HookLoadResult reports declarative hooks that were applied and hook
// functions that were intentionally not executed.
type HookLoadResult struct {
	Document *Document
	Skipped  []string
}

// LoadHooks applies safe declarative load and load_for_all_cards entries in one
// hook array. Every read goes through resolver. Other functions are reported
// in Skipped and are never executed implicitly.
func LoadHooks(document *Document, hookPath string, options EvalOptions, resolver IncludeResolver) (HookLoadResult, error) {
	if document == nil {
		return HookLoadResult{}, fmt.Errorf("alsa config: hook loader has no document")
	}
	hooks, err := document.Effective(hookPath)
	if err != nil {
		return HookLoadResult{}, err
	}
	if hooks.Kind != ValueArray {
		return HookLoadResult{}, fmt.Errorf("alsa config: hook %q is not an array", hookPath)
	}
	result := &Document{Statements: append([]Statement(nil), document.Statements...)}
	loader := hookLoader{path: hookPath, resolver: resolver, result: result,
		state: loadState{resolver: resolver, active: map[string]bool{}}, loaded: map[string]bool{}}
	var skipped []string
	for index, hook := range hooks.Items {
		if hook.Kind != ValueCompound {
			return HookLoadResult{}, fmt.Errorf("alsa config: hook %q item %d is not a compound", hookPath, index)
		}
		function, ok := compoundString(hook, "func")
		if !ok || function == "" {
			return HookLoadResult{}, fmt.Errorf("alsa config: hook %q item %d has no function", hookPath, index)
		}
		switch function {
		case "load":
			if err := loader.loadFiles(hook, NewEvaluator(document, options)); err != nil {
				return HookLoadResult{}, err
			}
		case "load_for_all_cards":
			root := strings.TrimSuffix(hookPath, ".@hooks")
			for _, card := range options.Cards {
				if card.Number < 0 || card.Driver == "" {
					return HookLoadResult{}, fmt.Errorf("alsa config: invalid hook card %#v", card)
				}
				cardOptions := options
				cardOptions.privateString = &card.Driver
				cardOptions.privateInteger = &card.Number
				firstLoaded := len(loader.result.Statements)
				if err := loader.loadFiles(hook, NewEvaluator(document, cardOptions)); err != nil {
					return HookLoadResult{}, err
				}
				prefixStatements(loader.result.Statements[firstLoaded:], root)
				name := root + "." + strconv.FormatInt(card.Number, 10)
				value := root + "." + card.Driver
				loader.result.Statements = append(loader.result.Statements, Statement{Position: hook.Position,
					Assignment: &Assignment{Name: name, Path: strings.Split(name, "."), Mode: AssignOverride,
						Position: hook.Position, Value: Value{Kind: ValueString, Position: hook.Position, String: value}}})
			}
		default:
			skipped = append(skipped, function)
		}
	}
	return HookLoadResult{Document: result, Skipped: skipped}, nil
}

func prefixStatements(statements []Statement, root string) {
	if root == "" {
		return
	}
	for index := range statements {
		assignment := statements[index].Assignment
		if assignment == nil {
			continue
		}
		assignment.Name = root + "." + assignment.Name
		assignment.Path = append([]string{root}, assignment.Path...)
	}
}

type hookLoader struct {
	path     string
	resolver IncludeResolver
	result   *Document
	state    loadState
	loaded   map[string]bool
}

func (l *hookLoader) loadFiles(hook Value, evaluator *Evaluator) error {
	files, ok := compoundValue(hook, "files")
	if !ok || files.Kind != ValueArray {
		return fmt.Errorf("alsa config: load hook %q has no files array", l.path)
	}
	ignoreErrors := false
	if value, exists := compoundValue(hook, "errors"); exists && value.Kind == ValueBoolean {
		ignoreErrors = !value.Boolean
	}
	for _, file := range files.Items {
		evaluated, err := evaluator.Value(file)
		if err != nil {
			if ignoreErrors {
				continue
			}
			return fmt.Errorf("alsa config: load hook %q file: %w", l.path, err)
		}
		specification, err := scalarText(evaluated)
		if err != nil || specification == "" {
			if ignoreErrors {
				continue
			}
			return fmt.Errorf("alsa config: load hook %q has a non-string file", l.path)
		}
		if err := l.load(hook.Position.Source, specification, ignoreErrors); err != nil {
			return err
		}
	}
	return nil
}

func (l *hookLoader) load(from, specification string, ignoreErrors bool) error {
	if l.resolver == nil {
		return fmt.Errorf("alsa config: load hook %q has no resolver", l.path)
	}
	source, contents, err := l.resolver(from, specification)
	if err != nil {
		if ignoreErrors {
			return nil
		}
		return fmt.Errorf("alsa config: load hook %q file %q: %w", l.path, specification, err)
	}
	if l.loaded[source] {
		return nil
	}
	l.loaded[source] = true
	l.state.bytes += len(contents)
	if l.state.bytes > maximumAggregateBytes {
		return fmt.Errorf("alsa config: hook input exceeds 64 MiB")
	}
	parsed, err := Parse(source, contents)
	if err != nil {
		return err
	}
	l.state.active[source] = true
	expanded, err := l.state.expand(source, parsed.Statements, 0)
	delete(l.state.active, source)
	if err != nil {
		return err
	}
	l.result.Statements = append(l.result.Statements, expanded...)
	return nil
}
