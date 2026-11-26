package alsa

import (
	"errors"
	"reflect"
	"testing"

	alsaconfig "github.com/sparques/go-alsa/config"
)

func TestConfigEvalOptions(t *testing.T) {
	options, err := ConfigEvalOptions([]Card{
		{Number: 3, ID: "Device", Driver: "USB-Audio", Name: "C-Media USB Audio"},
		{Number: 0, ID: "PCH", Driver: "HDA-Intel", Name: "Built-in Audio"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantHooks := []alsaconfig.HookCard{{Number: 0, Driver: "HDA-Intel"}, {Number: 3, Driver: "USB-Audio"}}
	if !reflect.DeepEqual(options.Cards, wantHooks) {
		t.Fatalf("hook cards = %#v; want %#v", options.Cards, wantHooks)
	}
	number, err := options.CardNumber("Device")
	if err != nil || number != 3 {
		t.Fatalf("CardNumber(Device) = %d, %v", number, err)
	}
	id, err := options.CardID("3")
	if err != nil || id != "Device" {
		t.Fatalf("CardID(3) = %q, %v", id, err)
	}
	name, err := options.CardName("PCH")
	if err != nil || name != "Built-in Audio" {
		t.Fatalf("CardName(PCH) = %q, %v", name, err)
	}
	if _, err := options.CardID("missing"); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("missing card error = %v", err)
	}
}

func TestConfigEvalOptionsRejectsAmbiguousCards(t *testing.T) {
	valid := Card{Number: 0, ID: "PCH", Driver: "HDA-Intel", Name: "Audio"}
	for _, cards := range [][]Card{
		{{Number: -1, ID: "bad", Driver: "driver", Name: "name"}},
		{valid, valid},
		{valid, {Number: 1, ID: "PCH", Driver: "other", Name: "Other"}},
	} {
		if _, err := ConfigEvalOptions(cards); err == nil {
			t.Fatalf("ConfigEvalOptions(%#v) succeeded", cards)
		}
	}
}
