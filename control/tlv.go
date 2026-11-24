package control

import "fmt"

type TLVType uint32

// DBGainMute is ALSA's hundredths-of-a-decibel sentinel for complete mute.
const DBGainMute int32 = -9999999

const (
	TLVContainer          TLVType = 0
	TLVDBScale            TLVType = 1
	TLVDBLinear           TLVType = 2
	TLVDBRange            TLVType = 3
	TLVDBMinMax           TLVType = 4
	TLVDBMinMaxMute       TLVType = 5
	TLVChannelMapFixed    TLVType = 0x101
	TLVChannelMapVariable TLVType = 0x102
	TLVChannelMapPaired   TLVType = 0x103
)

// DB describes a dB curve in hundredths of a decibel.
type DB struct {
	Minimum int32
	Maximum int32
	Step    uint16
	Mute    bool
	Linear  bool
}

type DBRange struct {
	MinimumValue int32
	MaximumValue int32
	Curve        TLV
}

// TLV is a bounds-checked decoded ALSA TLV. Unknown types retain Payload.
type TLV struct {
	Type       TLVType
	Payload    []uint32
	Children   []TLV
	DB         *DB
	DBRanges   []DBRange
	ChannelMap []uint32
}

func ParseTLV(words []uint32) (TLV, error) { return parseTLV(words, 0) }

func parseTLV(words []uint32, depth int) (TLV, error) {
	if depth > 32 {
		return TLV{}, fmt.Errorf("alsa control: TLV nesting exceeds 32")
	}
	if len(words) < 2 || words[1]%4 != 0 {
		return TLV{}, fmt.Errorf("alsa control: malformed TLV header")
	}
	want := int(words[1]/4) + 2
	if want != len(words) {
		return TLV{}, fmt.Errorf("alsa control: TLV length describes %d words, got %d", want, len(words))
	}
	payload := append([]uint32(nil), words[2:]...)
	result := TLV{Type: TLVType(words[0]), Payload: payload}
	switch result.Type {
	case TLVContainer:
		children, err := parseTLVSequence(payload, depth+1)
		if err != nil {
			return TLV{}, err
		}
		result.Children = children
	case TLVDBScale:
		if len(payload) != 2 {
			return TLV{}, fmt.Errorf("alsa control: dB scale needs two words")
		}
		result.DB = &DB{Minimum: int32(payload[0]), Step: uint16(payload[1] & 0xffff), Mute: payload[1]&0x10000 != 0}
	case TLVDBLinear, TLVDBMinMax, TLVDBMinMaxMute:
		if len(payload) != 2 {
			return TLV{}, fmt.Errorf("alsa control: dB min/max needs two words")
		}
		result.DB = &DB{Minimum: int32(payload[0]), Maximum: int32(payload[1]), Linear: result.Type == TLVDBLinear, Mute: result.Type == TLVDBMinMaxMute}
	case TLVDBRange:
		for offset := 0; offset < len(payload); {
			if len(payload)-offset < 4 {
				return TLV{}, fmt.Errorf("alsa control: truncated dB range")
			}
			curveWords := int(payload[offset+3]/4) + 2
			if curveWords < 2 || offset+2+curveWords > len(payload) {
				return TLV{}, fmt.Errorf("alsa control: invalid nested dB range length")
			}
			curve, err := parseTLV(payload[offset+2:offset+2+curveWords], depth+1)
			if err != nil {
				return TLV{}, err
			}
			result.DBRanges = append(result.DBRanges, DBRange{MinimumValue: int32(payload[offset]), MaximumValue: int32(payload[offset+1]), Curve: curve})
			offset += 2 + curveWords
		}
	case TLVChannelMapFixed, TLVChannelMapVariable, TLVChannelMapPaired:
		result.ChannelMap = append([]uint32(nil), payload...)
	}
	return result, nil
}

func parseTLVSequence(words []uint32, depth int) ([]TLV, error) {
	var result []TLV
	for offset := 0; offset < len(words); {
		if len(words)-offset < 2 || words[offset+1]%4 != 0 {
			return nil, fmt.Errorf("alsa control: malformed nested TLV")
		}
		length := int(words[offset+1]/4) + 2
		if length < 2 || offset+length > len(words) {
			return nil, fmt.Errorf("alsa control: truncated nested TLV")
		}
		item, err := parseTLV(words[offset:offset+length], depth)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
		offset += length
	}
	return result, nil
}
