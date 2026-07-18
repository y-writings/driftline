package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSyncMetadataPaths(t *testing.T) {
	if MetadataDirectoryPath != ".driftline" {
		t.Fatalf("unexpected metadata directory path: %q", MetadataDirectoryPath)
	}
	if ContractPath != ".driftline/contract.toml" {
		t.Fatalf("unexpected Contract path: %q", ContractPath)
	}
	if SyncManifestPath != ".driftline/sync.toml" {
		t.Fatalf("unexpected Sync manifest path: %q", SyncManifestPath)
	}
}

func TestPrepareSyncManifestCreateDefersManifestUntilCommit(t *testing.T) {
	root := t.TempDir()
	want := metadataTestManifest("y-writings/source-repo")

	commit, cleanup, err := PrepareSyncManifestCreate(root, want)
	if err != nil {
		t.Fatalf("prepare Sync manifest create failed: %v", err)
	}

	metadataInfo, err := os.Lstat(filepath.Join(root, MetadataDirectoryPath))
	if err != nil {
		t.Fatalf("lstat metadata directory failed: %v", err)
	}
	if !metadataInfo.IsDir() || metadataInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("metadata path should be a real directory, mode=%v", metadataInfo.Mode())
	}
	if got := metadataInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("metadata directory mode=%#o, want 0755", got)
	}
	metadataTestAssertMissing(t, filepath.Join(root, SyncManifestPath))

	tempPath := metadataTestSingleTempPath(t, root)
	tempInfo, err := os.Lstat(tempPath)
	if err != nil {
		t.Fatalf("lstat Sync manifest temp file failed: %v", err)
	}
	if !tempInfo.Mode().IsRegular() || tempInfo.Mode().Perm() != 0o644 {
		t.Fatalf("unexpected Sync manifest temp mode: %v", tempInfo.Mode())
	}
	tempData, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("read Sync manifest temp file failed: %v", err)
	}
	if got := string(tempData); got != FormatSyncManifest(want) {
		t.Fatalf("unexpected Sync manifest temp content:\n%s", got)
	}

	if err := commit(); err != nil {
		t.Fatalf("commit Sync manifest create failed: %v", err)
	}
	got, err := LoadSyncManifest(root)
	if err != nil {
		t.Fatalf("load committed Sync manifest failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded Sync manifest mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup after commit should ignore missing temp file: %v", err)
	}
	if temps := metadataTestTempPaths(t, root); len(temps) != 0 {
		t.Fatalf("temporary files remain after commit: %v", temps)
	}
}

func TestSyncMetadataCreationValidation(t *testing.T) {
	t.Run("missing metadata directory", func(t *testing.T) {
		if err := ValidateSyncManifestCreation(t.TempDir()); err != nil {
			t.Fatalf("missing metadata directory should be allowed: %v", err)
		}
	})

	t.Run("metadata directory without manifest", func(t *testing.T) {
		root := t.TempDir()
		metadataTestMkdir(t, filepath.Join(root, MetadataDirectoryPath), 0o755)
		if err := ValidateSyncManifestCreation(root); err != nil {
			t.Fatalf("missing Sync manifest should be allowed: %v", err)
		}
	})

	t.Run("existing regular manifest", func(t *testing.T) {
		root := t.TempDir()
		metadataTestWriteManifest(t, root, metadataTestManifest("y-writings/source-repo"), 0o644)
		metadataTestRequireError(t, ValidateSyncManifestCreation(root), "Sync manifest already exists: .driftline/sync.toml")
	})
}

func TestSyncMetadataCreationRejectsUnsafeMetadataDirectory(t *testing.T) {
	for _, state := range []string{"regular file", "live directory symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			metadataTestSetUnsafeMetadata(t, root, state)

			err := ValidateSyncManifestCreation(root)
			metadataTestRequireError(t, err, "driftline metadata path is not a real directory: .driftline")
			_, _, err = PrepareSyncManifestCreate(root, metadataTestManifest("y-writings/source-repo"))
			metadataTestRequireError(t, err, "driftline metadata path is not a real directory: .driftline")
		})
	}
}

func TestSyncMetadataCreationRejectsUnsafeSyncManifest(t *testing.T) {
	for _, state := range []string{"directory", "live symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			metadataTestSetUnsafeSyncManifest(t, root, state)

			wantErr := "Sync manifest path is not a regular file: .driftline/sync.toml"
			metadataTestRequireError(t, ValidateSyncManifestCreation(root), wantErr)
			_, _, err := PrepareSyncManifestCreate(root, metadataTestManifest("y-writings/source-repo"))
			metadataTestRequireError(t, err, wantErr)
		})
	}
}

func TestLoadSyncManifestReportsMissing(t *testing.T) {
	for _, setup := range []struct {
		name string
		run  func(*testing.T, string)
	}{
		{name: "metadata directory", run: func(*testing.T, string) {}},
		{name: "manifest", run: func(t *testing.T, root string) {
			metadataTestMkdir(t, filepath.Join(root, MetadataDirectoryPath), 0o755)
		}},
	} {
		t.Run(setup.name, func(t *testing.T) {
			root := t.TempDir()
			setup.run(t, root)
			_, err := LoadSyncManifest(root)
			metadataTestRequireError(t, err, "Sync manifest not found: .driftline/sync.toml")
		})
	}
}

func TestLoadSyncManifestRejectsUnsafePathsWithoutFollowing(t *testing.T) {
	for _, test := range []struct {
		name    string
		setup   func(*testing.T, string)
		wantErr string
	}{
		{
			name: "metadata regular file",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "regular file")
			},
		},
		{
			name: "metadata live directory symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "live directory symlink")
			},
		},
		{
			name: "metadata broken symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "broken symlink")
			},
		},
		{
			name: "manifest directory",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "directory")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
		{
			name: "manifest live symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "live symlink")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
		{
			name: "manifest broken symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "broken symlink")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.setup(t, root)
			_, err := LoadSyncManifest(root)
			if test.wantErr == "" {
				metadataTestRequireError(t, err, "driftline metadata path is not a real directory: .driftline")
				return
			}
			metadataTestRequireError(t, err, test.wantErr)
		})
	}
}

func TestPrepareSyncManifestRewriteRequiresExistingRegularManifest(t *testing.T) {
	t.Run("missing metadata directory", func(t *testing.T) {
		root := t.TempDir()
		commit, cleanup, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/source-repo"))
		if commit != nil || cleanup != nil {
			t.Fatal("failed preparation should not return closures")
		}
		metadataTestRequireError(t, err, "Sync manifest not found: .driftline/sync.toml")
		metadataTestAssertMissing(t, filepath.Join(root, MetadataDirectoryPath))
	})

	t.Run("missing manifest", func(t *testing.T) {
		root := t.TempDir()
		metadataTestMkdir(t, filepath.Join(root, MetadataDirectoryPath), 0o755)
		commit, cleanup, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/source-repo"))
		if commit != nil || cleanup != nil {
			t.Fatal("failed preparation should not return closures")
		}
		metadataTestRequireError(t, err, "Sync manifest not found: .driftline/sync.toml")
		metadataTestAssertMissing(t, filepath.Join(root, SyncManifestPath))
	})
}

func TestPrepareSyncManifestRewriteRejectsUnsafeMetadataDirectory(t *testing.T) {
	for _, state := range []string{"regular file", "live directory symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			metadataTestSetUnsafeMetadata(t, root, state)
			_, _, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/source-repo"))
			metadataTestRequireError(t, err, "driftline metadata path is not a real directory: .driftline")
		})
	}
}

func TestLoadSyncManifestReadErrorIncludesCanonicalPath(t *testing.T) {
	root := t.TempDir()
	metadataTestWriteManifest(t, root, metadataTestManifest("y-writings/source-repo"), 0o000)
	manifestPath := filepath.Join(root, SyncManifestPath)
	defer os.Chmod(manifestPath, 0o600)
	if _, err := os.ReadFile(manifestPath); err == nil {
		t.Skip("filesystem permits reading a mode 000 file")
	}

	_, err := LoadSyncManifest(root)
	if err == nil || !strings.HasPrefix(err.Error(), "read Sync manifest .driftline/sync.toml: ") {
		t.Fatalf("expected canonical read context, got %v", err)
	}
}

func TestPrepareSyncManifestCreateErrorIncludesCanonicalMetadataPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-parent", "root")
	_, _, err := PrepareSyncManifestCreate(root, metadataTestManifest("y-writings/source-repo"))
	if err == nil || !strings.HasPrefix(err.Error(), "create driftline metadata directory .driftline: ") {
		t.Fatalf("expected canonical metadata creation context, got %v", err)
	}
}

func TestSyncMetadataInspectionErrorIncludesCanonicalMetadataPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root-file")
	metadataTestWriteFile(t, root, []byte("not a root directory\n"), 0o600)

	err := ValidateSyncManifestCreation(root)
	if err == nil || !strings.HasPrefix(err.Error(), "inspect driftline metadata directory .driftline: ") {
		t.Fatalf("expected canonical metadata inspection context, got %v", err)
	}
}

func TestPrepareSyncManifestRewriteRejectsUnsafeSyncManifest(t *testing.T) {
	for _, state := range []string{"directory", "live symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			metadataTestSetUnsafeSyncManifest(t, root, state)
			_, _, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/source-repo"))
			metadataTestRequireError(t, err, "Sync manifest path is not a regular file: .driftline/sync.toml")
		})
	}
}

func TestPrepareSyncManifestRewritePreservesModeAndDefersReplacement(t *testing.T) {
	root := t.TempDir()
	oldManifest := metadataTestManifest("y-writings/old-source")
	newManifest := metadataTestManifest("y-writings/new-source")
	metadataTestWriteManifest(t, root, oldManifest, 0o640)
	original := FormatSyncManifest(oldManifest)

	commit, cleanup, err := PrepareSyncManifestRewrite(root, newManifest)
	if err != nil {
		t.Fatalf("prepare Sync manifest rewrite failed: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	manifestData, err := os.ReadFile(filepath.Join(root, SyncManifestPath))
	if err != nil {
		t.Fatalf("read original Sync manifest failed: %v", err)
	}
	if got := string(manifestData); got != original {
		t.Fatalf("prepare replaced Sync manifest before commit:\n%s", got)
	}
	tempPath := metadataTestSingleTempPath(t, root)
	tempInfo, err := os.Lstat(tempPath)
	if err != nil {
		t.Fatalf("lstat Sync manifest temp failed: %v", err)
	}
	if got := tempInfo.Mode().Perm(); got != 0o640 {
		t.Fatalf("Sync manifest temp mode=%#o, want 0640", got)
	}

	if err := commit(); err != nil {
		t.Fatalf("commit Sync manifest rewrite failed: %v", err)
	}
	got, err := LoadSyncManifest(root)
	if err != nil {
		t.Fatalf("load rewritten Sync manifest failed: %v", err)
	}
	if !reflect.DeepEqual(got, newManifest) {
		t.Fatalf("rewritten Sync manifest mismatch:\n got: %#v\nwant: %#v", got, newManifest)
	}
	manifestInfo, err := os.Lstat(filepath.Join(root, SyncManifestPath))
	if err != nil {
		t.Fatalf("lstat rewritten Sync manifest failed: %v", err)
	}
	if got := manifestInfo.Mode().Perm(); got != 0o640 {
		t.Fatalf("rewritten Sync manifest mode=%#o, want 0640", got)
	}
}

func TestPrepareSyncManifestCleanupRemovesTemp(t *testing.T) {
	root := t.TempDir()
	_, cleanup, err := PrepareSyncManifestCreate(root, metadataTestManifest("y-writings/source-repo"))
	if err != nil {
		t.Fatalf("prepare Sync manifest create failed: %v", err)
	}
	tempPath := metadataTestSingleTempPath(t, root)
	if got := filepath.Dir(tempPath); got != filepath.Join(root, MetadataDirectoryPath) {
		t.Fatalf("temp directory=%q, want %q", got, filepath.Join(root, MetadataDirectoryPath))
	}
	name := filepath.Base(tempPath)
	if !strings.HasPrefix(name, ".sync-") || !strings.HasSuffix(name, ".toml") {
		t.Fatalf("unexpected temp filename: %q", name)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	metadataTestAssertMissing(t, tempPath)
	if err := cleanup(); err != nil {
		t.Fatalf("repeated cleanup should ignore missing temp: %v", err)
	}
	metadataTestAssertMissing(t, filepath.Join(root, SyncManifestPath))
}

func TestPrepareSyncManifestCreateCommitRechecksDestination(t *testing.T) {
	for _, state := range []string{"regular file", "directory"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			commit, cleanup, err := PrepareSyncManifestCreate(root, metadataTestManifest("y-writings/source-repo"))
			if err != nil {
				t.Fatalf("prepare Sync manifest create failed: %v", err)
			}
			defer cleanup()

			manifestPath := filepath.Join(root, SyncManifestPath)
			switch state {
			case "regular file":
				metadataTestWriteFile(t, manifestPath, []byte("intruder\n"), 0o600)
				metadataTestRequireError(t, commit(), "Sync manifest already exists: .driftline/sync.toml")
				data, err := os.ReadFile(manifestPath)
				if err != nil || string(data) != "intruder\n" {
					t.Fatalf("commit replaced changed destination: data=%q err=%v", data, err)
				}
			case "directory":
				metadataTestMkdir(t, manifestPath, 0o755)
				metadataTestRequireError(t, commit(), "Sync manifest path is not a regular file: .driftline/sync.toml")
				info, err := os.Lstat(manifestPath)
				if err != nil {
					t.Fatalf("lstat changed destination directory failed: %v", err)
				}
				if !info.IsDir() {
					t.Fatalf("commit replaced changed destination directory: mode=%v", info.Mode())
				}
			}
		})
	}
}

func TestPrepareSyncManifestRewriteCommitRechecksDestination(t *testing.T) {
	t.Run("removed", func(t *testing.T) {
		root := t.TempDir()
		metadataTestWriteManifest(t, root, metadataTestManifest("y-writings/old-source"), 0o644)
		commit, cleanup, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/new-source"))
		if err != nil {
			t.Fatalf("prepare Sync manifest rewrite failed: %v", err)
		}
		defer cleanup()
		if err := os.Remove(filepath.Join(root, SyncManifestPath)); err != nil {
			t.Fatalf("remove Sync manifest failed: %v", err)
		}

		metadataTestRequireError(t, commit(), "Sync manifest not found: .driftline/sync.toml")
		metadataTestAssertMissing(t, filepath.Join(root, SyncManifestPath))
	})

	t.Run("replaced by symlink", func(t *testing.T) {
		root := t.TempDir()
		metadataTestWriteManifest(t, root, metadataTestManifest("y-writings/old-source"), 0o644)
		commit, cleanup, err := PrepareSyncManifestRewrite(root, metadataTestManifest("y-writings/new-source"))
		if err != nil {
			t.Fatalf("prepare Sync manifest rewrite failed: %v", err)
		}
		defer cleanup()
		manifestPath := filepath.Join(root, SyncManifestPath)
		if err := os.Remove(manifestPath); err != nil {
			t.Fatalf("remove Sync manifest failed: %v", err)
		}
		outside := filepath.Join(root, "outside-sync.toml")
		metadataTestWriteFile(t, outside, []byte("outside\n"), 0o600)
		if err := os.Symlink(outside, manifestPath); err != nil {
			t.Fatalf("create replacement symlink failed: %v", err)
		}

		metadataTestRequireError(t, commit(), "Sync manifest path is not a regular file: .driftline/sync.toml")
		outsideData, err := os.ReadFile(outside)
		if err != nil || string(outsideData) != "outside\n" {
			t.Fatalf("commit followed replacement symlink: data=%q err=%v", outsideData, err)
		}
	})
}

func TestPrepareSyncManifestValidatesModelBeforeFilesystemChanges(t *testing.T) {
	invalid := SyncManifest{}
	for _, prepare := range []struct {
		name string
		run  func(string) (func() error, func() error, error)
	}{
		{name: "create", run: func(root string) (func() error, func() error, error) {
			return PrepareSyncManifestCreate(root, invalid)
		}},
		{name: "rewrite", run: func(root string) (func() error, func() error, error) {
			return PrepareSyncManifestRewrite(root, invalid)
		}},
	} {
		t.Run(prepare.name, func(t *testing.T) {
			root := t.TempDir()
			commit, cleanup, err := prepare.run(root)
			if commit != nil || cleanup != nil {
				t.Fatal("failed validation should not return closures")
			}
			if err == nil || !strings.Contains(err.Error(), "unsupported Sync manifest version 0") {
				t.Fatalf("expected model validation error, got %v", err)
			}
			metadataTestAssertMissing(t, filepath.Join(root, MetadataDirectoryPath))
		})
	}
}

func TestSyncMetadataDefaultsEmptyRootToWorkingDirectory(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory failed: %v", err)
	}
	root := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDirectory); err != nil {
			t.Errorf("restore working directory failed: %v", err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory failed: %v", err)
	}

	if err := ValidateSyncManifestCreation(""); err != nil {
		t.Fatalf("validate empty root failed: %v", err)
	}
	manifest := metadataTestManifest("y-writings/source-repo")
	commit, cleanup, err := PrepareSyncManifestCreate("", manifest)
	if err != nil {
		t.Fatalf("prepare with empty root failed: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit with empty root failed: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup with empty root failed: %v", err)
	}
	if _, err := LoadSyncManifest(""); err != nil {
		t.Fatalf("load with empty root failed: %v", err)
	}

	rewriteCommit, rewriteCleanup, err := PrepareSyncManifestRewrite("", metadataTestManifest("y-writings/new-source"))
	if err != nil {
		t.Fatalf("prepare rewrite with empty root failed: %v", err)
	}
	if err := rewriteCommit(); err != nil {
		t.Fatalf("commit rewrite with empty root failed: %v", err)
	}
	if err := rewriteCleanup(); err != nil {
		t.Fatalf("cleanup rewrite with empty root failed: %v", err)
	}
}

func metadataTestManifest(repository string) SyncManifest {
	return SyncManifest{
		Version: 2,
		Source:  SyncSource{Repository: repository, Ref: "main"},
		Files: map[string]map[string]string{
			"github-workflow": {"ci": ".github/workflows/ci.yaml"},
		},
	}
}

func metadataTestSetUnsafeMetadata(t *testing.T, root string, state string) {
	t.Helper()
	metadataPath := filepath.Join(root, MetadataDirectoryPath)
	switch state {
	case "regular file":
		metadataTestWriteFile(t, metadataPath, []byte("not a directory\n"), 0o644)
	case "live directory symlink":
		outside := filepath.Join(root, "outside-metadata")
		metadataTestMkdir(t, outside, 0o755)
		metadataTestWriteFile(t, filepath.Join(outside, "sync.toml"), []byte(FormatSyncManifest(metadataTestManifest("y-writings/outside"))), 0o644)
		if err := os.Symlink(outside, metadataPath); err != nil {
			t.Fatalf("create live metadata symlink failed: %v", err)
		}
	case "broken symlink":
		if err := os.Symlink(filepath.Join(root, "missing-metadata"), metadataPath); err != nil {
			t.Fatalf("create broken metadata symlink failed: %v", err)
		}
	default:
		t.Fatalf("unknown unsafe metadata state %q", state)
	}
}

func metadataTestSetUnsafeSyncManifest(t *testing.T, root string, state string) {
	t.Helper()
	metadataTestMkdir(t, filepath.Join(root, MetadataDirectoryPath), 0o755)
	manifestPath := filepath.Join(root, SyncManifestPath)
	switch state {
	case "directory":
		metadataTestMkdir(t, manifestPath, 0o755)
	case "live symlink":
		outside := filepath.Join(root, "outside-sync.toml")
		metadataTestWriteFile(t, outside, []byte(FormatSyncManifest(metadataTestManifest("y-writings/outside"))), 0o644)
		if err := os.Symlink(outside, manifestPath); err != nil {
			t.Fatalf("create live Sync manifest symlink failed: %v", err)
		}
	case "broken symlink":
		if err := os.Symlink(filepath.Join(root, "missing-sync.toml"), manifestPath); err != nil {
			t.Fatalf("create broken Sync manifest symlink failed: %v", err)
		}
	default:
		t.Fatalf("unknown unsafe Sync manifest state %q", state)
	}
}

func metadataTestWriteManifest(t *testing.T, root string, manifest SyncManifest, mode os.FileMode) {
	t.Helper()
	metadataTestMkdir(t, filepath.Join(root, MetadataDirectoryPath), 0o755)
	metadataTestWriteFile(t, filepath.Join(root, SyncManifestPath), []byte(FormatSyncManifest(manifest)), mode)
	if err := os.Chmod(filepath.Join(root, SyncManifestPath), mode); err != nil {
		t.Fatalf("chmod Sync manifest failed: %v", err)
	}
}

func metadataTestWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %q failed: %v", path, err)
	}
}

func metadataTestMkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatalf("create directory %q failed: %v", path, err)
	}
}

func metadataTestSingleTempPath(t *testing.T, root string) string {
	t.Helper()
	temps := metadataTestTempPaths(t, root)
	if len(temps) != 1 {
		t.Fatalf("temporary Sync manifests=%v, want exactly one", temps)
	}
	return temps[0]
}

func metadataTestTempPaths(t *testing.T, root string) []string {
	t.Helper()
	directory := filepath.Join(root, MetadataDirectoryPath)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read metadata directory failed: %v", err)
	}
	var paths []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".sync-") && strings.HasSuffix(entry.Name(), ".toml") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	return paths
}

func metadataTestAssertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Lstat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %q to be missing, got %v", path, err)
	}
}

func metadataTestRequireError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("error=%v, want %q", err, want)
	}
}
