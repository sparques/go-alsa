//go:build linux && (amd64 || arm64)

package alsa

import (
	"os"
	"strconv"
	"testing"
)

func TestConfigEvalOptionsHardware(t *testing.T) {
	text := os.Getenv("GO_ALSA_TEST_CONFIG_CARD")
	if text == "" {
		t.Skip("set GO_ALSA_TEST_CONFIG_CARD to a kernel card number")
	}
	want, err := strconv.Atoi(text)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := Cards()
	if err != nil {
		t.Fatal(err)
	}
	options, err := ConfigEvalOptions(cards)
	if err != nil {
		t.Fatal(err)
	}
	var found *Card
	for index := range cards {
		if cards[index].Number == want {
			found = &cards[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("card %d not discovered", want)
	}
	number, err := options.CardNumber(found.ID)
	if err != nil || number != int64(want) {
		t.Fatalf("CardNumber(%q) = %d, %v", found.ID, number, err)
	}
	id, err := options.CardID(text)
	if err != nil || id != found.ID {
		t.Fatalf("CardID(%q) = %q, %v; want %q", text, id, err, found.ID)
	}
	name, err := options.CardName(text)
	if err != nil || name != found.Name {
		t.Fatalf("CardName(%q) = %q, %v; want %q", text, name, err, found.Name)
	}
}
