package main

import (
	"fmt"
	"os"

	alsa "github.com/sparques/go-alsa"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cards, err := alsa.Cards()
	if err != nil {
		return err
	}
	pcms, err := alsa.PCMs()
	if err != nil {
		return err
	}

	if len(cards) == 0 {
		fmt.Println("No ALSA cards found.")
		return nil
	}
	for _, card := range cards {
		fmt.Printf("card %d: %s [%s]\n", card.Number, card.Name, card.ID)
		fmt.Printf("  driver: %s\n  path: %s\n", card.Driver, card.Path)
		for _, pcm := range pcms {
			if pcm.Card != card.Number {
				continue
			}
			fmt.Printf("  pcm %d %s: %s [%s], subdevices %d/%d available\n",
				pcm.Device, pcm.Stream, pcm.Name, pcm.ID,
				pcm.AvailableSubdevices, pcm.Subdevices)
			fmt.Printf("    path: %s\n", pcm.Path)
		}
	}
	return nil
}
