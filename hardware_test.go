package alsa

import (
	"errors"
	"testing"
)

func TestParseHardwareAddress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want HardwareAddress
	}{
		{"hw:0,1", HardwareAddress{Card: "0", Device: 1, Subdevice: -1}},
		{"hw:2,3,4", HardwareAddress{Card: "2", Device: 3, Subdevice: 4}},
		{"hw:2,3,-1", HardwareAddress{Card: "2", Device: 3, Subdevice: -1}},
		{"hw:CARD=Radio,DEV=0", HardwareAddress{Card: "Radio", Device: 0, Subdevice: -1}},
		{"hw:CARD=Radio,DEV=0,SUBDEV=-1", HardwareAddress{Card: "Radio", Device: 0, Subdevice: -1}},
		{"hw:CARD=USB-Audio_1,DEV=2,SUBDEV=3", HardwareAddress{Card: "USB-Audio_1", Device: 2, Subdevice: 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseHardwareAddress(test.name)
			if err != nil || got != test.want {
				t.Fatalf("ParseHardwareAddress(%q) = %#v, %v; want %#v", test.name, got, err, test.want)
			}
		})
	}
}

func TestParseHardwareAddressRejectsMalformedNames(t *testing.T) {
	t.Parallel()
	names := []string{
		"", "default", "plughw:0,0", "hw:", "hw:0", "hw:-1,0",
		"hw:0,-1", "hw:0,0,-2", "hw:0,DEV=0", "hw:CARD=Radio",
		"hw:CARD=Radio,DEV=0,DEV=1", "hw:CARD=Radio,DEVICE=0",
		"hw:CARD=bad/card,DEV=0", "hw: 0,0",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			_, err := ParseHardwareAddress(name)
			if !errors.Is(err, ErrMalformedDevice) {
				t.Fatalf("ParseHardwareAddress(%q) error = %v; want ErrMalformedDevice", name, err)
			}
			var deviceError *DeviceError
			if !errors.As(err, &deviceError) || deviceError.Device != name {
				t.Fatalf("ParseHardwareAddress(%q) error = %#v; want contextual DeviceError", name, err)
			}
		})
	}
}

func TestResolveHardwarePCM(t *testing.T) {
	t.Parallel()
	cards := []Card{{Number: 0, ID: "HDMI"}, {Number: 2, ID: "Radio"}}
	pcms := []PCM{
		{Card: 0, Device: 3, Stream: Playback, Subdevices: 1, Path: "/dev/snd/pcmC0D3p"},
		{Card: 2, Device: 0, Stream: Playback, Subdevices: 2, Path: "/dev/snd/pcmC2D0p"},
		{Card: 2, Device: 0, Stream: Capture, Subdevices: 1, Path: "/dev/snd/pcmC2D0c"},
	}

	got, err := resolveHardwarePCM(HardwareAddress{Card: "Radio", Device: 0, Subdevice: 1}, Playback, cards, pcms)
	if err != nil {
		t.Fatal(err)
	}
	if got.Address.Card != "2" || got.Address.Subdevice != 1 || got.PCM.Path != "/dev/snd/pcmC2D0p" {
		t.Fatalf("resolved PCM = %#v", got)
	}

	tests := []struct {
		name    string
		address HardwareAddress
		stream  Stream
		want    error
	}{
		{"missing card", HardwareAddress{Card: "Missing", Device: 0}, Playback, ErrDeviceNotFound},
		{"missing device", HardwareAddress{Card: "Radio", Device: 9}, Playback, ErrDeviceNotFound},
		{"wrong direction", HardwareAddress{Card: "HDMI", Device: 3}, Capture, ErrDirectionUnavailable},
		{"missing subdevice", HardwareAddress{Card: "Radio", Device: 0, Subdevice: 1}, Capture, ErrSubdeviceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveHardwarePCM(test.address, test.stream, cards, pcms)
			if !errors.Is(err, test.want) {
				t.Fatalf("resolve error = %v; want %v", err, test.want)
			}
		})
	}
}

func TestHardwareAddressString(t *testing.T) {
	t.Parallel()
	address := HardwareAddress{Card: "Radio", Device: 2, Subdevice: 1}
	if got, want := address.String(), "hw:CARD=Radio,DEV=2,SUBDEV=1"; got != want {
		t.Fatalf("String() = %q; want %q", got, want)
	}
}
