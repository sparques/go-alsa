package control

import "testing"

func TestParseTLV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		words []uint32
		check func(*testing.T, TLV)
	}{
		{"scale", []uint32{uint32(TLVDBScale), 8, signedWord(-3000), 50 | 0x10000}, func(t *testing.T, got TLV) {
			if got.DB == nil || got.DB.Minimum != -3000 || got.DB.Step != 50 || !got.DB.Mute {
				t.Fatalf("scale = %#v", got)
			}
		}},
		{"minmax", []uint32{uint32(TLVDBMinMax), 8, signedWord(-2837), signedWord(-6)}, func(t *testing.T, got TLV) {
			if got.DB == nil || got.DB.Minimum != -2837 || got.DB.Maximum != -6 {
				t.Fatalf("minmax = %#v", got)
			}
		}},
		{"container channel map", []uint32{uint32(TLVContainer), 16, uint32(TLVChannelMapFixed), 8, 3, 4}, func(t *testing.T, got TLV) {
			if len(got.Children) != 1 || len(got.Children[0].ChannelMap) != 2 || got.Children[0].ChannelMap[1] != 4 {
				t.Fatalf("container = %#v", got)
			}
		}},
		{"range", []uint32{uint32(TLVDBRange), 48, 0, 10, uint32(TLVDBScale), 8, signedWord(-1000), 100, 11, 20, uint32(TLVDBMinMax), 8, 0, 1000}, func(t *testing.T, got TLV) {
			if len(got.DBRanges) != 2 || got.DBRanges[1].MinimumValue != 11 || got.DBRanges[1].Curve.DB.Maximum != 1000 {
				t.Fatalf("range = %#v", got)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseTLV(test.words)
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, got)
		})
	}
}

func signedWord(value int32) uint32 { return uint32(value) }

func TestParseTLVRejectsMalformedData(t *testing.T) {
	t.Parallel()
	for _, words := range [][]uint32{
		nil,
		{1, 3},
		{1, 8, 0},
		{uint32(TLVContainer), 4, 1},
		{uint32(TLVDBRange), 12, 0, 1, 2},
	} {
		if _, err := ParseTLV(words); err == nil {
			t.Errorf("ParseTLV(%v) succeeded", words)
		}
	}
}
