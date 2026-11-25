package pcm

import (
	"errors"
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
	if err == nil || errors.Is(err, alsa.ErrUnsupportedPlatform) {
		t.Fatalf("unsupported configured plugin error = %v", err)
	}
}
