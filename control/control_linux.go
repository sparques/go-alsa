//go:build linux && (amd64 || arm64)

package control

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	alsa "github.com/sparques/go-alsa"
	"github.com/sparques/go-alsa/internal/sysio"
	"github.com/sparques/go-alsa/internal/uapi"
	"golang.org/x/sys/unix"
)

const maximumElements = 1 << 20
const maximumTLVBytes = 1 << 20
const maximumEnumNamesBytes = 1 << 16

type handleCore struct{ fd *sysio.FD }

// Open opens a card control by numeric card or stable card ID. Accepted forms
// include "3", "Device", "hw:3", and "hw:CARD=Device".
func Open(selector string) (*Handle, error) {
	number, err := resolveCard(selector)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/dev/snd/controlC%d", number)
	fd, err := sysio.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK)
	if err != nil {
		return nil, &alsa.DeviceError{Op: "open control", Device: selector, Err: err}
	}
	return &Handle{core: handleCore{fd: fd}}, nil
}

func resolveCard(selector string) (int, error) {
	original := selector
	selector = strings.TrimPrefix(selector, "hw:")
	selector = strings.TrimPrefix(selector, "CARD=")
	if selector == "" || strings.Contains(selector, ",") {
		return 0, &alsa.DeviceError{Op: "parse control", Device: original, Err: alsa.ErrMalformedDevice}
	}
	if number, err := strconv.Atoi(selector); err == nil {
		if number < 0 {
			return 0, &alsa.DeviceError{Op: "parse control", Device: original, Err: alsa.ErrMalformedDevice}
		}
		return number, nil
	}
	cards, err := alsa.Cards()
	if err != nil {
		return 0, err
	}
	for _, card := range cards {
		if card.ID == selector {
			return card.Number, nil
		}
	}
	return 0, &alsa.DeviceError{Op: "resolve control", Device: original, Err: alsa.ErrDeviceNotFound}
}

func (h *handleCore) close() error {
	if h.fd == nil {
		return nil
	}
	return h.fd.Close()
}

// Elements returns full IDs in kernel enumeration order. It retries if the
// list grows between the count and data queries.
func (h *Handle) Elements() ([]ElementID, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var list uapi.ControlElementList
		if err := h.core.fd.Ioctl(uapi.ControlIoctlElementList, unsafe.Pointer(&list)); err != nil {
			return nil, fmt.Errorf("alsa control: count elements: %w", err)
		}
		if list.Count > maximumElements {
			return nil, fmt.Errorf("alsa control: kernel reported unreasonable element count %d", list.Count)
		}
		if list.Count == 0 {
			return nil, nil
		}
		ids := make([]uapi.ControlElementID, list.Count)
		list.Space = uint32(len(ids))
		list.IDs = uintptr(unsafe.Pointer(unsafe.SliceData(ids)))
		if err := h.core.fd.Ioctl(uapi.ControlIoctlElementList, unsafe.Pointer(&list)); err != nil {
			runtime.KeepAlive(ids)
			return nil, fmt.Errorf("alsa control: list elements: %w", err)
		}
		runtime.KeepAlive(ids)
		if list.Count > list.Space {
			continue
		}
		if list.Used > list.Space {
			return nil, errors.New("alsa control: kernel returned more elements than requested")
		}
		result := make([]ElementID, list.Used)
		for index := range result {
			result[index] = elementIDFromUAPI(ids[index])
		}
		return result, nil
	}
	return nil, errors.New("alsa control: element list kept changing")
}

// Info returns element metadata. For enumerated controls it reports item zero;
// use EnumeratedItem to retrieve other item names.
func (h *Handle) Info(id ElementID) (ElementInfo, error) {
	return h.info(id, 0)
}

// EnumeratedItem returns metadata and the name for one enumerated item.
func (h *Handle) EnumeratedItem(id ElementID, item uint32) (ElementInfo, error) {
	return h.info(id, item)
}

func (h *Handle) info(id ElementID, item uint32) (ElementInfo, error) {
	raw := uapi.ControlElementInfo{ID: elementIDToUAPI(id)}
	binary.NativeEndian.PutUint32(raw.Value[4:8], item)
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementInfo, unsafe.Pointer(&raw)); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			return ElementInfo{}, err
		}
		return ElementInfo{}, fmt.Errorf("alsa control: element info %s: %w", id, err)
	}
	info := ElementInfo{
		ID: elementIDFromUAPI(raw.ID), Type: ElementType(raw.Type),
		Access: Access(raw.Access), Count: raw.Count, OwnerPID: raw.Owner,
	}
	switch info.Type {
	case TypeBoolean, TypeInteger:
		info.Integer = IntegerRange{
			Minimum: int64(binary.NativeEndian.Uint64(raw.Value[0:8])),
			Maximum: int64(binary.NativeEndian.Uint64(raw.Value[8:16])),
			Step:    int64(binary.NativeEndian.Uint64(raw.Value[16:24])),
		}
	case TypeInteger64:
		info.Integer = IntegerRange{
			Minimum: int64(binary.NativeEndian.Uint64(raw.Value[0:8])),
			Maximum: int64(binary.NativeEndian.Uint64(raw.Value[8:16])),
			Step:    int64(binary.NativeEndian.Uint64(raw.Value[16:24])),
		}
	case TypeEnumerated:
		info.Enumerated = EnumeratedInfo{
			Items: binary.NativeEndian.Uint32(raw.Value[0:4]), SelectedItem: binary.NativeEndian.Uint32(raw.Value[4:8]),
			ItemName: cString(raw.Value[8:72]),
		}
	}
	return info, nil
}

// Read retrieves a typed control value after validating its metadata and
// kernel-reported count.
func (h *Handle) Read(id ElementID) (ElementValue, error) {
	info, err := h.Info(id)
	if err != nil {
		return ElementValue{}, err
	}
	if !info.Access.Readable() {
		return ElementValue{}, fmt.Errorf("alsa control: element %s is not readable", id)
	}
	raw := uapi.ControlElementValue{ID: elementIDToUAPI(id)}
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementRead, unsafe.Pointer(&raw)); err != nil {
		return ElementValue{}, fmt.Errorf("alsa control: read element %s: %w", id, err)
	}
	value := ElementValue{ID: elementIDFromUAPI(raw.ID), Type: info.Type}
	switch info.Type {
	case TypeBoolean:
		if info.Count > 128 {
			return ElementValue{}, invalidCount(info)
		}
		value.Booleans = make([]bool, info.Count)
		for index := range value.Booleans {
			value.Booleans[index] = int64(binary.NativeEndian.Uint64(raw.Value[index*8:])) != 0
		}
	case TypeInteger:
		if info.Count > 128 {
			return ElementValue{}, invalidCount(info)
		}
		value.Integers = decodeIntegers(raw.Value[:], info.Count)
	case TypeInteger64:
		if info.Count > 64 {
			return ElementValue{}, invalidCount(info)
		}
		value.Integers = decodeIntegers(raw.Value[:], info.Count)
	case TypeEnumerated:
		if info.Count > 128 {
			return ElementValue{}, invalidCount(info)
		}
		value.Enumerated = make([]uint32, info.Count)
		for index := range value.Enumerated {
			value.Enumerated[index] = binary.NativeEndian.Uint32(raw.Value[index*4:])
		}
	case TypeBytes:
		if info.Count > 512 {
			return ElementValue{}, invalidCount(info)
		}
		value.Bytes = append([]byte(nil), raw.Value[:info.Count]...)
	case TypeIEC958:
		value.IEC958 = append([]byte(nil), raw.Value[:IEC958StatusBytes]...)
	default:
		return ElementValue{}, fmt.Errorf("alsa control: unsupported element type %s", info.Type)
	}
	return value, nil
}

// Write validates and writes one typed control value.
func (h *Handle) Write(value ElementValue) error {
	info, err := h.Info(value.ID)
	if err != nil {
		return err
	}
	if !info.Access.Writable() {
		return fmt.Errorf("alsa control: element %s is not writable", value.ID)
	}
	if info.Access.Inactive() {
		return fmt.Errorf("alsa control: element %s is inactive", value.ID)
	}
	if value.Type != info.Type {
		return fmt.Errorf("alsa control: value type %s does not match element type %s", value.Type, info.Type)
	}
	raw := uapi.ControlElementValue{ID: elementIDToUAPI(value.ID)}
	switch info.Type {
	case TypeBoolean:
		if len(value.Booleans) != int(info.Count) || info.Count > 128 {
			return valueCountError(info, len(value.Booleans))
		}
		for index, item := range value.Booleans {
			if item {
				binary.NativeEndian.PutUint64(raw.Value[index*8:], 1)
			}
		}
	case TypeInteger, TypeInteger64:
		maximum := uint32(128)
		if info.Type == TypeInteger64 {
			maximum = 64
		}
		if len(value.Integers) != int(info.Count) || info.Count > maximum {
			return valueCountError(info, len(value.Integers))
		}
		for index, item := range value.Integers {
			if item < info.Integer.Minimum || item > info.Integer.Maximum || info.Integer.Step > 0 && (item-info.Integer.Minimum)%info.Integer.Step != 0 {
				return fmt.Errorf("alsa control: integer value %d is outside range %+v", item, info.Integer)
			}
			binary.NativeEndian.PutUint64(raw.Value[index*8:], uint64(item))
		}
	case TypeEnumerated:
		if len(value.Enumerated) != int(info.Count) || info.Count > 128 {
			return valueCountError(info, len(value.Enumerated))
		}
		for index, item := range value.Enumerated {
			if item >= info.Enumerated.Items {
				return fmt.Errorf("alsa control: enumerated item %d is outside 0..%d", item, info.Enumerated.Items-1)
			}
			binary.NativeEndian.PutUint32(raw.Value[index*4:], item)
		}
	case TypeBytes:
		if len(value.Bytes) != int(info.Count) || info.Count > 512 {
			return valueCountError(info, len(value.Bytes))
		}
		copy(raw.Value[:], value.Bytes)
	case TypeIEC958:
		if len(value.IEC958) != IEC958StatusBytes {
			return fmt.Errorf("alsa control: IEC958 value has %d bytes, need %d", len(value.IEC958), IEC958StatusBytes)
		}
		copy(raw.Value[:], value.IEC958)
	default:
		return fmt.Errorf("alsa control: unsupported element type %s", info.Type)
	}
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementWrite, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("alsa control: write element %s: %w", value.ID, err)
	}
	return nil
}

// Lock obtains the kernel write lock for an element. Locks are also released
// when the control handle closes.
func (h *Handle) Lock(id ElementID) error {
	raw := elementIDToUAPI(id)
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementLock, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("alsa control: lock element %s: %w", id, err)
	}
	return nil
}

// Unlock releases a kernel element write lock held by this handle.
func (h *Handle) Unlock(id ElementID) error {
	raw := elementIDToUAPI(id)
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementUnlock, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("alsa control: unlock element %s: %w", id, err)
	}
	return nil
}

func (h *Handle) powerState() (PowerState, error) {
	var state int32
	if err := h.core.fd.Ioctl(uapi.ControlIoctlPowerState, unsafe.Pointer(&state)); err != nil {
		return 0, fmt.Errorf("alsa control: read power state: %w", err)
	}
	return PowerState(state), nil
}

func (h *Handle) setPowerState(state PowerState) error {
	switch state {
	case PowerD0, PowerD1, PowerD2, PowerD3Hot, PowerD3Cold:
	default:
		return fmt.Errorf("alsa control: invalid power state %s", state)
	}
	value := int32(state)
	if err := h.core.fd.Ioctl(uapi.ControlIoctlPower, unsafe.Pointer(&value)); err != nil {
		return fmt.Errorf("alsa control: set power state %s: %w", state, err)
	}
	return nil
}

func (h *Handle) addOrReplaceUserElement(spec UserElementSpec, replace bool) (ElementID, error) {
	if !replace && spec.ID.Number != 0 {
		return ElementID{}, errors.New("alsa control: new user element numid must be zero")
	}
	raw, names, err := encodeUserElementSpec(spec)
	if err != nil {
		return ElementID{}, err
	}
	request := uapi.ControlIoctlElementAdd
	operation := "add"
	if replace {
		info, err := h.Info(spec.ID)
		if err != nil {
			return ElementID{}, err
		}
		if info.Access&AccessUser == 0 {
			return ElementID{}, fmt.Errorf("alsa control: element %s is not userspace-owned", spec.ID)
		}
		request = uapi.ControlIoctlElementReplace
		operation = "replace"
	}
	if err := h.core.fd.Ioctl(request, unsafe.Pointer(&raw)); err != nil {
		runtime.KeepAlive(names)
		return ElementID{}, fmt.Errorf("alsa control: %s user element %s: %w", operation, spec.ID, err)
	}
	runtime.KeepAlive(names)
	return elementIDFromUAPI(raw.ID), nil
}

func (h *Handle) removeUserElement(id ElementID) error {
	info, err := h.Info(id)
	if err != nil {
		return err
	}
	if info.Access&AccessUser == 0 {
		return fmt.Errorf("alsa control: element %s is not userspace-owned", id)
	}
	raw := elementIDToUAPI(id)
	if err := h.core.fd.Ioctl(uapi.ControlIoctlElementRemove, unsafe.Pointer(&raw)); err != nil {
		return fmt.Errorf("alsa control: remove user element %s: %w", id, err)
	}
	return nil
}

func encodeUserElementSpec(spec UserElementSpec) (uapi.ControlElementInfo, []byte, error) {
	if spec.ID.Interface < InterfaceCard || spec.ID.Interface > InterfaceSequencer {
		return uapi.ControlElementInfo{}, nil, fmt.Errorf("alsa control: invalid user element interface %d", spec.ID.Interface)
	}
	if spec.ID.Name == "" || len(spec.ID.Name) >= len(uapi.ControlElementID{}.Name) || strings.IndexByte(spec.ID.Name, 0) >= 0 {
		return uapi.ControlElementInfo{}, nil, fmt.Errorf("alsa control: invalid user element name %q", spec.ID.Name)
	}
	if spec.Count == 0 {
		return uapi.ControlElementInfo{}, nil, errors.New("alsa control: user element count must be positive")
	}
	raw := uapi.ControlElementInfo{ID: elementIDToUAPI(spec.ID), Type: int32(spec.Type), Count: spec.Count}
	var names []byte
	switch spec.Type {
	case TypeBoolean:
		if spec.Count > 128 {
			return raw, nil, fmt.Errorf("alsa control: boolean user element count %d exceeds 128", spec.Count)
		}
	case TypeInteger, TypeInteger64:
		maximum := uint32(128)
		if spec.Type == TypeInteger64 {
			maximum = 64
		}
		if spec.Count > maximum {
			return raw, nil, fmt.Errorf("alsa control: integer user element count %d exceeds %d", spec.Count, maximum)
		}
		span := uint64(spec.Integer.Maximum) - uint64(spec.Integer.Minimum)
		if spec.Integer.Minimum > spec.Integer.Maximum || spec.Integer.Step < 0 || spec.Integer.Step > 0 && span%uint64(spec.Integer.Step) != 0 {
			return raw, nil, fmt.Errorf("alsa control: invalid user element integer range %+v", spec.Integer)
		}
		binary.NativeEndian.PutUint64(raw.Value[0:8], uint64(spec.Integer.Minimum))
		binary.NativeEndian.PutUint64(raw.Value[8:16], uint64(spec.Integer.Maximum))
		binary.NativeEndian.PutUint64(raw.Value[16:24], uint64(spec.Integer.Step))
	case TypeEnumerated:
		if spec.Count > 128 || len(spec.EnumeratedItems) == 0 || uint64(len(spec.EnumeratedItems)) > uint64(^uint32(0)) {
			return raw, nil, errors.New("alsa control: invalid enumerated user element count or items")
		}
		for _, name := range spec.EnumeratedItems {
			if name == "" || len(name) > 63 || strings.IndexByte(name, 0) >= 0 || len(names)+len(name)+1 > maximumEnumNamesBytes {
				return raw, nil, fmt.Errorf("alsa control: invalid enumerated item name %q", name)
			}
			names = append(names, name...)
			names = append(names, 0)
		}
		binary.NativeEndian.PutUint32(raw.Value[0:4], uint32(len(spec.EnumeratedItems)))
		binary.NativeEndian.PutUint64(raw.Value[72:80], uint64(uintptr(unsafe.Pointer(unsafe.SliceData(names)))))
		binary.NativeEndian.PutUint32(raw.Value[80:84], uint32(len(names)))
	case TypeBytes:
		if spec.Count > 512 {
			return raw, nil, fmt.Errorf("alsa control: byte user element count %d exceeds 512", spec.Count)
		}
	case TypeIEC958:
		if spec.Count != 1 {
			return raw, nil, errors.New("alsa control: IEC958 user element count must be one")
		}
	default:
		return raw, nil, fmt.Errorf("alsa control: unsupported user element type %s", spec.Type)
	}
	return raw, names, nil
}

// ReadTLV reads up to maximumBytes of raw TLV words. The returned slice begins
// with the ALSA TLV type and payload-length words.
func (h *Handle) ReadTLV(id ElementID, maximumBytes int) ([]uint32, error) {
	info, err := h.Info(id)
	if err != nil {
		return nil, err
	}
	if !info.Access.TLVReadable() {
		return nil, fmt.Errorf("alsa control: element %s has no readable TLV", id)
	}
	if maximumBytes < 8 || maximumBytes > maximumTLVBytes || maximumBytes%4 != 0 {
		return nil, fmt.Errorf("alsa control: TLV maximum must be aligned and between 8 and %d bytes", maximumTLVBytes)
	}
	buffer := make([]byte, 8+maximumBytes)
	binary.NativeEndian.PutUint32(buffer[0:4], id.Number)
	binary.NativeEndian.PutUint32(buffer[4:8], uint32(maximumBytes))
	if err := h.core.fd.Ioctl(uapi.ControlIoctlTLVRead, unsafe.Pointer(unsafe.SliceData(buffer))); err != nil {
		runtime.KeepAlive(buffer)
		return nil, fmt.Errorf("alsa control: read TLV for %s: %w", id, err)
	}
	runtime.KeepAlive(buffer)
	length := int(binary.NativeEndian.Uint32(buffer[12:16])) + 8
	if length < 8 || length > maximumBytes || length%4 != 0 {
		return nil, fmt.Errorf("alsa control: kernel returned invalid TLV length %d", length)
	}
	words := make([]uint32, length/4)
	for index := range words {
		words[index] = binary.NativeEndian.Uint32(buffer[8+index*4:])
	}
	return words, nil
}

// ReadDecodedTLV reads and decodes standard dB, container, range, and
// channel-map forms.
func (h *Handle) ReadDecodedTLV(id ElementID, maximumBytes int) (TLV, error) {
	words, err := h.ReadTLV(id, maximumBytes)
	if err != nil {
		return TLV{}, err
	}
	return ParseTLV(words)
}

// WriteTLV writes raw TLV words, beginning with type and payload length.
func (h *Handle) WriteTLV(id ElementID, words []uint32) error {
	return h.writeTLV(id, words, false)
}

// CommandTLV sends a raw TLV command to an element.
func (h *Handle) CommandTLV(id ElementID, words []uint32) error {
	return h.writeTLV(id, words, true)
}

func (h *Handle) writeTLV(id ElementID, words []uint32, command bool) error {
	info, err := h.Info(id)
	if err != nil {
		return err
	}
	if command && !info.Access.TLVCommandable() {
		return fmt.Errorf("alsa control: element %s accepts no TLV commands", id)
	}
	if !command && !info.Access.TLVWritable() {
		return fmt.Errorf("alsa control: element %s has no writable TLV", id)
	}
	length := len(words) * 4
	if length < 8 || length > maximumTLVBytes {
		return fmt.Errorf("alsa control: TLV payload must be between 8 and %d bytes", maximumTLVBytes)
	}
	if int(words[1])+8 != length {
		return fmt.Errorf("alsa control: TLV length word %d does not match %d-byte value", words[1], length)
	}
	buffer := make([]byte, 8+length)
	binary.NativeEndian.PutUint32(buffer[0:4], id.Number)
	binary.NativeEndian.PutUint32(buffer[4:8], uint32(length))
	for index, word := range words {
		binary.NativeEndian.PutUint32(buffer[8+index*4:], word)
	}
	request := uapi.ControlIoctlTLVWrite
	operation := "write"
	if command {
		request = uapi.ControlIoctlTLVCommand
		operation = "command"
	}
	if err := h.core.fd.Ioctl(request, unsafe.Pointer(unsafe.SliceData(buffer))); err != nil {
		runtime.KeepAlive(buffer)
		return fmt.Errorf("alsa control: %s TLV for %s: %w", operation, id, err)
	}
	runtime.KeepAlive(buffer)
	return nil
}

func (h *Handle) subscribe(enabled bool) error {
	value := int32(0)
	if enabled {
		value = 1
	}
	if err := h.core.fd.Ioctl(uapi.ControlIoctlSubscribeEvents, unsafe.Pointer(&value)); err != nil {
		return fmt.Errorf("alsa control: subscribe events: %w", err)
	}
	return nil
}

func (h *Handle) nextEvent(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		var raw uapi.ControlEvent
		buffer := unsafe.Slice((*byte)(unsafe.Pointer(&raw)), int(unsafe.Sizeof(raw)))
		count, err := h.core.fd.Read(buffer)
		runtime.KeepAlive(&raw)
		if err == nil {
			if count != len(buffer) {
				return Event{}, fmt.Errorf("alsa control: short event read %d of %d bytes", count, len(buffer))
			}
			if raw.Type != 0 {
				return Event{}, fmt.Errorf("alsa control: unsupported event type %d", raw.Type)
			}
			return Event{Mask: EventMask(raw.Mask), ID: elementIDFromUAPI(raw.ID)}, nil
		}
		if !errors.Is(err, syscall.EAGAIN) {
			return Event{}, fmt.Errorf("alsa control: read event: %w", err)
		}
		revents, err := h.core.fd.Wait(ctx, unix.POLLIN)
		if err != nil {
			return Event{}, err
		}
		if revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return Event{}, fmt.Errorf("alsa control: event poll error %#x", revents)
		}
	}
}

func decodeIntegers(raw []byte, count uint32) []int64 {
	values := make([]int64, count)
	for index := range values {
		values[index] = int64(binary.NativeEndian.Uint64(raw[index*8:]))
	}
	return values
}

func invalidCount(info ElementInfo) error {
	return fmt.Errorf("alsa control: kernel count %d is invalid for %s", info.Count, info.Type)
}

func valueCountError(info ElementInfo, got int) error {
	return fmt.Errorf("alsa control: got %d values for %s, need %d", got, info.Type, info.Count)
}

func elementIDFromUAPI(id uapi.ControlElementID) ElementID {
	return ElementID{Number: id.Number, Interface: Interface(id.Interface), Device: id.Device, Subdevice: id.Subdevice, Name: cString(id.Name[:]), Index: id.Index}
}

func elementIDToUAPI(id ElementID) uapi.ControlElementID {
	raw := uapi.ControlElementID{Number: id.Number, Interface: int32(id.Interface), Device: id.Device, Subdevice: id.Subdevice, Index: id.Index}
	copy(raw.Name[:], id.Name)
	return raw
}

func cString(value []byte) string {
	for index, character := range value {
		if character == 0 {
			return string(value[:index])
		}
	}
	return string(value)
}
