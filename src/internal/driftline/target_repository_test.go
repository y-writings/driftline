package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetRepositoryApplyRejectsConflictPlanBeforeWriting(t *testing.T) {
	targetDir := t.TempDir()
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, syncManifestTOMLForApplyTest(""))
	writeTargetRepositoryTestFile(t, targetDir, "existing.txt", "target-owned\n")

	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "existing.txt", Status: StatusConflict, Reason: "target already exists", ForceAllowed: true},
			{ID: "tool.config", Target: "existing.txt", TargetPath: filepath.Join(targetDir, "existing.txt"), SourceBytes: []byte("source\n"), Status: StatusUpdate, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "existing.txt"}}},
	}

	err := TargetRepository{Root: targetDir}.Apply(plan)
	if err == nil {
		t.Fatal("expected conflict plan to be rejected")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, "existing.txt"); got != "target-owned\n" {
		t.Fatalf("conflict plan must not write target file, got %q", got)
	}
	manifest := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath)
	if strings.Contains(manifest, "tool") {
		t.Fatalf("conflict plan must not commit Sync manifest:\n%s", manifest)
	}
}

func TestTargetRepositoryApplyDeletesBeforeWritingChildPath(t *testing.T) {
	targetDir := t.TempDir()
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, syncManifestTOMLForApplyTest(`[files.old]
config = "dir"
`))
	writeTargetRepositoryTestFile(t, targetDir, "dir", "old\n")

	plan := Plan{
		Changes: []Change{
			{ID: "old.config", Target: "dir", TargetPath: filepath.Join(targetDir, "dir"), Status: StatusRemove, DeletesTarget: true},
			{ID: "old.config", Target: "dir", Status: StatusSyncManifestRemove},
			{ID: "new.config", Target: "dir/file", Status: StatusSyncManifestAdd},
			{ID: "new.config", Target: "dir/file", TargetPath: filepath.Join(targetDir, "dir/file"), SourceBytes: []byte("new\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"new": {"config": "dir/file"}}},
	}

	if err := (TargetRepository{Root: targetDir}).Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, "dir/file"); got != "new\n" {
		t.Fatalf("unexpected child file content: %q", got)
	}
	manifest := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath)
	if strings.Contains(manifest, "old") || !strings.Contains(manifest, `[files.new]`) || !strings.Contains(manifest, `config = "dir/file"`) {
		t.Fatalf("Sync manifest should move to new child entry:\n%s", manifest)
	}
}

func TestTargetRepositoryApplyDoesNotCommitSyncManifestWhenWriteFails(t *testing.T) {
	targetDir := t.TempDir()
	originalSyncManifest := syncManifestTOMLForApplyTest("")
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, originalSyncManifest)
	writeTargetRepositoryTestFile(t, targetDir, "blocked", "target-owned\n")

	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "blocked/file.txt", Status: StatusSyncManifestAdd},
			{ID: "tool.config", Target: "blocked/file.txt", TargetPath: filepath.Join(targetDir, "blocked/file.txt"), SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "blocked/file.txt"}}},
	}

	if err := (TargetRepository{Root: targetDir}).Apply(plan); err == nil {
		t.Fatal("expected write failure")
	}
	if got := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath); got != originalSyncManifest {
		t.Fatalf("Sync manifest must not be committed after write failure:\n%s", got)
	}
}

func TestTargetRepositoryApplyDoesNotRewriteSyncManifestForFileOnlyUpdate(t *testing.T) {
	targetDir := t.TempDir()
	originalSyncManifest := `version = 2

# keep target-side comments and order
[source]
ref = "main"
repository = "y-writings/source-repo"

[files.github-workflow]
# local placement rationale
ci = ".github/workflows/ci.yaml"
`
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, originalSyncManifest)
	writeTargetRepositoryTestFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")

	plan := Plan{
		Changes: []Change{
			{ID: "github-workflow.ci", Target: ".github/workflows/ci.yaml", TargetPath: filepath.Join(targetDir, ".github/workflows/ci.yaml"), SourceBytes: []byte("new\n"), Status: StatusUpdate, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"github-workflow": {"ci": ".github/workflows/ci.yaml"}}},
	}

	if err := (TargetRepository{Root: targetDir}).Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, ".github/workflows/ci.yaml"); got != "new\n" {
		t.Fatalf("managed file should be updated, got %q", got)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath); got != originalSyncManifest {
		t.Fatalf("Sync manifest should not be rewritten for file-only update:\n%s", got)
	}
}

func TestTargetRepositoryApplyDoesNotCreateMissingSyncManifest(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "managed.txt")
	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "managed.txt", Status: StatusSyncManifestAdd},
			{ID: "tool.config", Target: "managed.txt", TargetPath: targetPath, SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "managed.txt"}}},
	}

	err := (TargetRepository{Root: targetDir}).Apply(plan)
	if err == nil || !strings.Contains(err.Error(), "Sync manifest not found: .driftline/sync.toml") {
		t.Fatalf("expected missing canonical Sync manifest error, got %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("managed target must not be written when Sync manifest rewrite cannot prepare, stat err=%v", err)
	}
}

func TestTargetRepositoryApplyRejectsStaleSyncBeforeManagedWrite(t *testing.T) {
	for _, state := range []string{"removed", "live symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			targetDir := t.TempDir()
			managedPath := filepath.Join(targetDir, "managed.txt")
			writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, syncManifestTOMLForApplyTest(`[files.old]
config = "old.txt"
`))
			writeTargetRepositoryTestFile(t, targetDir, "managed.txt", "target-owned\n")
			plan := Plan{
				Changes: []Change{
					{ID: "old.config", Target: "old.txt", Status: StatusSyncManifestRemove},
					{ID: "tool.config", Target: "managed.txt", Status: StatusSyncManifestAdd},
					{ID: "tool.config", Target: "managed.txt", TargetPath: managedPath, SourceBytes: []byte("source\n"), Status: StatusUpdate, WritesTarget: true},
				},
				NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "managed.txt"}}},
			}

			manifestPath := filepath.Join(targetDir, SyncManifestPath)
			if err := os.Remove(manifestPath); err != nil {
				t.Fatal(err)
			}
			outsideTarget := ""
			switch state {
			case "live symlink":
				outsideTarget = filepath.Join(t.TempDir(), "outside-sync.toml")
				if err := os.WriteFile(outsideTarget, []byte("outside\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outsideTarget, manifestPath); err != nil {
					t.Fatal(err)
				}
			case "broken symlink":
				outsideTarget = filepath.Join(t.TempDir(), "missing-sync.toml")
				if err := os.Symlink(outsideTarget, manifestPath); err != nil {
					t.Fatal(err)
				}
			}

			err := (TargetRepository{Root: targetDir}).Apply(plan)
			wantErr := "Sync manifest path is not a regular file: .driftline/sync.toml"
			if state == "removed" {
				wantErr = "Sync manifest not found: .driftline/sync.toml"
			}
			if err == nil || err.Error() != wantErr {
				t.Fatalf("expected stale Sync error %q, got %v", wantErr, err)
			}
			if got := readTargetRepositoryTestFile(t, targetDir, "managed.txt"); got != "target-owned\n" {
				t.Fatalf("Managed file changed before Sync preflight completed: %q", got)
			}
			info, statErr := os.Lstat(manifestPath)
			if state == "removed" {
				if !os.IsNotExist(statErr) {
					t.Fatalf("Apply recreated removed Sync manifest: %v", statErr)
				}
			} else if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("Apply replaced stale Sync symlink: info=%v err=%v", info, statErr)
			}
			if state == "live symlink" {
				data, readErr := os.ReadFile(outsideTarget)
				if readErr != nil || string(data) != "outside\n" {
					t.Fatalf("Apply wrote through live Sync symlink: data=%q err=%v", data, readErr)
				}
			}
			if state == "broken symlink" {
				if _, statErr := os.Lstat(outsideTarget); !os.IsNotExist(statErr) {
					t.Fatalf("Apply wrote through broken Sync symlink: %v", statErr)
				}
			}
		})
	}
}

func TestTargetRepositoryApplySyncRewritePreservesContract(t *testing.T) {
	targetDir := t.TempDir()
	contractBytes := `# source-owned Contract must remain byte-for-byte stable
version = 2

[files.tool]
# local source-side rationale
config = { path = "managed.txt", mode = "managed" }
`
	writeTargetRepositoryTestFile(t, targetDir, ContractPath, contractBytes)
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, syncManifestTOMLForApplyTest(""))
	managedPath := filepath.Join(targetDir, "managed.txt")
	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "managed.txt", Status: StatusSyncManifestAdd},
			{ID: "tool.config", Target: "managed.txt", TargetPath: managedPath, SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "managed.txt"}}},
	}

	if err := (TargetRepository{Root: targetDir}).Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, ContractPath); got != contractBytes {
		t.Fatalf("Sync rewrite changed adjacent Contract bytes:\n%s", got)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, "managed.txt"); got != "source\n" {
		t.Fatalf("Managed file content mismatch: %q", got)
	}
	manifest := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath)
	if !strings.Contains(manifest, `[files.tool]`) || !strings.Contains(manifest, `config = "managed.txt"`) {
		t.Fatalf("Sync manifest was not rewritten:\n%s", manifest)
	}
}

func syncManifestTOMLForApplyTest(files string) string {
	return `version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"
` + files
}

func writeTargetRepositoryTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTargetRepositoryTestFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
