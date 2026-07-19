//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris || windows

package driftline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRegularFileNoFollowReadsOpenedRegularFile(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), GitIgnorePath)
	want := []byte("local\n")
	if err := os.WriteFile(targetPath, want, 0o600); err != nil {
		t.Fatal(err)
	}

	got, mode, err := readRegularFileNoFollow(targetPath)
	if err != nil {
		t.Fatalf("read regular file without following: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("read bytes = %q, want %q", got, want)
	}
	if mode != 0o600 {
		t.Fatalf("read mode = %#o, want 0600", mode)
	}
}
