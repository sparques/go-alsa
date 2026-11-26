package control

import (
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
	if _, err := OpenNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "default"); err == nil {
		t.Fatal("OpenNamed accepted unsupported control plugin")
	}
}
