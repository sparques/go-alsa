package control

import (
	"errors"
	"testing"

	alsaconfig "github.com/sparques/go-alsa/config"
)

func TestOpenNamedValidation(t *testing.T) {
	if _, err := OpenNamed(nil, "default"); err == nil {
		t.Fatal("OpenNamed(nil) succeeded")
	}
	document, err := alsaconfig.Parse("test", `ctl.default { type shm server socket }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "default"); !errors.Is(err, ErrUnsupportedPlugin) {
		t.Fatalf("OpenNamed unsupported plugin error = %v", err)
	}
}

func TestDirectGraphSelectorTraversesEmpty(t *testing.T) {
	node := &alsaconfig.ControlNode{Name: "default", Type: "empty", Child: &alsaconfig.ControlNode{
		Name: "default.child", Type: "hw", Fields: map[string]alsaconfig.Value{"card": {Kind: alsaconfig.ValueString, String: "3"}},
	}}
	selector, err := directGraphSelector(node)
	if err != nil || selector != "hw:CARD=3" {
		t.Fatalf("directGraphSelector = %q, %v", selector, err)
	}
}
