package alsa

import (
	"fmt"
	"slices"
	"strconv"

	alsaconfig "github.com/sparques/go-alsa/config"
)

// ConfigEvalOptions converts an explicit card snapshot, such as the result of
// Cards, into pure configuration evaluator callbacks and card-hook metadata.
// It performs no discovery itself. Callers may add DataDir, LookupEnv, or
// Variables to the returned options before constructing an evaluator.
func ConfigEvalOptions(cards []Card) (alsaconfig.EvalOptions, error) {
	byNumber := make(map[int]Card, len(cards))
	byID := make(map[string]Card, len(cards))
	hookCards := make([]alsaconfig.HookCard, 0, len(cards))
	for _, card := range cards {
		if card.Number < 0 || card.ID == "" || card.Driver == "" || card.Name == "" {
			return alsaconfig.EvalOptions{}, fmt.Errorf("alsa: invalid card metadata for configuration: %#v", card)
		}
		if _, exists := byNumber[card.Number]; exists {
			return alsaconfig.EvalOptions{}, fmt.Errorf("alsa: duplicate configuration card number %d", card.Number)
		}
		if _, exists := byID[card.ID]; exists {
			return alsaconfig.EvalOptions{}, fmt.Errorf("alsa: duplicate configuration card ID %q", card.ID)
		}
		byNumber[card.Number] = card
		byID[card.ID] = card
		hookCards = append(hookCards, alsaconfig.HookCard{Number: int64(card.Number), Driver: card.Driver})
	}
	slices.SortFunc(hookCards, func(a, b alsaconfig.HookCard) int { return int(a.Number - b.Number) })
	lookup := func(selector string) (Card, error) {
		if number, err := strconv.Atoi(selector); err == nil {
			if card, ok := byNumber[number]; ok {
				return card, nil
			}
		} else if card, ok := byID[selector]; ok {
			return card, nil
		}
		return Card{}, fmt.Errorf("%w: card %q", ErrDeviceNotFound, selector)
	}
	return alsaconfig.EvalOptions{
		Cards: hookCards,
		CardNumber: func(selector string) (int64, error) {
			card, err := lookup(selector)
			return int64(card.Number), err
		},
		CardID: func(selector string) (string, error) {
			card, err := lookup(selector)
			return card.ID, err
		},
		CardName: func(selector string) (string, error) {
			card, err := lookup(selector)
			return card.Name, err
		},
	}, nil
}
