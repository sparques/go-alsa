package config

import "fmt"

// HookLoadResult reports declarative load hooks that were applied and hook
// functions that were intentionally not executed.
type HookLoadResult struct {
	Document *Document
	Skipped  []string
}

// LoadHooks applies the safe, declarative func=load entries in one hook array.
// File names and concat/getenv/datadir expressions are evaluated with options,
// but every read still goes through resolver. Other hook functions are listed
// in Skipped; they are never executed implicitly.
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
	state := loadState{resolver: resolver, active: map[string]bool{}}
	evaluator := NewEvaluator(document, options)
	loaded := map[string]bool{}
	var skipped []string
	for index, hook := range hooks.Items {
		if hook.Kind != ValueCompound {
			return HookLoadResult{}, fmt.Errorf("alsa config: hook %q item %d is not a compound", hookPath, index)
		}
		function, ok := compoundString(hook, "func")
		if !ok || function == "" {
			return HookLoadResult{}, fmt.Errorf("alsa config: hook %q item %d has no function", hookPath, index)
		}
		if function != "load" {
			skipped = append(skipped, function)
			continue
		}
		files, ok := compoundValue(hook, "files")
		if !ok || files.Kind != ValueArray {
			return HookLoadResult{}, fmt.Errorf("alsa config: load hook %q item %d has no files array", hookPath, index)
		}
		ignoreErrors := false
		if errorsValue, exists := compoundValue(hook, "errors"); exists && errorsValue.Kind == ValueBoolean {
			ignoreErrors = !errorsValue.Boolean
		}
		for _, file := range files.Items {
			evaluated, evalErr := evaluator.Value(file)
			if evalErr != nil {
				if ignoreErrors {
					continue
				}
				return HookLoadResult{}, fmt.Errorf("alsa config: load hook %q file: %w", hookPath, evalErr)
			}
			specification, textErr := scalarText(evaluated)
			if textErr != nil || specification == "" {
				if ignoreErrors {
					continue
				}
				return HookLoadResult{}, fmt.Errorf("alsa config: load hook %q has a non-string file", hookPath)
			}
			if resolver == nil {
				return HookLoadResult{}, fmt.Errorf("alsa config: load hook %q has no resolver", hookPath)
			}
			source, contents, resolveErr := resolver(hook.Position.Source, specification)
			if resolveErr != nil {
				if ignoreErrors {
					continue
				}
				return HookLoadResult{}, fmt.Errorf("alsa config: load hook %q file %q: %w", hookPath, specification, resolveErr)
			}
			if loaded[source] {
				continue
			}
			loaded[source] = true
			state.bytes += len(contents)
			if state.bytes > maximumAggregateBytes {
				return HookLoadResult{}, fmt.Errorf("alsa config: hook input exceeds 64 MiB")
			}
			parsed, parseErr := Parse(source, contents)
			if parseErr != nil {
				return HookLoadResult{}, parseErr
			}
			state.active[source] = true
			expanded, expandErr := state.expand(source, parsed.Statements, 0)
			delete(state.active, source)
			if expandErr != nil {
				return HookLoadResult{}, expandErr
			}
			result.Statements = append(result.Statements, expanded...)
		}
	}
	return HookLoadResult{Document: result, Skipped: skipped}, nil
}
