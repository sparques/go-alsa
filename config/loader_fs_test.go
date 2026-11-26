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
}
