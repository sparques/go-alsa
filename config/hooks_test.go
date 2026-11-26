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
	document, err := Parse("base", `@hooks [
 { func load files [ { @func concat strings [ { @func datadir } "/aliases.conf" ] } ] }
 { func load_for_all_cards files [ ignored ] }
]`)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := LoadHooks(document, "@hooks", EvalOptions{DataDir: root}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Skipped, []string{"load_for_all_cards"}) {
		t.Fatalf("skipped = %v", result.Skipped)
	}
	value, err := NewEvaluator(result.Document, EvalOptions{}).Reference("alias.value")
	if err != nil || value.Integer != 7 {
		t.Fatalf("loaded alias = %#v, %v", value, err)
	}
}
