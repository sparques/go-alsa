package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadHooksAppliesOnlyDeclarativeLoads(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "aliases.conf"), []byte("alias.value 7"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "USB-Audio.conf"), []byte("cards.USB-Audio.loaded true"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := Parse("base", `cards.@hooks [
 { func load files [ { @func concat strings [ { @func datadir } "/aliases.conf" ] } ] }
 { func load_for_all_cards files [ { @func concat strings [ { @func datadir } "/" { @func private_string } ".conf" ] } ] }
]`)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := LoadHooks(document, "cards.@hooks", EvalOptions{DataDir: root, Cards: []HookCard{{Number: 3, Driver: "USB-Audio"}}}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Skipped, []string(nil)) {
		t.Fatalf("skipped = %v", result.Skipped)
	}
	value, err := NewEvaluator(result.Document, EvalOptions{}).Reference("alias.value")
	if err != nil || value.Integer != 7 {
		t.Fatalf("loaded alias = %#v, %v", value, err)
	}
	value, err = NewEvaluator(result.Document, EvalOptions{}).Reference("cards.USB-Audio.loaded")
	if err != nil || value.Kind != ValueBoolean || !value.Boolean {
		t.Fatalf("loaded card file = %#v, %v", value, err)
	}
	value, err = result.Document.Effective("cards.3")
	if err != nil || value.String != "cards.USB-Audio" {
		t.Fatalf("numeric card alias = %#v, %v", value, err)
	}
}
