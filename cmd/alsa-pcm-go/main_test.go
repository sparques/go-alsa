package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/sparques/go-alsa/pcm"
)

func TestParseFormat(t *testing.T) {
	t.Parallel()
	tests := map[string]pcm.Format{
		"s8": pcm.S8, "U8": pcm.U8, "s16_le": pcm.S16LE,
		"s24le": pcm.S24LE, "s24_3le": pcm.S24_3LE, "s32le": pcm.S32LE,
	}
	for name, want := range tests {
		got, err := parseFormat(name)
		if err != nil || got != want {
			t.Errorf("parseFormat(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := parseFormat("float"); err == nil {
		t.Fatal("unsupported format succeeded")
	}
}

func TestWriteAll(t *testing.T) {
	t.Parallel()
	writer := &shortWriter{maximum: 2}
	if err := writeAll(writer, []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if got := writer.buffer.String(); got != "abcdef" {
		t.Fatalf("output = %q", got)
	}
	if err := writeAll(zeroWriter{}, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero-progress error = %v; want io.ErrShortWrite", err)
	}
}

type shortWriter struct {
	buffer  bytes.Buffer
	maximum int
}

func (w *shortWriter) Write(buffer []byte) (int, error) {
	if len(buffer) > w.maximum {
		buffer = buffer[:w.maximum]
	}
	return w.buffer.Write(buffer)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
