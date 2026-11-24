//go:build linux && (amd64 || arm64)

package control

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEncodeUserElementSpec(t *testing.T) {
	base := ElementID{Interface: InterfaceMixer, Name: "go-alsa test"}
	tests := []struct {
		name string
		spec UserElementSpec
	}{
		{"boolean", UserElementSpec{ID: base, Type: TypeBoolean, Count: 2}},
		{"integer", UserElementSpec{ID: base, Type: TypeInteger, Count: 2, Integer: IntegerRange{Minimum: -10, Maximum: 10, Step: 2}}},
		{"integer64", UserElementSpec{ID: base, Type: TypeInteger64, Count: 1, Integer: IntegerRange{Minimum: -1 << 40, Maximum: 1 << 40, Step: 1}}},
		{"enumerated", UserElementSpec{ID: base, Type: TypeEnumerated, Count: 2, EnumeratedItems: []string{"Mic", "Line"}}},
		{"bytes", UserElementSpec{ID: base, Type: TypeBytes, Count: 16}},
		{"iec958", UserElementSpec{ID: base, Type: TypeIEC958, Count: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, names, err := encodeUserElementSpec(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			if ElementType(raw.Type) != test.spec.Type || raw.Count != test.spec.Count {
				t.Fatalf("encoded type/count = %d/%d", raw.Type, raw.Count)
			}
			if test.spec.Type == TypeEnumerated {
				if string(names) != "Mic\x00Line\x00" || binary.NativeEndian.Uint32(raw.Value[0:4]) != 2 ||
					binary.NativeEndian.Uint64(raw.Value[72:80]) == 0 || binary.NativeEndian.Uint32(raw.Value[80:84]) != uint32(len(names)) {
					t.Fatalf("encoded enumeration = names %q value %x", names, raw.Value[:84])
				}
			}
		})
	}
}

func TestUserElementHardwareEventStress(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_USER_CONTROL")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_USER_CONTROL to a card number or ID")
	}
	subscriber, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	actor, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.Close() })
	if err := subscriber.Subscribe(true); err != nil {
		t.Fatal(err)
	}
	defer subscriber.Subscribe(false)

	prefix := fmt.Sprintf("ga-%d-%06d-", os.Getpid(), time.Now().UnixNano()%1_000_000)
	created := make([]ElementID, 0, 64)
	t.Cleanup(func() {
		for _, id := range created {
			if err := actor.RemoveUserElement(id); err != nil {
				t.Errorf("cleanup %s: %v", id, err)
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waitFor := func(name string, mask EventMask) {
		t.Helper()
		for {
			event, err := subscriber.NextEvent(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if event.ID.Name != name {
				continue
			}
			if mask == EventRemove {
				if event.Mask == EventRemove {
					return
				}
			} else if event.Mask&mask != 0 {
				return
			}
		}
	}

	for index := 0; index < cap(created); index++ {
		name := fmt.Sprintf("%s%02d", prefix, index)
		id, err := actor.AddUserElement(UserElementSpec{
			ID: ElementID{Interface: InterfaceMixer, Name: name}, Type: TypeBoolean, Count: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, id)
		waitFor(name, EventAdd)
		if err := actor.Write(ElementValue{ID: id, Type: TypeBoolean, Booleans: []bool{true}}); err != nil {
			t.Fatal(err)
		}
		waitFor(name, EventValue)
	}

	ids, err := actor.Elements()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool, len(created))
	for _, id := range ids {
		if strings.HasPrefix(id.Name, prefix) {
			found[id.Name] = true
		}
	}
	if len(found) != len(created) {
		t.Fatalf("enumeration found %d of %d temporary controls", len(found), len(created))
	}

	for len(created) > 0 {
		last := len(created) - 1
		id := created[last]
		if err := actor.RemoveUserElement(id); err != nil {
			t.Fatal(err)
		}
		created = created[:last]
		waitFor(id.Name, EventRemove)
	}
}

func TestUserElementHardwareLifecycle(t *testing.T) {
	selector := os.Getenv("GO_ALSA_TEST_USER_CONTROL")
	if selector == "" {
		t.Skip("set GO_ALSA_TEST_USER_CONTROL to a card number or ID")
	}
	handle, err := Open(selector)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	id := ElementID{
		Interface: InterfaceMixer,
		Name:      fmt.Sprintf("go-alsa-%d-%d", os.Getpid(), time.Now().UnixNano()%1_000_000_000),
	}
	current, err := handle.AddUserElement(UserElementSpec{ID: id, Type: TypeBoolean, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	t.Cleanup(func() {
		if !removed {
			if err := handle.RemoveUserElement(current); err != nil {
				t.Errorf("cleanup user element: %v", err)
			}
		}
	})
	check := func(wantType ElementType, wantCount uint32) {
		t.Helper()
		info, err := handle.Info(current)
		if err != nil {
			t.Fatal(err)
		}
		if info.Type != wantType || info.Count != wantCount || info.Access&AccessUser == 0 {
			t.Fatalf("user info = type %s count %d access %#x", info.Type, info.Count, info.Access)
		}
	}
	replace := func(spec UserElementSpec) {
		t.Helper()
		spec.ID = current
		current, err = handle.ReplaceUserElement(spec)
		if err != nil {
			t.Fatal(err)
		}
	}

	check(TypeBoolean, 2)
	if err := handle.Write(ElementValue{ID: current, Type: TypeBoolean, Booleans: []bool{true, false}}); err != nil {
		t.Fatal(err)
	}
	replace(UserElementSpec{Type: TypeInteger, Count: 2, Integer: IntegerRange{Minimum: -2, Maximum: 2, Step: 2}})
	check(TypeInteger, 2)
	if err := handle.Write(ElementValue{ID: current, Type: TypeInteger, Integers: []int64{-2, 2}}); err != nil {
		t.Fatal(err)
	}
	replace(UserElementSpec{Type: TypeInteger64, Count: 1, Integer: IntegerRange{Minimum: -1 << 40, Maximum: 1 << 40, Step: 1}})
	check(TypeInteger64, 1)
	if err := handle.Write(ElementValue{ID: current, Type: TypeInteger64, Integers: []int64{1 << 39}}); err != nil {
		t.Fatal(err)
	}
	replace(UserElementSpec{Type: TypeEnumerated, Count: 1, EnumeratedItems: []string{"Mic", "Line"}})
	check(TypeEnumerated, 1)
	item, err := handle.EnumeratedItem(current, 1)
	if err != nil || item.Enumerated.ItemName != "Line" {
		t.Fatalf("enumerated item = %q, %v", item.Enumerated.ItemName, err)
	}
	if err := handle.Write(ElementValue{ID: current, Type: TypeEnumerated, Enumerated: []uint32{1}}); err != nil {
		t.Fatal(err)
	}
	replace(UserElementSpec{Type: TypeBytes, Count: 4})
	check(TypeBytes, 4)
	if err := handle.Write(ElementValue{ID: current, Type: TypeBytes, Bytes: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatal(err)
	}
	replace(UserElementSpec{Type: TypeIEC958, Count: 1})
	check(TypeIEC958, 1)
	if err := handle.Write(ElementValue{ID: current, Type: TypeIEC958, IEC958: make([]byte, IEC958StatusBytes)}); err != nil {
		t.Fatal(err)
	}

	if err := handle.RemoveUserElement(current); err != nil {
		t.Fatal(err)
	}
	removed = true
}

func TestEncodeUserElementSpecRejectsInvalid(t *testing.T) {
	base := ElementID{Interface: InterfaceMixer, Name: "go-alsa test"}
	tests := []UserElementSpec{
		{ID: ElementID{Interface: InterfaceMixer}, Type: TypeBoolean, Count: 1},
		{ID: base, Type: TypeBoolean, Count: 0},
		{ID: base, Type: TypeBoolean, Count: 129},
		{ID: base, Type: TypeInteger, Count: 1, Integer: IntegerRange{Minimum: 0, Maximum: 10, Step: 3}},
		{ID: base, Type: TypeInteger64, Count: 65, Integer: IntegerRange{Maximum: 1}},
		{ID: base, Type: TypeEnumerated, Count: 1},
		{ID: base, Type: TypeEnumerated, Count: 1, EnumeratedItems: []string{"bad\x00name"}},
		{ID: base, Type: TypeBytes, Count: 513},
		{ID: base, Type: TypeIEC958, Count: 2},
		{ID: base, Type: TypeNone, Count: 1},
	}
	for index, spec := range tests {
		if _, _, err := encodeUserElementSpec(spec); err == nil {
			t.Errorf("invalid spec %d unexpectedly accepted: %+v", index, spec)
		}
	}
}
