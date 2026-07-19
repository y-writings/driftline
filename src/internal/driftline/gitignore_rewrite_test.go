package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareGitIgnoreRewriteRejectsAppearedMissingTarget(t *testing.T) {
	for _, state := range []string{"regular file", "symlink", "directory"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			targetPath := filepath.Join(root, GitIgnorePath)
			switch state {
			case "regular file":
				if err := os.WriteFile(targetPath, []byte("appeared\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("external bytes must not be read\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, targetPath); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
			case "directory":
				if err := os.Mkdir(targetPath, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
				TargetPath:    targetPath,
				TargetMissing: true,
				DesiredBytes:  []byte("generated\n"),
			})
			if err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("expected stale plan error, got %v", err)
			}
			if commit != nil || cleanup != nil {
				t.Fatal("failed preparation returned commit or cleanup")
			}
			assertNoGitIgnoreRewriteTemp(t, root)
		})
	}
}

func TestPrepareGitIgnoreRewriteRejectsChangedRegularTarget(t *testing.T) {
	const original = "original\n"
	for _, state := range []string{"different bytes", "missing", "symlink", "broken symlink", "directory"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			targetPath := filepath.Join(root, GitIgnorePath)
			if err := os.WriteFile(targetPath, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(targetPath); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "different bytes":
				if err := os.WriteFile(targetPath, []byte("changed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing":
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("external bytes must not be read\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, targetPath); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
			case "broken symlink":
				outside := filepath.Join(t.TempDir(), "missing")
				if err := os.Symlink(outside, targetPath); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
			case "directory":
				if err := os.Mkdir(targetPath, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
				TargetPath:    targetPath,
				OriginalBytes: []byte(original),
				DesiredBytes:  []byte("generated\n"),
			})
			if err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("expected stale plan error, got %v", err)
			}
			if commit != nil || cleanup != nil {
				t.Fatal("failed preparation returned commit or cleanup")
			}
			assertNoGitIgnoreRewriteTemp(t, root)
		})
	}
}

func TestPrepareGitIgnoreRewritePreservesRevalidationErrorCause(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	if err := os.WriteFile(targetPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(targetPath, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(targetPath, 0o600) })
	file, openErr := os.Open(targetPath)
	if openErr == nil {
		file.Close()
		t.Skip("filesystem permits reading a mode 000 file")
	}

	_, _, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: []byte("original\n"),
		DesiredBytes:  []byte("generated\n"),
	})
	if err == nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected wrapped permission error, got %v", err)
	}
	assertNoGitIgnoreRewriteTemp(t, root)
}

func TestPrepareGitIgnoreRewriteDefersAtomicReplacement(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	original := []byte("local\n")
	desired := []byte("local\n\ngenerated\n")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: original,
		DesiredBytes:  desired,
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	tempPath := singleGitIgnoreRewriteTemp(t, root)
	tempInfo, err := os.Lstat(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := readGitIgnoreRewriteTestFile(t, targetPath); string(got) != string(original) {
		t.Fatalf("prepare changed target to %q", got)
	}
	if got := readGitIgnoreRewriteTestFile(t, tempPath); string(got) != string(desired) {
		t.Fatalf("prepared bytes = %q, want %q", got, desired)
	}

	if err := commit(); err != nil {
		t.Fatalf("commit rewrite: %v", err)
	}
	if got := readGitIgnoreRewriteTestFile(t, targetPath); string(got) != string(desired) {
		t.Fatalf("committed bytes = %q, want %q", got, desired)
	}
	targetInfo, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(tempInfo, targetInfo) {
		t.Fatal("commit did not rename the prepared file over the target")
	}
	if _, err := os.Lstat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed temp still exists: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup after commit: %v", err)
	}
}

func TestPrepareGitIgnoreRewriteCleanupRemovesUncommittedTemp(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	original := []byte("local\n")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: original,
		DesiredBytes:  []byte("generated\n"),
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	tempPath := singleGitIgnoreRewriteTemp(t, root)
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup rewrite: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("repeated cleanup rewrite: %v", err)
	}
	if _, err := os.Lstat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncommitted temp still exists: %v", err)
	}
	if got := readGitIgnoreRewriteTestFile(t, targetPath); string(got) != string(original) {
		t.Fatalf("cleanup changed target to %q", got)
	}
}

func TestPrepareGitIgnoreRewritePreservesExistingMode(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	original := []byte("local\n")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(targetPath, 0o640); err != nil {
		t.Fatal(err)
	}

	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: original,
		DesiredBytes:  []byte("generated\n"),
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	defer cleanup()
	assertGitIgnoreRewriteMode(t, singleGitIgnoreRewriteTemp(t, root), 0o640)
	if err := commit(); err != nil {
		t.Fatalf("commit rewrite: %v", err)
	}
	assertGitIgnoreRewriteMode(t, targetPath, 0o640)
}

func TestPrepareGitIgnoreRewriteNewModeMatchesUmaskApplied0644(t *testing.T) {
	referenceRoot := t.TempDir()
	referencePath := filepath.Join(referenceRoot, "reference")
	reference, err := os.OpenFile(referencePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := reference.Close(); err != nil {
		t.Fatal(err)
	}
	referenceInfo, err := os.Lstat(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	wantMode := referenceInfo.Mode().Perm()

	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		TargetMissing: true,
		DesiredBytes:  []byte("generated\n"),
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	defer cleanup()
	assertGitIgnoreRewriteMode(t, singleGitIgnoreRewriteTemp(t, root), wantMode)
	if err := commit(); err != nil {
		t.Fatalf("commit rewrite: %v", err)
	}
	assertGitIgnoreRewriteMode(t, targetPath, wantMode)
}

func TestPrepareGitIgnoreRewriteCommitsEmptyFileInsteadOfDeleting(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	original := []byte("generated\n")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    targetPath,
		OriginalBytes: original,
		DesiredBytes:  []byte{},
	})
	if err != nil {
		t.Fatalf("prepare rewrite: %v", err)
	}
	defer cleanup()
	if err := commit(); err != nil {
		t.Fatalf("commit rewrite: %v", err)
	}
	info, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatalf("empty target missing: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("empty target mode=%v size=%d", info.Mode(), info.Size())
	}
}

func singleGitIgnoreRewriteTemp(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.Name() != GitIgnorePath {
			paths = append(paths, filepath.Join(root, entry.Name()))
		}
	}
	if len(paths) != 1 {
		t.Fatalf("rewrite temp paths = %q, want exactly one", paths)
	}
	return paths[0]
}

func assertNoGitIgnoreRewriteTemp(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != GitIgnorePath {
			t.Fatalf("unexpected rewrite temp %q", entry.Name())
		}
	}
}

func readGitIgnoreRewriteTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertGitIgnoreRewriteMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode of %s = %#o, want %#o", filepath.Base(path), got, want)
	}
}
