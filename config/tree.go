package config

import (
	"errors"
	"fmt"
	"path/filepath"
)

// TreeOptions describes an explicit, confined ALSA configuration tree. Main
// may be relative to Root or an absolute path inside Root. OverrideLayers are
// applied after system includes and safe hooks.
type TreeOptions struct {
	Root           string
	Main           string
	OverrideLayers []Layer
	Eval           EvalOptions
}

// TreeResult contains the loaded document and any unknown hook functions that
// were deliberately not executed.
type TreeResult struct {
	Document     *Document
	SkippedHooks []string
}

// LoadTree loads one explicitly rooted ALSA tree, expands includes, applies
// declarative top-level and per-card hooks, then appends ordered override
// layers. It never discovers host paths, environment, users, or cards.
func LoadTree(options TreeOptions) (TreeResult, error) {
	if options.Root == "" || options.Main == "" {
		return TreeResult{}, fmt.Errorf("alsa config: tree root and main file are required")
	}
	resolver, err := NewDirectoryResolver(options.Root)
	if err != nil {
		return TreeResult{}, err
	}
	mainSource, contents, err := resolver("", options.Main)
	if err != nil {
		return TreeResult{}, fmt.Errorf("alsa config: load main file %q: %w", options.Main, err)
	}
	document, err := Load(mainSource, contents, resolver)
	if err != nil {
		return TreeResult{}, err
	}
	realRoot, err := filepath.EvalSymlinks(options.Root)
	if err != nil {
		return TreeResult{}, fmt.Errorf("alsa config: resolve tree root: %w", err)
	}
	eval := options.Eval
	if eval.DataDir == "" {
		eval.DataDir = realRoot
	}
	var skipped []string
	for _, hookPath := range []string{"@hooks", "cards.@hooks"} {
		if _, hookErr := document.Effective(hookPath); hookErr != nil {
			if errors.Is(hookErr, ErrNotFound) {
				continue
			}
			return TreeResult{}, hookErr
		}
		loaded, hookErr := LoadHooks(document, hookPath, eval, resolver)
		if hookErr != nil {
			return TreeResult{}, hookErr
		}
		document = loaded.Document
		for _, function := range loaded.Skipped {
			skipped = append(skipped, hookPath+":"+function)
		}
	}
	if len(options.OverrideLayers) != 0 {
		overrides, loadErr := LoadLayers(options.OverrideLayers, resolver)
		if loadErr != nil {
			return TreeResult{}, loadErr
		}
		document.Statements = append(document.Statements, overrides.Statements...)
	}
	return TreeResult{Document: document, SkippedHooks: skipped}, nil
}
