package control

import (
	"context"
	"fmt"
	"sync"
)

// Interface identifies the kernel subsystem that owns a control element.
type Interface int32

const (
	InterfaceCard Interface = iota
	InterfaceHardware
	InterfaceMixer
	InterfacePCM
	InterfaceRawMIDI
	InterfaceTimer
	InterfaceSequencer
)

func (i Interface) String() string {
	names := [...]string{"CARD", "HWDEP", "MIXER", "PCM", "RAWMIDI", "TIMER", "SEQUENCER"}
	if i < 0 || int(i) >= len(names) {
		return "UNKNOWN"
	}
	return names[i]
}

// ElementID is the full stable kernel identity of one control element.
type ElementID struct {
	Number    uint32
	Interface Interface
	Device    uint32
	Subdevice uint32
	Name      string
	Index     uint32
}

func (id ElementID) String() string {
	return fmt.Sprintf("numid=%d,iface=%s,name=%q,index=%d,device=%d,subdevice=%d",
		id.Number, id.Interface, id.Name, id.Index, id.Device, id.Subdevice)
}

// ElementType identifies the value representation of a control.
type ElementType int32

const (
	TypeNone ElementType = iota
	TypeBoolean
	TypeInteger
	TypeEnumerated
	TypeBytes
	TypeIEC958
	TypeInteger64
)

func (t ElementType) String() string {
	names := [...]string{"NONE", "BOOLEAN", "INTEGER", "ENUMERATED", "BYTES", "IEC958", "INTEGER64"}
	if t < 0 || int(t) >= len(names) {
		return "UNKNOWN"
	}
	return names[t]
}

// Access is the kernel access-bit mask for a control element.
type Access uint32

const (
	AccessRead        Access = 1 << 0
	AccessWrite       Access = 1 << 1
	AccessVolatile    Access = 1 << 2
	AccessTLVRead     Access = 1 << 4
	AccessTLVWrite    Access = 1 << 5
	AccessTLVCommand  Access = 1 << 6
	AccessInactive    Access = 1 << 8
	AccessLock        Access = 1 << 9
	AccessOwner       Access = 1 << 10
	AccessTLVCallback Access = 1 << 28
	AccessUser        Access = 1 << 29
)

func (a Access) Readable() bool       { return a&AccessRead != 0 }
func (a Access) Writable() bool       { return a&AccessWrite != 0 }
func (a Access) Inactive() bool       { return a&AccessInactive != 0 }
func (a Access) TLVReadable() bool    { return a&AccessTLVRead != 0 }
func (a Access) TLVWritable() bool    { return a&AccessTLVWrite != 0 }
func (a Access) TLVCommandable() bool { return a&AccessTLVCommand != 0 }

// IntegerRange describes integer and integer64 controls.
type IntegerRange struct {
	Minimum int64
	Maximum int64
	Step    int64
}

// EnumeratedInfo describes the selected metadata item returned by the kernel.
type EnumeratedInfo struct {
	Items        uint32
	SelectedItem uint32
	ItemName     string
}

// ElementInfo describes a control element and its type-specific constraints.
type ElementInfo struct {
	ID         ElementID
	Type       ElementType
	Access     Access
	Count      uint32
	OwnerPID   int32
	Integer    IntegerRange
	Enumerated EnumeratedInfo
}

// IEC958StatusBytes is the fixed kernel snd_aes_iec958 representation size.
const IEC958StatusBytes = 176

// ElementValue contains exactly one type-specific value slice according to
// Type. Boolean and integer controls use one entry per Info.Count.
type ElementValue struct {
	ID         ElementID
	Type       ElementType
	Booleans   []bool
	Integers   []int64
	Enumerated []uint32
	Bytes      []byte
	IEC958     []byte
}

// Handle owns an ALSA card control descriptor.
type Handle struct {
	core    handleCore
	eventMu sync.Mutex
}

// Close releases the control descriptor. It is idempotent.
func (h *Handle) Close() error { return h.core.close() }

type EventMask uint32

const (
	EventValue  EventMask = 1 << 0
	EventInfo   EventMask = 1 << 1
	EventAdd    EventMask = 1 << 2
	EventTLV    EventMask = 1 << 3
	EventRemove EventMask = ^EventMask(0)
)

// Event reports a kernel control-element change.
type Event struct {
	Mask EventMask
	ID   ElementID
}

// Subscribe enables or disables delivery of control events to this handle.
func (h *Handle) Subscribe(enabled bool) error { return h.subscribe(enabled) }

// NextEvent waits for one subscribed control event, context cancellation, or
// handle closure. Concurrent calls are serialized to preserve event order.
func (h *Handle) NextEvent(ctx context.Context) (Event, error) {
	h.eventMu.Lock()
	defer h.eventMu.Unlock()
	return h.nextEvent(ctx)
}
