//go:build windows

package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsExtendedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "drive absolute", path: `C:\repository\.gitignore`, want: `\\?\C:\repository\.gitignore`},
		{name: "UNC", path: `\\server\share\repository\.gitignore`, want: `\\?\UNC\server\share\repository\.gitignore`},
		{name: "Win32 device", path: `\\.\C:\repository\.gitignore`, want: `\\.\C:\repository\.gitignore`},
		{name: "NT native", path: `\??\C:\repository\.gitignore`, want: `\??\C:\repository\.gitignore`},
		{name: "extended drive", path: `\\?\C:\repository\.gitignore`, want: `\\?\C:\repository\.gitignore`},
		{name: "extended UNC", path: `\\?\UNC\server\share\repository\.gitignore`, want: `\\?\UNC\server\share\repository\.gitignore`},
		{name: "relative", path: `repository\.gitignore`, want: `repository\.gitignore`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowsExtendedPath(tt.path); got != tt.want {
				t.Fatalf("windowsExtendedPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

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

func TestTargetRepositoryApplyRejectsUnsupportedGitIgnoreBeforeSyncPreparationOrManagedWrite(t *testing.T) {
	root := t.TempDir()
	managedPath := filepath.Join(root, "managed.txt")
	if err := os.WriteFile(managedPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "managed.txt", Status: StatusSyncManifestAdd},
			{
				ID:           "tool.config",
				TargetPath:   managedPath,
				SourceBytes:  []byte("changed\n"),
				Status:       StatusUpdate,
				WritesTarget: true,
			},
		},
		GitIgnore: &GitIgnoreSectionChange{
			TargetPath:    filepath.Join(root, GitIgnorePath),
			TargetMissing: true,
			DesiredBytes:  []byte("desired\n"),
		},
	}

	syncPrepared := false
	repository := TargetRepository{
		Root: root,
		prepareSyncManifestRewrite: func(string, SyncManifest) (func() error, func() error, error) {
			syncPrepared = true
			return func() error { return nil }, func() error { return nil }, nil
		},
	}
	err := repository.Apply(plan)
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
	if syncPrepared {
		t.Fatal("unsupported Gitignore apply prepared Sync manifest")
	}
	if _, statErr := os.Lstat(filepath.Join(root, MetadataDirectoryPath)); !os.IsNotExist(statErr) {
		t.Fatalf("unsupported Gitignore apply created Sync metadata: %v", statErr)
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

func TestReadRegularFileNoFollowReadsLongRegularPath(t *testing.T) {
	dir := t.TempDir()
	for len(filepath.Join(dir, GitIgnorePath)) <= 300 {
		dir = filepath.Join(dir, "long-path-segment-0123456789abcdef")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("cannot create long Windows path: %v", err)
	}
	targetPath := filepath.Join(dir, GitIgnorePath)
	want := []byte("long path bytes\n")
	if err := os.WriteFile(targetPath, want, 0o600); err != nil {
		t.Skipf("cannot write long Windows path: %v", err)
	}

	got, _, err := readRegularFileNoFollow(targetPath)
	if err != nil {
		t.Fatalf("read long regular path: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("long regular path bytes = %q, want %q", got, want)
	}
}
