package alsa

import (
	"errors"
	"testing"
)

func FuzzParseHardwareAddress(f *testing.F) {
	for _, seed := range []string{
		"hw:0", "hw:0,1", "hw:Device,0,0", "hw:CARD=Generic,DEV=0,SUBDEV=1",
		"hw:CARD=Generic,DEV=0,PLAYBACK", "hw:CARD=Generic,DEV=0,CAPTURE",
		"", "default", "hw:", "hw:-1,0", "hw:CARD=x,DEV=999999999999999999999999",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		address, err := ParseHardwareAddress(name)
		if err != nil {
			if !errors.Is(err, ErrMalformedDevice) {
				t.Fatalf("ParseHardwareAddress(%q) = %v; want ErrMalformedDevice", name, err)
			}
			var deviceError *DeviceError
			if !errors.As(err, &deviceError) {
				t.Fatalf("ParseHardwareAddress(%q) lacks DeviceError: %v", name, err)
			}
			return
		}
		if address.Card == "" {
			t.Fatalf("ParseHardwareAddress(%q) returned an empty card", name)
		}
		if address.Device < 0 || address.Subdevice < 0 {
			t.Fatalf("ParseHardwareAddress(%q) returned negative indices: %#v", name, address)
		}
	})
}
