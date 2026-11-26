package pcm

import (
	"errors"
	"strings"
	"testing"

	alsa "github.com/sparques/go-alsa"
	alsaconfig "github.com/sparques/go-alsa/config"
)

func TestOpenNamedValidation(t *testing.T) {
	if _, err := OpenNamed(nil, "default", alsa.Playback); err == nil {
		t.Fatal("OpenNamed(nil) succeeded")
	}
	document, err := alsaconfig.Parse("test", "pcm.default { type plug }")
	if err != nil {
		t.Fatal(err)
	}
	_, err = OpenNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "default", alsa.Playback)
	if err == nil || !errors.Is(err, ErrUnsupportedPlugin) || errors.Is(err, alsa.ErrUnsupportedPlatform) {
		t.Fatalf("unsupported configured plugin error = %v", err)
	}
	var pluginError *UnsupportedPluginError
	if !errors.As(err, &pluginError) || pluginError.Plugin != "plug" || pluginError.Position.Source != "test" {
		t.Fatalf("unsupported plugin detail = %#v", pluginError)
	}
}

func TestDirectGraphSelectorTraversesEmptyAndAsym(t *testing.T) {
	playback := &alsaconfig.PCMNode{Name: "playback", Type: "hw", Fields: map[string]alsaconfig.Value{
		"card": {Kind: alsaconfig.ValueString, String: "3"},
	}}
	capture := &alsaconfig.PCMNode{Name: "capture", Type: "hw", Fields: map[string]alsaconfig.Value{
		"card":   {Kind: alsaconfig.ValueString, String: "3"},
		"device": {Kind: alsaconfig.ValueInteger, Integer: 1},
	}}
	root := &alsaconfig.PCMNode{Name: "default", Type: "empty", Slave: &alsaconfig.PCMNode{
		Name: "duplex", Type: "asym", Playback: playback, Capture: capture,
	}}
	for stream, want := range map[alsa.Stream]string{
		alsa.Playback: "hw:CARD=3,DEV=0",
		alsa.Capture:  "hw:CARD=3,DEV=1",
	} {
		selector, err := directGraphSelector(root, stream)
		if err != nil || selector != want {
			t.Errorf("directGraphSelector(%s) = %q, %v; want %q", stream, selector, err, want)
		}
	}
}

func TestDirectGraphSelectorReportsUnsupportedPlugin(t *testing.T) {
	node := &alsaconfig.PCMNode{Name: "default.slave", Type: "dmix", Position: alsaconfig.Position{Source: "system.conf", Line: 12, Column: 3}}
	_, err := directGraphSelector(node, alsa.Playback)
	if !errors.Is(err, ErrUnsupportedPlugin) || !strings.Contains(err.Error(), "system.conf:12:3") {
		t.Fatalf("unsupported plugin error = %v", err)
	}
}
