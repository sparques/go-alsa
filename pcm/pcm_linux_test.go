//go:build linux && (amd64 || arm64)

package pcm

import "testing"

func TestCString(t *testing.T) {
	t.Parallel()
	if got := cString([]byte{'p', 'c', 'm', 0, 'x'}); got != "pcm" {
		t.Fatalf("cString = %q; want pcm", got)
	}
}
