package config

import (
	"errors"
	"fmt"
	"testing"
)

func TestLoadExpandsIncludesInOrder(t *testing.T) {
	files := map[string]string{
		"base":   "first 1\n<child>\nlast 4",
		"child":  "second 2\n<nested>",
		"nested": "third 3",
	}
	resolver := func(_ string, specification string) (string, string, error) {
		contents, ok := files[specification]
		if !ok {
			return "", "", fmt.Errorf("not found")
		}
		return specification, contents, nil
	}
	document, err := Load("base", files["base"], resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Statements) != 4 {
		t.Fatalf("statements = %#v", document.Statements)
	}
	for index, want := range []struct{ name, source string }{{"first", "base"}, {"second", "child"}, {"third", "nested"}, {"last", "base"}} {
		got := document.Statements[index].Assignment
		if got.Name != want.name || got.Position.Source != want.source {
			t.Errorf("statement %d = %#v; want %s from %s", index, got, want.name, want.source)
		}
	}
}

func TestLoadRejectsMissingResolverAndCycles(t *testing.T) {
	if _, err := Load("base", "<child>", nil); err == nil {
		t.Fatal("include without resolver succeeded")
	}
	resolver := func(_ string, specification string) (string, string, error) {
		if specification == "child" {
			return "child", "<base>", nil
		}
		return "base", "<child>", nil
	}
	if _, err := Load("base", "<child>", resolver); err == nil {
		t.Fatal("include cycle succeeded")
	}
}

func TestLoadWrapsResolverError(t *testing.T) {
	want := errors.New("denied")
	_, err := Load("base", "<child>", func(string, string) (string, string, error) { return "", "", want })
	if !errors.Is(err, want) {
		t.Fatalf("Load error = %v; want wrapped resolver error", err)
	}
}
