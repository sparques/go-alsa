package config

import (
	"errors"
	"testing"
)

func TestParseDocument(t *testing.T) {
	document, err := Parse("test.conf", `<confdir:pcm/front.conf>
defaults.pcm.card 0
defaults.pcm.enabled on
defaults.pcm.gain -1.5
pcm.!default {
    type plug
    slave.pcm "hw:0,0"
    values [ 1, false, "three", { name four } ]
}`)
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != "test.conf" || len(document.Statements) != 5 || document.Statements[0].Include != "confdir:pcm/front.conf" {
		t.Fatalf("document = %#v", document)
	}
	if got := document.Statements[1].Assignment.Value; got.Kind != ValueInteger || got.Integer != 0 {
		t.Fatalf("integer = %#v", got)
	}
	if got := document.Statements[2].Assignment.Value; got.Kind != ValueBoolean || !got.Boolean {
		t.Fatalf("boolean = %#v", got)
	}
	if got := document.Statements[3].Assignment.Value; got.Kind != ValueReal || got.Real != -1.5 {
		t.Fatalf("real = %#v", got)
	}
	root := document.Statements[4].Assignment
	if root.Name != "pcm.default" || root.RawName != "pcm.!default" || root.Mode != AssignOverride || root.Value.Kind != ValueCompound {
		t.Fatalf("root = %#v", root)
	}
	items := root.Value.Entries[2].Assignment.Value.Items
	if len(items) != 4 || items[0].Kind != ValueInteger || items[1].Kind != ValueBoolean || items[2].String != "three" || items[3].Kind != ValueCompound {
		t.Fatalf("items = %#v", items)
	}
}

func TestAssignmentModes(t *testing.T) {
	for raw, want := range map[string]AssignmentMode{"pcm.!default": AssignOverride, "?defaults.foo": AssignDefault, "cards.+x": AssignMerge, "-obsolete": AssignRemove, "normal": AssignNormal} {
		_, _, got := assignmentName(raw, false)
		if got != want {
			t.Errorf("assignmentName(%q) mode = %v; want %v", raw, got, want)
		}
	}
}

func TestParseRejectsMalformedStructure(t *testing.T) {
	for _, input := range []string{"pcm { type hw", "values [ 1 2", "pcm", "[ x ]", "values [ <file> ]"} {
		_, err := Parse("bad.conf", input)
		var syntax *syntaxError
		if !errors.As(err, &syntax) {
			t.Errorf("Parse(%q) = %v; want syntax error", input, err)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"pcm.hw { type hw card 0 }", "values [ 1 true x ]", "pcm.!default \"hw:0\"", "<confdir:pcm/front.conf>"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) { _, _ = Parse("fuzz", input) })
}
