//go:build linux && (amd64 || arm64)

package control

import (
	"os"
	"testing"

	alsaconfig "github.com/sparques/go-alsa/config"
)

func TestOpenNamedHardware(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_CONTROL")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_CONTROL to a card number or ID")
	}
	document, err := alsaconfig.Parse("test", `ctl.hw {
 @args [ CARD ]
 @args.CARD { type string }
 type hw
 card $CARD
}`)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := OpenNamed(alsaconfig.NewEvaluator(document, alsaconfig.EvalOptions{}), "hw:"+selector)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	info, err := handle.CardInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.Number < 0 || info.ID == "" {
		t.Fatalf("invalid card info: %#v", info)
	}
}
