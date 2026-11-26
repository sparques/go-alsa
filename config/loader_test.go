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

func TestLoadLayersAppliesOrderedPrecedence(t *testing.T) {
	layers := []Layer{
		{Source: "system.conf", Contents: `pcm.default { type hw card 0 hint { description System } }`},
		{Source: "application.conf", Contents: `pcm.default { device 1 }`},
		{Source: "user.conf", Contents: `pcm.!default { type plug slave.pcm "hw:2,0" }`},
	}
	document, err := LoadLayers(layers, nil)
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.Effective("pcm.default")
	if err != nil {
		t.Fatal(err)
	}
	typeValue, _ := compoundValue(value, "type")
	slave, _ := compoundValue(value, "slave.pcm")
	if typeValue.String != "plug" || slave.String != "hw:2,0" {
		t.Fatalf("effective user override = %#v", value)
	}
	if value.Position.Source != "user.conf" {
		t.Fatalf("effective source = %q", value.Position.Source)
	}
}

func TestLoadLayersSharesIncludeState(t *testing.T) {
	resolver := func(from, specification string) (string, string, error) {
		return from + "/" + specification, "included 1", nil
	}
	document, err := LoadLayers([]Layer{
		{Source: "system", Contents: "<common>"},
		{Source: "user", Contents: "<common>"},
	}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Statements) != 2 || document.Statements[0].Position.Source != "system/common" || document.Statements[1].Position.Source != "user/common" {
		t.Fatalf("layered includes = %#v", document.Statements)
	}
}
