//go:build linux && (amd64 || arm64)

package control

import (
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
)

const maximumElements = 1 << 20

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
