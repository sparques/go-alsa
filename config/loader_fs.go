package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NewDirectoryResolver returns an include resolver confined to root. The
// resolver accepts ALSA's confdir:path form and ordinary relative paths.
func NewDirectoryResolver(root string) (IncludeResolver, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("alsa config: resolve include root: %w", err)
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, fmt.Errorf("alsa config: inspect include root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("alsa config: include root %q is not a directory", root)
	}
	return func(fromSource, specification string) (string, string, error) {
		name := specification
		base := realRoot
		if value, ok := strings.CutPrefix(name, "confdir:"); ok {
			name = value
		} else if fromSource != "" && filepath.IsAbs(fromSource) {
			base = filepath.Dir(fromSource)
		}
		if name == "" {
			return "", "", fmt.Errorf("include path %q must not be empty", specification)
		}
		candidate := filepath.Clean(name)
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(base, candidate)
		}
		realPath, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", "", err
		}
		relative, err := filepath.Rel(realRoot, realPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("include path %q escapes root %q", specification, realRoot)
		}
		info, err := os.Stat(realPath)
		if err != nil {
			return "", "", err
		}
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("include path %q is not a regular file", specification)
		}
		if info.Size() > maximumConfigBytes {
			return "", "", fmt.Errorf("include path %q exceeds 16 MiB", specification)
		}
		contents, err := os.ReadFile(realPath)
		if err != nil {
			return "", "", err
		}
		return realPath, string(contents), nil
	}, nil
}
