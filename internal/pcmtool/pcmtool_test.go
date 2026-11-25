package pcmtool

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/sparques/go-alsa/pcm"
)

func TestParseFormat(t *testing.T) {
	for name, want := range map[string]pcm.Format{"s8": pcm.S8, "U8": pcm.U8, "s16_le": pcm.S16LE, "s24le": pcm.S24LE, "s24_3le": pcm.S24_3LE, "s32le": pcm.S32LE} {
		got, err := ParseFormat(name)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v", name, got, err)
		}
	}
	if _, err := ParseFormat("float"); err == nil {
		t.Fatal("unsupported format succeeded")
	}
}

func TestWriteAll(t *testing.T) {
	w := &shortWriter{maximum: 2}
	if err := WriteAll(w, []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if w.buffer.String() != "abcdef" {
		t.Fatalf("output = %q", w.buffer.String())
	}
	if err := WriteAll(zeroWriter{}, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v", err)
	}
}

type shortWriter struct {
	buffer  bytes.Buffer
	maximum int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.maximum {
		p = p[:w.maximum]
	}
	return w.buffer.Write(p)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
