package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptInitialTargetRepositoryHappyPath(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-owned\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt":     []byte("missing template\n"),
		"y-writings/source-repo@abc123:.github/workflows/ci.yaml": []byte("managed source\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Contract:     initialAdoptionContract(),
		SyncManifest: initialAdoptionSyncManifest(),
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}

	manifest, err := LoadSyncManifest(root)
	if err != nil {
		t.Fatalf("Sync manifest should parse: %v", err)
	}
	if got := manifest.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("managed Sync manifest entry mismatch: %q", got)
	}
	if _, ok := manifest.Files["templates"]; ok {
		t.Fatalf("template entries must not be recorded in Sync manifest: %#v", manifest.Files)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("missing template content mismatch: %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/existing.txt"); got != "target-owned\n" {
		t.Fatalf("existing template should be skipped, got %q", got)
	}
	if initialAdoptionTestPathExists(t, root, ".github/workflows/ci.yaml") {
		t.Fatal("managed files must not be copied during initial adoption")
	}
	if got, want := source.reads, []string{"y-writings/source-repo@abc123:templates/missing.txt"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected source reads: got %#v want %#v", got, want)
	}
}

func TestAdoptInitialTargetRepositoryPreservesContract(t *testing.T) {
	root := t.TempDir()
	contractBytes := `# source-owned Contract in a dual-role repository
version = 2

[files.github-workflow]
# managed source declaration
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }

[files.templates]
existing = { path = "templates/existing.txt", mode = "template" }
`
	writeInitialAdoptionTestFile(t, root, ContractPath, contractBytes)
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-owned\n")
	source := &fakeInitialAdoptionSource{}
	contract := Contract{Version: 2, Files: map[string]map[string]ContractFile{
		"github-workflow": {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
		"templates":       {"existing": {Path: "templates/existing.txt", Mode: ModeTemplate}},
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Contract:   contract,
		SyncManifest: SyncManifest{
			Version: 2,
			Source:  SyncSource{Repository: "y-writings/source-repo", Ref: "main"},
			Files:   map[string]map[string]string{"github-workflow": {"ci": ".github/workflows/ci.yaml"}},
		},
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}
	if len(source.reads) != 0 {
		t.Fatalf("adoption should not read bytes for existing Templates or Managed files: %#v", source.reads)
	}
	if got := readInitialAdoptionTestFile(t, root, ContractPath); got != contractBytes {
		t.Fatalf("initial adoption changed Contract bytes:\n%s", got)
	}
	if !initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("initial adoption should create Sync manifest beside Contract")
	}
}

func TestAdoptInitialTargetRepositoryDefaultsEmptyRootToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Source:       &fakeInitialAdoptionSource{},
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Contract:     initialAdoptionManagedOnlyContract(),
		SyncManifest: initialAdoptionManagedOnlySyncManifest(),
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}
	if !initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("empty root should write Sync manifest in the working directory")
	}
}

func TestAdoptInitialTargetRepositoryRequiresSource(t *testing.T) {
	err := AdoptInitialTargetRepository(InitialAdoptionOptions{})
	if err == nil || err.Error() != "source client is required" {
		t.Fatalf("expected required source error, got %v", err)
	}
}

func TestAdoptInitialTargetRepositoryRejectsExistingSyncManifestBeforeWrites(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, SyncManifestPath, "existing\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Contract:     initialAdoptionContract(),
		SyncManifest: initialAdoptionSyncManifest(),
	})
	if err == nil || err.Error() != "Sync manifest already exists: .driftline/sync.toml" {
		t.Fatalf("expected existing Sync manifest error, got %v", err)
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template files must not be written after existing Sync manifest error")
	}
	if len(source.reads) != 0 {
		t.Fatalf("source files must not be read after existing Sync manifest error: %#v", source.reads)
	}
}

func TestAdoptInitialTargetRepositoryRejectsUnsafeMetadataBeforeReadsOrWrites(t *testing.T) {
	for _, tt := range []struct {
		name    string
		setup   func(t *testing.T, root string)
		wantErr string
	}{
		{
			name: "metadata regular file",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "regular file")
			},
			wantErr: "driftline metadata path is not a real directory: .driftline",
		},
		{
			name: "metadata live directory symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "live directory symlink")
			},
			wantErr: "driftline metadata path is not a real directory: .driftline",
		},
		{
			name: "metadata broken symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeMetadata(t, root, "broken symlink")
			},
			wantErr: "driftline metadata path is not a real directory: .driftline",
		},
		{
			name: "Sync manifest directory",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "directory")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
		{
			name: "Sync manifest live symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "live symlink")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
		{
			name: "Sync manifest broken symlink",
			setup: func(t *testing.T, root string) {
				metadataTestSetUnsafeSyncManifest(t, root, "broken symlink")
			},
			wantErr: "Sync manifest path is not a regular file: .driftline/sync.toml",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:         root,
				Source:       source,
				Repository:   "y-writings/source-repo",
				Commit:       "abc123",
				Contract:     initialAdoptionTemplateOnlyContract(),
				SyncManifest: initialAdoptionNoFilesSyncManifest(),
			})
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("expected metadata preflight error %q, got %v", tt.wantErr, err)
			}
			if len(source.reads) != 0 {
				t.Fatalf("unsafe metadata must fail before source reads: %#v", source.reads)
			}
			if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
				t.Fatal("unsafe metadata must fail before Template writes")
			}
			temps, globErr := filepath.Glob(filepath.Join(root, MetadataDirectoryPath, ".sync-*.toml"))
			if globErr != nil {
				t.Fatalf("find Sync manifest temp files: %v", globErr)
			}
			if len(temps) != 0 {
				t.Fatalf("unsafe metadata must fail before Sync manifest preparation: %v", temps)
			}
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsExistingManagedTarget(t *testing.T) {
	for _, tt := range []struct {
		name              string
		setup             func(t *testing.T, root string)
		wantError         string
		wantForceGuidance bool
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
			},
			wantError:         "managed target already exists",
			wantForceGuidance: true,
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, ".github/workflows/ci.yaml"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "managed target is not a regular file",
		},
		{
			name: "symlink to regular file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, "real-ci.yaml", "real\n")
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "real-ci.yaml"), path); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "managed target is not a regular file",
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing-target", path); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "managed target is not a regular file",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:         root,
				Source:       source,
				Repository:   "y-writings/source-repo",
				Commit:       "abc123",
				Contract:     initialAdoptionContract(),
				SyncManifest: initialAdoptionSyncManifest(),
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected managed target error containing %q, got %v", tt.wantError, err)
			}
			hasForceGuidance := strings.Contains(err.Error(), "rerun with --force")
			if tt.wantForceGuidance && !hasForceGuidance {
				t.Fatalf("expected managed target guidance error, got %v", err)
			}
			if !tt.wantForceGuidance && hasForceGuidance {
				t.Fatalf("non-regular managed target must not suggest force adoption, got %v", err)
			}
			if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
				t.Fatal("Sync manifest must not be written after managed target conflict")
			}
			if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
				t.Fatal("template files must not be written after managed target conflict")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after managed target conflict: %#v", source.reads)
			}
		})
	}
}

func TestAdoptInitialTargetRepositoryPreflightsAllManagedTargetsBeforeReadingTemplateSources(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
	source := &fakeInitialAdoptionSource{}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Contract: Contract{Version: 2, Files: map[string]map[string]ContractFile{
			"aaa-template": {"missing": {Path: "templates/missing.txt", Mode: ModeTemplate}},
			"zzz-managed":  {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
		}},
		SyncManifest: initialAdoptionSyncManifest(),
	})
	if err == nil || err.Error() != "managed target already exists: .github/workflows/ci.yaml (rerun with --force to adopt existing regular files)" {
		t.Fatalf("expected managed target guidance error, got %v", err)
	}
	if len(source.reads) != 0 {
		t.Fatalf("source files must not be read before all local preflight passes: %#v", source.reads)
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest must not be written after managed target conflict")
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template files must not be written after managed target conflict")
	}
}

func TestAdoptInitialTargetRepositoryForceAdoptsExistingManagedRegularFile(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-template\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:                        root,
		Source:                      source,
		Repository:                  "y-writings/source-repo",
		Commit:                      "abc123",
		Contract:                    initialAdoptionContract(),
		SyncManifest:                initialAdoptionSyncManifest(),
		AdoptExistingManagedTargets: true,
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}
	if got := readInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("force adoption must not overwrite managed target, got %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("missing template content mismatch: %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/existing.txt"); got != "target-template\n" {
		t.Fatalf("existing template should be skipped, got %q", got)
	}
	manifest := readInitialAdoptionTestFile(t, root, SyncManifestPath)
	if !strings.Contains(manifest, `[files.github-workflow]`) || !strings.Contains(manifest, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("Sync manifest should record adopted managed target:\n%s", manifest)
	}
	if got, want := source.reads, []string{"y-writings/source-repo@abc123:templates/missing.txt"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("force adoption must not read managed source bytes: got %#v want %#v", got, want)
	}
}

func TestAdoptInitialTargetRepositoryForceRejectsNonRegularManagedTargets(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, ".github/workflows/ci.yaml"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink to regular file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, "real-ci.yaml", "real\n")
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "real-ci.yaml"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, root string) {
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing-target", path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink ancestor",
			setup: func(t *testing.T, root string) {
				outside := t.TempDir()
				if err := os.MkdirAll(filepath.Join(outside, "workflows"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(outside, "workflows/ci.yaml"), []byte("outside\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, ".github")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "parent path is file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, ".github", "not a directory\n")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:                        root,
				Source:                      source,
				Repository:                  "y-writings/source-repo",
				Commit:                      "abc123",
				Contract:                    initialAdoptionContract(),
				SyncManifest:                initialAdoptionSyncManifest(),
				AdoptExistingManagedTargets: true,
			})
			if err == nil {
				t.Fatal("expected non-regular managed target to fail")
			}
			if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
				t.Fatal("Sync manifest must not be written after non-regular managed target error")
			}
			if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
				t.Fatal("template files must not be written after non-regular managed target error")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after non-regular managed target error: %#v", source.reads)
			}
		})
	}
}

func TestAdoptInitialTargetRepositorySkipsExistingTemplateThroughSymlinkAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "workflows/release.yaml"), []byte("target-owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".github")); err != nil {
		t.Fatal(err)
	}
	source := &fakeInitialAdoptionSource{}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Contract: Contract{Version: 2, Files: map[string]map[string]ContractFile{
			"templates": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
		}},
		SyncManifest: initialAdoptionNoFilesSyncManifest(),
	})
	if err != nil {
		t.Fatalf("existing template through symlink ancestor should be skipped: %v", err)
	}
	if len(source.reads) != 0 {
		t.Fatalf("existing template should not read source bytes: %#v", source.reads)
	}
	if got := readInitialAdoptionTestFile(t, outside, "workflows/release.yaml"); got != "target-owned\n" {
		t.Fatalf("existing template should remain untouched, got %q", got)
	}
	if !initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest should be written after skipping existing template")
	}
}

func TestAdoptInitialTargetRepositoryRejectsMissingTemplateThroughSymlinkAncestorBeforeSourceRead(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".github")); err != nil {
		t.Fatal(err)
	}
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:.github/workflows/release.yaml": []byte("source\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Contract: Contract{Version: 2, Files: map[string]map[string]ContractFile{
			"templates": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
		}},
		SyncManifest: initialAdoptionNoFilesSyncManifest(),
	})
	if err == nil || !strings.Contains(err.Error(), "target path contains symlink") {
		t.Fatalf("expected symlink ancestor error, got %v", err)
	}
	if len(source.reads) != 0 {
		t.Fatalf("missing template should fail before source reads: %#v", source.reads)
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest must not be written after symlink ancestor error")
	}
}

func TestAdoptInitialTargetRepositoryRejectsReservedTargetPath(t *testing.T) {
	for _, tt := range []struct {
		name     string
		contract Contract
	}{
		{
			name: "managed",
			contract: Contract{Version: 2, Files: map[string]map[string]ContractFile{
				"driftline": {"target": {Path: SyncManifestPath, Mode: ModeManaged}},
			}},
		},
		{
			name: "template",
			contract: Contract{Version: 2, Files: map[string]map[string]ContractFile{
				"driftline": {"target": {Path: SyncManifestPath, Mode: ModeTemplate}},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:" + SyncManifestPath: []byte("reserved\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:         root,
				Source:       source,
				Repository:   "y-writings/source-repo",
				Commit:       "abc123",
				Contract:     tt.contract,
				SyncManifest: initialAdoptionNoFilesSyncManifest(),
			})
			if err == nil || !strings.Contains(err.Error(), "reserved driftline metadata path") {
				t.Fatalf("expected reserved path error, got %v", err)
			}
			if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
				t.Fatal("Sync manifest must not be written after reserved path error")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after reserved path error: %#v", source.reads)
			}
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsMissingTemplateSourceBeforeWrites(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Contract:     initialAdoptionTemplateOnlyContract(),
		SyncManifest: initialAdoptionNoFilesSyncManifest(),
	})
	if err == nil || !strings.Contains(err.Error(), "source template not found in source repository") {
		t.Fatalf("expected missing template source error, got %v", err)
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest must not be written after missing source template")
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template file must not be written after missing source template")
	}
}

func TestAdoptInitialTargetRepositoryDoesNotWriteTemplatesWhenSyncManifestPrepareFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Contract:   initialAdoptionTemplateOnlyContract(),
		SyncManifest: SyncManifest{
			Version: 2,
			Source:  SyncSource{Repository: "invalid", Ref: "main"},
		},
	})
	if err == nil {
		t.Fatal("expected Sync manifest prepare failure")
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template file must not be written when Sync manifest prepare fails")
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest must not be committed when prepare fails")
	}
}

func TestInitialAdoptionDoesNotCommitSyncManifestWhenTemplateWriteFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}
	var writes []string

	err := initialAdoption{
		opts: InitialAdoptionOptions{
			Root:         root,
			Source:       source,
			Repository:   "y-writings/source-repo",
			Commit:       "abc123",
			Contract:     initialAdoptionTemplateOnlyContract(),
			SyncManifest: initialAdoptionNoFilesSyncManifest(),
		},
		writeFileBytes: func(target string, data []byte) error {
			writes = append(writes, target)
			return errors.New("write failed")
		},
	}.adopt()
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("expected write failure, got %v", err)
	}
	if len(writes) != 1 || writes[0] != filepath.Join(root, "templates/missing.txt") {
		t.Fatalf("unexpected template writes: %#v", writes)
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest must not be committed after template write failure")
	}
}

func TestInitialAdoptionLeavesTemplatesWhenSyncManifestCommitFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := initialAdoption{
		opts: InitialAdoptionOptions{
			Root:         root,
			Source:       source,
			Repository:   "y-writings/source-repo",
			Commit:       "abc123",
			Contract:     initialAdoptionTemplateOnlyContract(),
			SyncManifest: initialAdoptionNoFilesSyncManifest(),
		},
		prepareSyncManifestCreate: func(root string, manifest SyncManifest) (func() error, func() error, error) {
			metadataDir := filepath.Join(root, MetadataDirectoryPath)
			if err := os.MkdirAll(metadataDir, 0o755); err != nil {
				return nil, nil, err
			}
			temp, err := os.CreateTemp(metadataDir, ".sync-*.toml")
			if err != nil {
				return nil, nil, err
			}
			if _, err := temp.WriteString(FormatSyncManifest(manifest)); err != nil {
				temp.Close()
				os.Remove(temp.Name())
				return nil, nil, err
			}
			if err := temp.Close(); err != nil {
				os.Remove(temp.Name())
				return nil, nil, err
			}
			cleanup := func() error { return os.Remove(temp.Name()) }
			commit := func() error { return errors.New("commit failed") }
			return commit, cleanup, nil
		},
	}.adopt()
	if err == nil || err.Error() != "commit failed" {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("template file should remain after commit failure, got %q", got)
	}
	if initialAdoptionTestPathExists(t, root, SyncManifestPath) {
		t.Fatal("Sync manifest path must be missing after commit failure")
	}
}

type fakeInitialAdoptionSource struct {
	files map[string][]byte
	reads []string
}

func (s *fakeInitialAdoptionSource) ResolveDefaultRef(repository string) (string, string, error) {
	return "", "", errors.New("ResolveDefaultRef is not used by initial adoption")
}

func (s *fakeInitialAdoptionSource) ResolveRef(repository string, ref string) (string, error) {
	return "", errors.New("ResolveRef is not used by initial adoption")
}

func (s *fakeInitialAdoptionSource) ReadFile(repository string, commit string, path string) ([]byte, error) {
	key := fmt.Sprintf("%s@%s:%s", repository, commit, path)
	s.reads = append(s.reads, key)
	data, ok := s.files[key]
	if !ok {
		return nil, fmt.Errorf("missing source file %s", key)
	}
	return append([]byte(nil), data...), nil
}

func initialAdoptionContract() Contract {
	return Contract{Version: 2, Files: map[string]map[string]ContractFile{
		"github-workflow": {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
		"templates": {
			"existing": {Path: "templates/existing.txt", Mode: ModeTemplate},
			"missing":  {Path: "templates/missing.txt", Mode: ModeTemplate},
		},
	}}
}

func initialAdoptionManagedOnlyContract() Contract {
	return Contract{Version: 2, Files: map[string]map[string]ContractFile{
		"github-workflow": {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
	}}
}

func initialAdoptionTemplateOnlyContract() Contract {
	return Contract{Version: 2, Files: map[string]map[string]ContractFile{
		"templates": {"missing": {Path: "templates/missing.txt", Mode: ModeTemplate}},
	}}
}

func initialAdoptionSyncManifest() SyncManifest {
	return SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{
		"github-workflow": {"ci": ".github/workflows/ci.yaml"},
	}}
}

func initialAdoptionManagedOnlySyncManifest() SyncManifest {
	return initialAdoptionSyncManifest()
}

func initialAdoptionNoFilesSyncManifest() SyncManifest {
	return SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}}
}

func writeInitialAdoptionTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readInitialAdoptionTestFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func initialAdoptionTestPathExists(t *testing.T, root, path string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, path))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatal(err)
	return false
}
