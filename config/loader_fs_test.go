package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryResolverLoadsConfinedIncludes(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child.conf")
	if err := os.WriteFile(child, []byte("value 42"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Load(filepath.Join(root, "base.conf"), "<confdir:child.conf>", resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Statements) != 1 || document.Statements[0].Assignment.Value.Integer != 42 || document.Statements[0].Position.Source != child {
		t.Fatalf("document = %#v", document)
	}
}

func TestDirectoryResolverRejectsEscapes(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.conf")
	if err := os.WriteFile(outside, []byte("secret true"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.conf")); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, specification := range []string{"../outside.conf", "link.conf", outside} {
		if _, _, err := resolver(filepath.Join(root, "base.conf"), specification); err == nil {
			t.Errorf("resolver accepted escape %q", specification)
		}
	}
}

func TestLoadInstalledConfiguration(t *testing.T) {
	root := os.Getenv("GO_ALSA_TEST_CONFIG_ROOT")
	path := os.Getenv("GO_ALSA_TEST_CONFIG")
	if root == "" || path == "" {
		t.Skip("set GO_ALSA_TEST_CONFIG_ROOT and GO_ALSA_TEST_CONFIG")
	}
	resolver, err := NewDirectoryResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Load(path, string(contents), resolver)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded %s into %d expanded statements", path, len(document.Statements))
	definition, err := NewEvaluator(document, EvalOptions{}).PCMName("hw:3,0")
	if err != nil {
		t.Fatalf("evaluate installed pcm.hw: %v", err)
	}
	selector, err := definition.HardwareSelector()
	if err != nil {
		t.Fatalf("installed pcm.hw selector: %v", err)
	}
	if selector != "hw:CARD=3,DEV=0,SUBDEV=-1" {
		t.Fatalf("installed pcm.hw selector = %q", selector)
	}
	control, err := NewEvaluator(document, EvalOptions{}).ControlName("hw:3")
	if err != nil {
		t.Fatalf("evaluate installed ctl.hw: %v", err)
	}
	controlSelector, err := control.HardwareSelector()
	if err != nil || controlSelector != "hw:CARD=3" {
		t.Fatalf("installed ctl.hw selector = %q, %v", controlSelector, err)
	}

	// Apply the optional top-level override hook through the confined resolver;
	// host paths outside root are ignored because this hook declares errors=false.
	topHooks, err := LoadHooks(document, "@hooks", EvalOptions{DataDir: root}, resolver)
	if err != nil {
		t.Fatalf("load installed top-level hooks: %v", err)
	}
	if len(topHooks.Skipped) != 0 {
		t.Fatalf("installed skipped top-level hooks = %v", topHooks.Skipped)
	}

	// Apply only the declarative card load hook. The driver-specific
	// load_for_all_cards hook remains explicit and is reported as skipped.
	hooks, err := LoadHooks(topHooks.Document, "cards.@hooks", EvalOptions{
		DataDir: root,
		Cards:   []HookCard{{Number: 3, Driver: "USB-Audio"}},
	}, resolver)
	if err != nil {
		t.Fatalf("load installed card hooks: %v", err)
	}
	if len(hooks.Skipped) != 0 {
		t.Fatalf("installed skipped hooks = %v", hooks.Skipped)
	}
	withAliases := hooks.Document
	defaultPCM, err := NewEvaluator(withAliases, EvalOptions{}).PCMName("default")
	if err != nil {
		t.Fatalf("evaluate installed pcm.default: %v", err)
	}
	if defaultPCM.Type != "empty" {
		t.Fatalf("installed pcm.default type = %q; want empty", defaultPCM.Type)
	}
	graph, err := NewEvaluator(withAliases, EvalOptions{}).Graph("default")
	if err != nil {
		t.Fatalf("materialize installed pcm.default graph: %v", err)
	}
	if graph.Root.Type != "empty" || graph.Root.Slave == nil || graph.Root.Slave.Type != "plug" || graph.Root.Slave.Slave == nil || graph.Root.Slave.Slave.Type != "hw" {
		t.Fatalf("installed pcm.default plugin chain is not empty -> plug -> hw")
	}
	cardEvaluator := NewEvaluator(withAliases, EvalOptions{
		CardName: func(card string) (string, error) {
			if card != "3" {
				t.Fatalf("installed card_name received %q", card)
			}
			return "No-Special-Profile", nil
		},
		CardID: func(card string) (string, error) {
			if card != "3" {
				t.Fatalf("installed card_id received %q", card)
			}
			return "Device", nil
		},
	})
	cardValue, err := cardEvaluator.Reference("cards.3.pcm.default:CARD=3")
	if err != nil {
		t.Fatalf("resolve installed numeric-card alias: %v", err)
	}
	cardType, _ := compoundString(cardValue, "type")
	if cardType != "asym" {
		t.Fatalf("installed numeric-card alias type = %q; want asym", cardType)
	}
	cardGraph, err := cardEvaluator.Graph("default:CARD=3")
	if err != nil {
		t.Fatalf("materialize installed card-specific default: %v", err)
	}
	if cardGraph.Root.Type != "empty" || cardGraph.Root.Slave == nil || cardGraph.Root.Slave.Type != "asym" {
		slaveType := "<nil>"
		if cardGraph.Root.Slave != nil {
			slaveType = cardGraph.Root.Slave.Type
		}
		t.Fatalf("installed USB card default types = %s -> %s; want empty -> asym", cardGraph.Root.Type, slaveType)
	}
	asym := cardGraph.Root.Slave
	if asym.Playback == nil || asym.Playback.Type != "plug" || asym.Playback.Slave == nil || asym.Playback.Slave.Type != "dmix" {
		t.Fatalf("installed USB playback branch did not resolve to plug -> dmix")
	}
	if asym.Capture == nil || asym.Capture.Type != "plug" || asym.Capture.Slave == nil || asym.Capture.Slave.Type != "dsnoop" {
		t.Fatalf("installed USB capture branch did not resolve to plug -> dsnoop")
	}
}
