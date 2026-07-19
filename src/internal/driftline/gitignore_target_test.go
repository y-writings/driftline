package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadRegularFileNoFollowReadsOpenedRegularFile(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), GitIgnorePath)
	want := []byte("local\n")
	if err := os.WriteFile(targetPath, want, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readRegularFileNoFollow(targetPath)
	if err != nil {
		t.Fatalf("read regular file without following: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("read bytes = %q, want %q", got, want)
	}
}

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

	got, err := readRegularFileNoFollow(targetPath)
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

func TestReadRegularFileNoFollowRejectsFIFO(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), GitIgnorePath)
	if err := syscall.Mkfifo(targetPath, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readRegularFileNoFollow(targetPath)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected FIFO regular-file error, got bytes %q and error %v", got, err)
	}
	if len(got) != 0 {
		t.Fatalf("FIFO bytes must not be read: %q", got)
	}
}
