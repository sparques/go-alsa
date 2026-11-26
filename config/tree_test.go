package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTreeAppliesHooksCardsAndOverrides(t *testing.T) {
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("alsa.conf", `value system
@hooks [ { func load files [ { @func concat strings [ { @func datadir } "/base.conf" ] } ] } ]
cards.@hooks [ { func load_for_all_cards files [ { @func concat strings [ { @func datadir } "/cards/" { @func private_string } ".conf" ] } ] } ]`)
	write("base.conf", "loaded true")
	write("cards/USB-Audio.conf", "USB-Audio.present true")
	result, err := LoadTree(TreeOptions{
		Root: root, Main: "alsa.conf",
		Eval:           EvalOptions{Cards: []HookCard{{Number: 3, Driver: "USB-Audio"}}},
		OverrideLayers: []Layer{{Source: "user.conf", Contents: "!value user"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SkippedHooks) != 0 {
		t.Fatalf("skipped hooks = %v", result.SkippedHooks)
	}
	evaluator := NewEvaluator(result.Document, EvalOptions{})
	for name := range map[string]bool{"loaded": true, "cards.USB-Audio.present": true} {
		value, err := evaluator.Reference(name)
		if err != nil || !value.Boolean {
			t.Errorf("Reference(%q) = %#v, %v", name, value, err)
		}
	}
	value, err := evaluator.Reference("value")
	if err != nil || value.String != "user" || value.Position.Source != "user.conf" {
		t.Fatalf("override value = %#v, %v", value, err)
	}
	alias, err := result.Document.Effective("cards.3")
	if err != nil || alias.String != "cards.USB-Audio" {
		t.Fatalf("card alias = %#v, %v", alias, err)
	}
}

func TestLoadTreeRequiresExplicitPaths(t *testing.T) {
	if _, err := LoadTree(TreeOptions{}); err == nil {
		t.Fatal("LoadTree accepted empty paths")
	}
}
