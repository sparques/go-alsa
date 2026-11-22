//go:build linux && (amd64 || arm64)

package main

import "testing"

func TestParseValues(t *testing.T) {
	t.Parallel()
	values, err := parseValues([]byte("decimal=12\nhex=0xff\n"))
	if err != nil {
		t.Fatal(err)
	}
	if values["decimal"] != 12 || values["hex"] != 255 {
		t.Fatalf("values = %#v", values)
	}
}
