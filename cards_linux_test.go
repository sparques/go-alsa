//go:build linux && (amd64 || arm64)

package alsa

import (
	"errors"
	"testing"
)

func TestControlCardNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want int
		ok   bool
	}{
		{path: "/dev/snd/controlC0", want: 0, ok: true},
		{path: "/dev/snd/controlC27", want: 27, ok: true},
		{path: "/dev/snd/pcmC0D0p"},
		{path: "/dev/snd/controlC"},
		{path: "/dev/snd/controlC-1"},
		{path: "/dev/snd/controlCfoo"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, err := controlCardNumber(test.path)
			if test.ok {
				if err != nil || got != test.want {
					t.Fatalf("controlCardNumber(%q) = %d, %v; want %d, nil", test.path, got, err, test.want)
				}
				return
			}
			if !errors.Is(err, ErrMalformedDevice) {
				t.Fatalf("controlCardNumber(%q) error = %v; want ErrMalformedDevice", test.path, err)
			}
		})
	}
}

func TestCString(t *testing.T) {
	t.Parallel()
	if got := cString([]byte{'a', 'b', 0, 'c'}); got != "ab" {
		t.Fatalf("cString() = %q; want ab", got)
	}
}

func TestCardsSmoke(t *testing.T) {
	cards, err := Cards()
	if err != nil {
		t.Fatalf("Cards() error = %v", err)
	}
	for _, card := range cards {
		if card.Number < 0 || card.ID == "" || card.Name == "" || card.Path == "" {
			t.Errorf("invalid card: %#v", card)
		}
	}
}

func TestPCMsSmoke(t *testing.T) {
	pcms, err := PCMs()
	if err != nil {
		t.Fatalf("PCMs() error = %v", err)
	}
	for _, pcm := range pcms {
		if pcm.Card < 0 || pcm.Device < 0 || pcm.ID == "" || pcm.Name == "" || pcm.Path == "" {
			t.Errorf("invalid PCM: %#v", pcm)
		}
		if pcm.Stream != Playback && pcm.Stream != Capture {
			t.Errorf("invalid PCM stream: %#v", pcm)
		}
	}
}

func TestStreamString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		stream Stream
		want   string
	}{
		{Playback, "playback"},
		{Capture, "capture"},
		{Stream(99), "unknown"},
	}
	for _, test := range tests {
		if got := test.stream.String(); got != test.want {
			t.Errorf("Stream(%d).String() = %q; want %q", test.stream, got, test.want)
		}
	}
}
