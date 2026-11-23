//go:build linux && (amd64 || arm64)

package control

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	alsa "github.com/sparques/go-alsa"
)

func TestResolveNumericCard(t *testing.T) {
	t.Parallel()
	for _, selector := range []string{"3", "hw:3", "hw:CARD=3"} {
		got, err := resolveCard(selector)
		if err != nil || got != 3 {
			t.Errorf("resolveCard(%q) = %d, %v; want 3, nil", selector, got, err)
		}
	}
	for _, selector := range []string{"", "-1", "hw:", "hw:3,0"} {
		if _, err := resolveCard(selector); !errors.Is(err, alsa.ErrMalformedDevice) {
			t.Errorf("resolveCard(%q) error = %v; want ErrMalformedDevice", selector, err)
		}
	}
}

func TestControlHardwareEvent(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_CONTROL")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_CONTROL to a card number or ID")
	}
	subscriber, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer subscriber.Close()
	actor, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	ids, err := actor.Elements()
	if err != nil {
		t.Fatal(err)
	}
	var target ElementID
	var original ElementValue
	for _, id := range ids {
		info, err := actor.Info(id)
		if err == nil && info.Type == TypeBoolean && info.Access.Readable() && info.Access.Writable() && !info.Access.Inactive() {
			target = id
			original, err = actor.Read(id)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if target.Number == 0 {
		t.Skip("card has no active writable control")
	}
	if err := subscriber.Subscribe(true); err != nil {
		t.Fatal(err)
	}
	defer subscriber.Subscribe(false)
	changed := original
	changed.Booleans = append([]bool(nil), original.Booleans...)
	changed.Booleans[0] = !changed.Booleans[0]
	if err := actor.Write(changed); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := actor.Write(original); err != nil {
			t.Errorf("restore %s: %v", target, err)
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	event, err := subscriber.NextEvent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != target || event.Mask&EventValue == 0 {
		t.Fatalf("event = %#v; want VALUE for %#v", event, target)
	}
}

func TestElementIDRoundTrip(t *testing.T) {
	t.Parallel()
	want := ElementID{Number: 7, Interface: InterfaceMixer, Device: 2, Subdevice: 3, Name: "Speaker Playback Switch", Index: 1}
	if got := elementIDFromUAPI(elementIDToUAPI(want)); got != want {
		t.Fatalf("round trip = %#v; want %#v", got, want)
	}
}

func TestControlHardware(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_CONTROL")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_CONTROL to a card number or ID")
	}
	handle, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	ids, err := handle.Elements()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("card has no controls")
	}
	locked := false
	tlvCount := 0
	for _, id := range ids {
		if id.Number == 0 || id.Name == "" {
			t.Errorf("invalid ID: %#v", id)
		}
		info, err := handle.Info(id)
		if err != nil {
			t.Fatalf("Info(%s): %v", id, err)
		}
		if info.ID != id || info.Type == TypeNone || info.Count == 0 {
			t.Errorf("invalid info: %#v", info)
		}
		if info.Type == TypeEnumerated {
			for item := uint32(0); item < info.Enumerated.Items; item++ {
				itemInfo, err := handle.EnumeratedItem(id, item)
				if err != nil || itemInfo.Enumerated.SelectedItem != item || itemInfo.Enumerated.ItemName == "" {
					t.Errorf("enumerated item %d for %s = %#v, %v", item, id, itemInfo, err)
				}
			}
		}
		if info.Access.TLVReadable() {
			decoded, err := handle.ReadDecodedTLV(id, 4096)
			if err != nil {
				t.Errorf("ReadDecodedTLV(%s) = %#v, %v", id, decoded, err)
			} else {
				tlvCount++
			}
		}
		if !locked && info.Access.Writable() && !info.Access.Inactive() {
			if err := handle.Lock(id); err != nil {
				t.Errorf("Lock(%s): %v", id, err)
			} else {
				if err := handle.Unlock(id); err != nil {
					t.Errorf("Unlock(%s): %v", id, err)
				}
				locked = true
			}
		}
		if info.Access.Readable() {
			value, err := handle.Read(id)
			if err != nil {
				t.Errorf("Read(%s): %v", id, err)
				continue
			}
			if info.Access.Writable() && !info.Access.Inactive() {
				if err := handle.Write(value); err != nil {
					t.Errorf("write unchanged %s: %v", id, err)
					continue
				}
				after, err := handle.Read(id)
				if err != nil || !reflect.DeepEqual(after, value) {
					t.Errorf("unchanged round trip %s = %#v, %v; want %#v", id, after, err, value)
				}
			}
		}
	}
	t.Logf("card %s exposes %s controls, %d readable TLVs", selector, strconv.Itoa(len(ids)), tlvCount)
}
