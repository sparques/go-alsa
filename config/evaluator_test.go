package config

import "testing"

func TestEvaluatorFunctionsAndVariables(t *testing.T) {
	document, err := Parse("test", `base 7
alias { @func refer name base }
text { @func concat strings [ "card=" $CARD ] }
environment { @func getenv vars [ MISSING PRESENT ] default fallback }
integer { @func igetenv vars [ NUMBER ] default 3 }`)
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{"PRESENT": "usb", "NUMBER": "42"}
	evaluator := NewEvaluator(document, EvalOptions{
		Variables: map[string]Value{"CARD": {Kind: ValueInteger, Integer: 2}},
		LookupEnv: func(name string) (string, bool) { value, ok := environment[name]; return value, ok },
	})
	tests := []struct {
		name    string
		kind    ValueKind
		text    string
		integer int64
	}{
		{"alias", ValueInteger, "", 7},
		{"text", ValueString, "card=2", 0},
		{"environment", ValueString, "usb", 0},
		{"integer", ValueInteger, "", 42},
	}
	for _, test := range tests {
		value, err := evaluator.Reference(test.name)
		if err != nil || value.Kind != test.kind || value.String != test.text || value.Integer != test.integer {
			t.Errorf("Reference(%q) = %#v, %v", test.name, value, err)
		}
	}
}

func TestEvaluatorCardNumber(t *testing.T) {
	document, err := Parse("test", `numeric { @func card_inum card 3 }
stable { @func card_inum card USB }`)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{CardNumber: func(card string) (int64, error) {
		if card != "USB" {
			t.Fatalf("card resolver received %q", card)
		}
		return 7, nil
	}})
	for name, want := range map[string]int64{"numeric": 3, "stable": 7} {
		value, err := evaluator.Reference(name)
		if err != nil || value.Kind != ValueInteger || value.Integer != want {
			t.Errorf("Reference(%q) = %#v, %v; want %d", name, value, err, want)
		}
	}
}

func TestEvaluatorDefaultsAndUnsupportedFunctions(t *testing.T) {
	document, err := Parse("test", `fallback { @func getenv vars [ ABSENT ] default "safe" }
dynamic { @func refer name { @func concat strings [ target .value ] } }
missing { @func refer name absent.value default { type plug slave.pcm hw } }
target.value 17
unsupported { @func shell command bad }`)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{})
	value, err := evaluator.Reference("fallback")
	if err != nil || value.String != "safe" {
		t.Fatalf("fallback = %#v, %v", value, err)
	}
	value, err = evaluator.Reference("dynamic")
	if err != nil || value.Kind != ValueInteger || value.Integer != 17 {
		t.Fatalf("dynamic refer = %#v, %v", value, err)
	}
	value, err = evaluator.Reference("missing")
	if err != nil || value.Kind != ValueCompound {
		t.Fatalf("refer fallback = %#v, %v", value, err)
	}
	if _, err := evaluator.Reference("unsupported"); err == nil {
		t.Fatal("unsupported function succeeded")
	}
}

func TestEvaluatorReferFallbackDoesNotHideCycles(t *testing.T) {
	document, err := Parse("test", `a { @func refer name b default safe }
b { @func refer name a }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvaluator(document, EvalOptions{}).Reference("a"); err == nil {
		t.Fatal("refer cycle used fallback")
	}
}

func TestEvaluatorRejectsCycles(t *testing.T) {
	document, err := Parse("test", "a.path b.path\nb.path a.path")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvaluator(document, EvalOptions{}).Reference("a.path"); err == nil {
		t.Fatal("evaluation cycle succeeded")
	}
}

func FuzzEvaluator(f *testing.F) {
	for _, seed := range []string{
		"value { @func concat strings [ a 1 true ] }",
		"value { @func getenv vars [ HOME ] default safe }",
		"base 1\nvalue { @func refer name base }",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		document, err := Parse("fuzz", input)
		if err != nil {
			return
		}
		evaluator := NewEvaluator(document, EvalOptions{})
		for _, statement := range document.Statements {
			if statement.Assignment != nil {
				_, _ = evaluator.Value(statement.Assignment.Value)
			}
		}
	})
}
