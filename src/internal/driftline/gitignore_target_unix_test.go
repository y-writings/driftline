//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadRegularFileNoFollowRejectsFinalSymlink(t *testing.T) {
	root := t.TempDir()
	externalPath := filepath.Join(root, "external")
	if err := os.WriteFile(externalPath, []byte("external bytes must not be read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, GitIgnorePath)
	if err := os.Symlink(externalPath, targetPath); err != nil {
		t.Fatal(err)
	}

	got, _, err := readRegularFileNoFollow(targetPath)
	if err == nil {
		t.Fatalf("expected final symlink rejection, read %q", got)
	}
	if !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("expected no-follow symlink error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("final symlink bytes must not be read: %q", got)
	}
}
