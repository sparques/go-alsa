package config

import "testing"

const argumentTemplate = `pcm.hw {
 @args [ CARD DEV SUBDEV ]
 @args.CARD { type string default Default }
 @args.DEV { type integer default 0 }
 @args.SUBDEV { type integer default 0 }
 type hw
 card $CARD
 device $DEV
 subdevice $SUBDEV
}`

func TestPCMRuntimeArguments(t *testing.T) {
	document, err := Parse("test", argumentTemplate)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{})
	tests := map[string]string{
		"hw":                           "hw:CARD=Default,DEV=0,SUBDEV=0",
		"hw:Device,2,1":                "hw:CARD=Device,DEV=2,SUBDEV=1",
		"hw:CARD=Radio,DEV=3,SUBDEV=2": "hw:CARD=Radio,DEV=3,SUBDEV=2",
		"hw:{ CARD USB DEV 4 }":        "hw:CARD=USB,DEV=4,SUBDEV=0",
	}
	for invocation, want := range tests {
		definition, err := evaluator.PCMName(invocation)
		if err != nil {
			t.Errorf("PCMName(%q): %v", invocation, err)
			continue
		}
		selector, err := definition.HardwareSelector()
		if err != nil || selector != want {
			t.Errorf("PCMName(%q) selector = %q, %v; want %q", invocation, selector, err, want)
		}
	}
}

func TestPCMRuntimeArgumentsUseInjectedValuesWithoutColon(t *testing.T) {
	document, err := Parse("test", argumentTemplate)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{Variables: map[string]Value{
		"CARD": {Kind: ValueString, String: "Injected"},
		"DEV":  {Kind: ValueInteger, Integer: 7},
	}})
	definition, err := evaluator.PCMName("hw")
	if err != nil {
		t.Fatal(err)
	}
	selector, err := definition.HardwareSelector()
	if err != nil || selector != "hw:CARD=Injected,DEV=7,SUBDEV=0" {
		t.Fatalf("selector = %q, %v", selector, err)
	}
}

func TestPCMRuntimeArgumentErrors(t *testing.T) {
	document, err := Parse("test", argumentTemplate)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{})
	for _, invocation := range []string{"hw:", "hw:1,2,3,4", "hw:UNKNOWN=1", "hw:DEV=bad", "missing:0", "hw:{ BAD", "hw:{ DEV 1 DEV 2 }"} {
		if _, err := evaluator.PCMName(invocation); err == nil {
			t.Errorf("PCMName(%q) succeeded", invocation)
		}
	}
}

func FuzzParsePCMInvocation(f *testing.F) {
	for _, seed := range []string{"hw:0,1", "hw:CARD=Device,DEV=0", "hw:{ CARD 0 DEV 1 }", "default"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) { _, _, _, _ = parsePCMInvocation(input) })
}
