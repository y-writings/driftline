//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitAtomicGitIgnoreReplacementRenamesPreparedFileOverTarget(t *testing.T) {
	root := t.TempDir()
	tempPath := filepath.Join(root, ".prepared")
	targetPath := filepath.Join(root, GitIgnorePath)
	if err := os.WriteFile(tempPath, []byte("desired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tempInfo, err := os.Lstat(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := commitAtomicGitIgnoreReplacement(tempPath, targetPath); err != nil {
		t.Fatalf("commit atomic replacement: %v", err)
	}
	targetInfo, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(tempInfo, targetInfo) {
		t.Fatal("target is not the atomically renamed prepared file")
	}
}

func TestPrepareGitIgnoreRewriteRenameFailureLeavesTempForCleanup(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	original := []byte("original\n")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	commit, cleanup, err := prepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: original,
		DesiredBytes:  []byte("desired\n"),
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	tempPath := singleGitIgnoreRewriteTemp(t, root)
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(targetPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := commit(); err == nil {
		t.Fatal("expected rename failure")
	}
	if _, err := os.Lstat(tempPath); err != nil {
		t.Fatalf("failed commit removed temp before cleanup: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup after failed commit: %v", err)
	}
	if _, err := os.Lstat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup left failed-commit temp: %v", err)
	}
}
