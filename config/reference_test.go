package config

import "testing"

func TestDocumentLookupNestedAndDuplicateAssignments(t *testing.T) {
	document, err := Parse("test", "defaults.pcm.card 0\ndefaults { pcm { card 1 } }\n\"literal.key\" 2")
	if err != nil {
		t.Fatal(err)
	}
	items := document.Assignments("defaults.pcm.card")
	if len(items) != 2 || items[0].Value.Integer != 0 || items[1].Value.Integer != 1 {
		t.Fatalf("assignments = %#v", items)
	}
	last, ok := document.Lookup("defaults.pcm.card")
	if !ok || last.Value.Integer != 1 {
		t.Fatalf("lookup = %#v, %t", last, ok)
	}
	literal, ok := document.Lookup("literal.key")
	if !ok || len(literal.Path) != 1 {
		t.Fatalf("literal lookup = %#v, %t", literal, ok)
	}
}

func TestResolveReference(t *testing.T) {
	document, err := Parse("test", "defaults.pcm.card 3\ndefaults.alias defaults.pcm.card\ndefaults.indirect { @func refer name defaults.alias }")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"defaults.pcm.card", "defaults.alias", "defaults.indirect"} {
		value, err := document.ResolveReference(name)
		if err != nil || value.Kind != ValueInteger || value.Integer != 3 {
			t.Errorf("ResolveReference(%q) = %#v, %v", name, value, err)
		}
	}
}

func TestResolveReferenceRejectsCycles(t *testing.T) {
	document, err := Parse("test", "a.path b.path\nb.path a.path")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.ResolveReference("a.path"); err == nil {
		t.Fatal("reference cycle succeeded")
	}
	if _, err := document.ResolveReference("missing.path"); err == nil {
		t.Fatal("missing reference succeeded")
	}
}
