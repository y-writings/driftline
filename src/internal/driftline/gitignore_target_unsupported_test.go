//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRegularFileNoFollowReportsUnsupportedPlatform(t *testing.T) {
	_, _, err := readRegularFileNoFollow(".gitignore")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported-platform error, got %v", err)
	}
}

func TestTargetRepositoryApplyRejectsUnsupportedGitIgnoreBeforeSyncPreparationOnUnsupportedPlatform(t *testing.T) {
	root := t.TempDir()
	managedPath := filepath.Join(root, "managed.txt")
	if err := os.WriteFile(managedPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	syncPrepared := false
	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "managed.txt", Status: StatusSyncManifestAdd},
			{ID: "tool.config", TargetPath: managedPath, SourceBytes: []byte("changed\n"), Status: StatusUpdate, WritesTarget: true},
		},
		GitIgnore: &GitIgnoreSectionChange{TargetPath: filepath.Join(root, GitIgnorePath), TargetMissing: true},
	}
	repository := TargetRepository{
		Root: root,
		prepareSyncManifestRewrite: func(string, SyncManifest) (func() error, func() error, error) {
			syncPrepared = true
			return func() error { return nil }, func() error { return nil }, nil
		},
	}

	err := repository.Apply(plan)
	if err == nil || !strings.Contains(err.Error(), "atomic .gitignore replacement is unsupported") {
		t.Fatalf("expected unsupported atomic replacement error, got %v", err)
	}
	if syncPrepared {
		t.Fatal("unsupported Gitignore apply prepared Sync manifest")
	}
	if _, statErr := os.Lstat(filepath.Join(root, MetadataDirectoryPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsupported Gitignore apply created Sync metadata: %v", statErr)
	}
	data, readErr := os.ReadFile(managedPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "original\n" {
		t.Fatalf("unsupported Gitignore apply changed Managed file: %q", data)
	}
}

func TestPrepareGitIgnoreRewriteReportsUnsupportedAtomicReplacement(t *testing.T) {
	if err := validateAtomicGitIgnoreReplacement(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected platform atomic replacement rejection, got %v", err)
	}

	_, _, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    filepath.Join(t.TempDir(), GitIgnorePath),
		TargetMissing: true,
		DesiredBytes:  []byte("desired\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "atomic .gitignore replacement is unsupported") {
		t.Fatalf("expected unsupported atomic replacement error, got %v", err)
	}
}
