package main

import (
	"reflect"
	"testing"
)

func TestParseFloats(t *testing.T) {
	t.Parallel()
	got, err := parseFloats("0, 0.5,1")
	if err != nil || !reflect.DeepEqual(got, []float64{0, 0.5, 1}) {
		t.Fatalf("parseFloats = %v, %v", got, err)
	}
	for _, value := range []string{"-1", "1.1", "NaN", "Inf", "x"} {
		if _, err := parseFloats(value); err == nil {
			t.Errorf("parseFloats(%q) succeeded", value)
		}
	}
}

func TestParseSwitches(t *testing.T) {
	t.Parallel()
	got, err := parseSwitches("on, 0,TRUE,off")
	if err != nil || !reflect.DeepEqual(got, []bool{true, false, true, false}) {
		t.Fatalf("parseSwitches = %v, %v", got, err)
	}
	if _, err := parseSwitches("maybe"); err == nil {
		t.Fatal("invalid switch succeeded")
	}
}

func TestParseUint32s(t *testing.T) {
	t.Parallel()
	got, err := parseUint32s("0, 42")
	if err != nil || !reflect.DeepEqual(got, []uint32{0, 42}) {
		t.Fatalf("parseUint32s = %v, %v", got, err)
	}
	for _, value := range []string{"-1", "4294967296", "x"} {
		if _, err := parseUint32s(value); err == nil {
			t.Errorf("parseUint32s(%q) succeeded", value)
		}
	}
}

func TestActionCount(t *testing.T) {
	t.Parallel()
	if got := actionCount(options{}); got != 0 {
		t.Fatalf("empty action count = %d", got)
	}
	if got := actionCount(options{playbackVolume: "1", captureSwitch: "on"}); got != 2 {
		t.Fatalf("action count = %d; want 2", got)
	}
}
