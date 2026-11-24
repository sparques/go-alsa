//go:build linux && (amd64 || arm64)

package mixer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sparques/go-alsa/control"
)

func TestMixerHardware(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_MIXER")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_MIXER to an ALSA card number or ID")
	}
	handle, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	elements, err := handle.Elements()
	if err != nil {
		t.Fatal(err)
	}
	if len(elements) == 0 {
		t.Fatal("no conventional simple mixer elements found")
	}
	for _, element := range elements {
		t.Logf("element %q index=%d playback=(switch=%v volume=%v) capture=(switch=%v volume=%v)",
			element.Name, element.Index, element.HasPlaybackSwitch(), element.HasPlaybackVolume(),
			element.HasCaptureSwitch(), element.HasCaptureVolume())
		if element.HasPlaybackSwitch() {
			values, err := element.PlaybackSwitch()
			if err != nil {
				t.Fatal(err)
			}
			if err := element.SetPlaybackSwitch(values); err != nil {
				t.Fatal(err)
			}
		}
		if element.HasCaptureSwitch() {
			values, err := element.CaptureSwitch()
			if err != nil {
				t.Fatal(err)
			}
			if err := element.SetCaptureSwitch(values); err != nil {
				t.Fatal(err)
			}
		}
		if element.HasPlaybackVolume() {
			volume, err := element.PlaybackVolume()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s playback raw=%v range=[%d,%d] dB=%v", element.Name, volume.Values, volume.Minimum, volume.Maximum, volume.DB)
			if err := element.SetPlaybackVolume(volume.Values); err != nil {
				t.Fatal(err)
			}
			if err := element.SetPlaybackVolumeNormalized(volume.Normalized); err != nil {
				t.Fatal(err)
			}
		}
		if element.HasCaptureVolume() {
			volume, err := element.CaptureVolume()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s capture raw=%v range=[%d,%d] dB=%v", element.Name, volume.Values, volume.Minimum, volume.Maximum, volume.DB)
			if err := element.SetCaptureVolume(volume.Values); err != nil {
				t.Fatal(err)
			}
			if err := element.SetCaptureVolumeNormalized(volume.Normalized); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(elements) > 0 {
		originalName := elements[0].Name
		elements[0].Name = "modified test copy"
		cached, err := handle.Elements()
		if err != nil {
			t.Fatal(err)
		}
		if len(cached) != len(elements) || cached[0].Name != originalName {
			t.Fatalf("cached elements were not isolated: got %d elements, first name %q", len(cached), cached[0].Name)
		}
	}

	enumerations, err := handle.EnumeratedElements()
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range enumerations {
		values, err := element.Values()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("enum %q index=%d values=%v items=%q writable=%v",
			element.Name, element.Index, values, element.Items(), element.Writable())
		if element.Writable() {
			if err := element.SetValues(values); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(enumerations) > 0 {
		originalName := enumerations[0].Name
		originalItems := enumerations[0].Items()
		enumerations[0].Name = "modified test copy"
		if len(enumerations[0].items) > 0 {
			enumerations[0].items[0] = "modified test item"
		}
		cached, err := handle.EnumeratedElements()
		if err != nil {
			t.Fatal(err)
		}
		if len(cached) != len(enumerations) || cached[0].Name != originalName {
			t.Fatal("cached enumerated elements were not isolated")
		}
		if len(originalItems) > 0 && cached[0].Items()[0] != originalItems[0] {
			t.Fatal("cached enumerated item names were not isolated")
		}
	}
}

func TestMixerHardwareEvent(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_MIXER")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_MIXER to an ALSA card number or ID")
	}
	subscriber, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer subscriber.Close()
	writer, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	elements, err := writer.Elements()
	if err != nil {
		t.Fatal(err)
	}
	var target *Element
	for _, element := range elements {
		if element.playbackSwitch != nil && element.playbackSwitch.info.Access.Writable() {
			target = element
			break
		}
		if element.captureSwitch != nil && element.captureSwitch.info.Access.Writable() {
			target = element
			break
		}
	}
	if target == nil {
		t.Skip("no writable conventional switch found")
	}
	playback := target.playbackSwitch != nil && target.playbackSwitch.info.Access.Writable()
	var original []bool
	if playback {
		original, err = target.PlaybackSwitch()
	} else {
		original, err = target.CaptureSwitch()
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(original) == 0 {
		t.Fatal("switch has no channels")
	}
	replacement := append([]bool(nil), original...)
	replacement[0] = !replacement[0]
	restore := func() {
		var restoreErr error
		if playback {
			restoreErr = target.SetPlaybackSwitch(original)
		} else {
			restoreErr = target.SetCaptureSwitch(original)
		}
		if restoreErr != nil {
			t.Errorf("restore switch: %v", restoreErr)
		}
	}
	t.Cleanup(restore)

	if err := subscriber.Subscribe(true); err != nil {
		t.Fatal(err)
	}
	defer subscriber.Subscribe(false)
	if playback {
		err = target.SetPlaybackSwitch(replacement)
	} else {
		err = target.SetCaptureSwitch(replacement)
	}
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		event, err := subscriber.NextEvent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.Recognized && event.Name == target.Name && event.Index == target.Index &&
			event.Kind == EventSwitch && event.Raw.Mask&control.EventValue != 0 {
			wantDirection := DirectionCapture
			if playback {
				wantDirection = DirectionPlayback
			}
			if event.Direction != wantDirection {
				t.Fatalf("event direction = %v; want %v", event.Direction, wantDirection)
			}
			t.Logf("event name=%q index=%d direction=%d kind=%d mask=%#x",
				event.Name, event.Index, event.Direction, event.Kind, event.Raw.Mask)
			return
		}
	}
}
