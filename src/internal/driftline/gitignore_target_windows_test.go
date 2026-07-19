//go:build windows

package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareGitIgnoreRewriteFailsClosedBeforeCreatingTemp(t *testing.T) {
	if err := validateAtomicGitIgnoreReplacement(); err == nil || !strings.Contains(err.Error(), "unsupported on windows") {
		t.Fatalf("expected Windows atomic replacement rejection, got %v", err)
	}

	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		TargetMissing: true,
		DesiredBytes:  []byte("desired\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "atomic .gitignore replacement is unsupported on windows") {
		t.Fatalf("expected unsupported atomic replacement error, got %v", err)
	}
	if commit != nil || cleanup != nil {
		t.Fatal("unsupported preparation returned commit or cleanup")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported preparation created files: %v", entries)
	}
}

func TestTargetRepositoryApplyRejectsUnsupportedGitIgnoreBeforeManagedWrite(t *testing.T) {
	root := t.TempDir()
	managedPath := filepath.Join(root, "managed.txt")
	if err := os.WriteFile(managedPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		Changes: []Change{{
			ID:           "tool.config",
			TargetPath:   managedPath,
			SourceBytes:  []byte("changed\n"),
			Status:       StatusUpdate,
			WritesTarget: true,
		}},
		GitIgnore: &GitIgnoreSectionChange{
			TargetPath:    filepath.Join(root, GitIgnorePath),
			TargetMissing: true,
			DesiredBytes:  []byte("desired\n"),
		},
	}

	err := (TargetRepository{Root: root}).Apply(plan)
	if err == nil || !strings.Contains(err.Error(), "atomic .gitignore replacement is unsupported on windows") {
		t.Fatalf("expected unsupported atomic replacement error, got %v", err)
	}
	data, readErr := os.ReadFile(managedPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "original\n" {
		t.Fatalf("unsupported Gitignore apply changed Managed file: %q", data)
	}
}

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
