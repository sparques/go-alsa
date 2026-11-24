//go:build linux && (amd64 || arm64)

package mixer

import (
	"os"
	"testing"
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
		}
	}
}
