package config

import "testing"

func TestControlRuntimeArguments(t *testing.T) {
	document, err := Parse("test", `ctl.hw {
 @args [ CARD ]
 @args.CARD { type string default 0 }
 type hw
 card $CARD
 hint.description "Direct control"
}`)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{})
	for invocation, want := range map[string]string{"hw": "0", "hw:USB": "USB", "hw:CARD=Radio": "Radio", "hw:{ CARD 3 }": "3"} {
		control, err := evaluator.ControlName(invocation)
		if err != nil {
			t.Errorf("ControlName(%q): %v", invocation, err)
			continue
		}
		card, _ := control.Field("card")
		if control.Type != "hw" || card.String != want || control.Fields["hint.description"].String != "Direct control" {
			t.Errorf("ControlName(%q) = %#v", invocation, control)
		}
		selector, err := control.HardwareSelector()
		if err != nil || selector != "hw:CARD="+want {
			t.Errorf("ControlName(%q) selector = %q, %v", invocation, selector, err)
		}
	}
}

func TestControlRuntimeArgumentErrors(t *testing.T) {
	document, err := Parse("test", `ctl.hw { @args [ CARD ] @args.CARD { type string } type hw card $CARD }`)
	if err != nil {
		t.Fatal(err)
	}
	for _, invocation := range []string{"hw", "hw:", "hw:UNKNOWN=1", "missing:0"} {
		if _, err := NewEvaluator(document, EvalOptions{}).ControlName(invocation); err == nil {
			t.Errorf("ControlName(%q) succeeded", invocation)
		}
	}
}
