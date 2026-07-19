//go:build windows

package driftline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRegularFileNoFollowRejectsFinalReparsePoint(t *testing.T) {
	root := t.TempDir()
	externalPath := filepath.Join(root, "external")
	if err := os.WriteFile(externalPath, []byte("external bytes must not be read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, GitIgnorePath)
	if err := os.Symlink(externalPath, targetPath); err != nil {
		t.Skipf("cannot create Windows symlink: %v", err)
	}

	got, _, err := readRegularFileNoFollow(targetPath)
	if err == nil {
		t.Fatalf("expected final reparse-point rejection, read %q", got)
	}
	if len(got) != 0 {
		t.Fatalf("final reparse-point bytes must not be read: %q", got)
	}
}
