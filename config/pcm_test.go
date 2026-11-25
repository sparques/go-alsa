package config

import "testing"

func TestResolveNamedPCM(t *testing.T) {
	document, err := Parse("test", `defaults.card 0
pcm.device {
    type hw
    card $CARD
    device $DEV
    hint { description "Direct device" }
}`)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{Variables: map[string]Value{
		"CARD": {Kind: ValueString, String: "Radio"},
		"DEV":  {Kind: ValueInteger, Integer: 2},
	}})
	definition, err := evaluator.PCM("device")
	if err != nil {
		t.Fatal(err)
	}
	if definition.Name != "device" || definition.Type != "hw" {
		t.Fatalf("definition = %#v", definition)
	}
	card, _ := definition.Field("card")
	device, _ := definition.Field("device")
	hint, _ := definition.Field("hint")
	if card.String != "Radio" || device.Integer != 2 || hint.Kind != ValueCompound {
		t.Fatalf("fields = %#v", definition.Fields)
	}
	selector, err := definition.HardwareSelector()
	if err != nil || selector != "hw:CARD=Radio,DEV=2" {
		t.Fatalf("HardwareSelector = %q, %v", selector, err)
	}
}

func TestHardwareSelectorValidation(t *testing.T) {
	for _, definition := range []PCM{
		{Name: "plug", Type: "plug"},
		{Name: "missing", Type: "hw", Fields: map[string]Value{}},
		{Name: "bad-card", Type: "hw", Fields: map[string]Value{"card": {Kind: ValueString, String: "bad,card"}}},
		{Name: "bad-device", Type: "hw", Fields: map[string]Value{"card": {Kind: ValueInteger, Integer: 0}, "device": {Kind: ValueInteger, Integer: -1}}},
	} {
		if _, err := definition.HardwareSelector(); err == nil {
			t.Errorf("HardwareSelector(%#v) succeeded", definition)
		}
	}
}

func TestResolveNamedPCMRejectsIncompleteDefinitions(t *testing.T) {
	document, err := Parse("test", "pcm.scalar hw\npcm.incomplete { card 0 }")
	if err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(document, EvalOptions{})
	for _, name := range []string{"scalar", "incomplete", "missing"} {
		if _, err := evaluator.PCM(name); err == nil {
			t.Errorf("PCM(%q) succeeded", name)
		}
	}
}
