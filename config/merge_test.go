package config

import "testing"

func TestEffectiveAssignmentModes(t *testing.T) {
	document, err := Parse("test", `node { a 1 b 2 }
node { a 3 c 4 }
node { ?a 9 !b 5 -c 6 }
?node { ignored true }`)
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.Effective("node")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, statement := range value.Entries {
		if statement.Assignment != nil && statement.Assignment.Value.Kind == ValueInteger {
			got[statement.Assignment.Name] = statement.Assignment.Value.Integer
		}
	}
	if got["a"] != 3 || got["b"] != 5 || got["c"] != 6 || len(got) != 3 {
		t.Fatalf("effective fields = %#v", got)
	}
}

func TestEffectiveModeErrors(t *testing.T) {
	for _, input := range []string{"-missing 1", "value 1\nvalue text"} {
		document, err := Parse("test", input)
		if err != nil {
			t.Fatal(err)
		}
		name := "missing"
		if input[0] == 'v' {
			name = "value"
		}
		if _, err := document.Effective(name); err == nil {
			t.Errorf("Effective for %q succeeded", input)
		}
	}
}

func TestEffectiveOverrideChangesType(t *testing.T) {
	document, err := Parse("test", "value 1\n!value text")
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.Effective("value")
	if err != nil || value.Kind != ValueString || value.String != "text" {
		t.Fatalf("Effective = %#v, %v", value, err)
	}
}
