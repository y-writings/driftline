package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadContractTOML(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if contract.Version != 2 {
		t.Fatalf("unexpected version: %d", contract.Version)
	}
	ci := contract.Files["github-workflow"]["ci"]
	if ci.Path != ".github/workflows/ci.yaml" || ci.Mode != ModeManaged {
		t.Fatalf("unexpected ci entry: %#v", ci)
	}
	release := contract.Files["github-workflow"]["release"]
	if release.Path != ".github/workflows/release.yaml" || release.Mode != ModeTemplate {
		t.Fatalf("unexpected release entry: %#v", release)
	}
	config := contract.Files["mise"]["config"]
	if config.Path != ".mise/config.toml" || config.Mode != ModeTemplate {
		t.Fatalf("unexpected mise config entry: %#v", config)
	}
}

func TestLoadContractAcceptsTOML11MultilineInlineTables(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = {
  path = ".github/workflows/ci.yaml",
  mode = "managed",
}
release = {
  path = ".github/workflows/release.yaml",
  mode = "template",
}
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if got := contract.Files["github-workflow"]["ci"].Mode; got != ModeManaged {
		t.Fatalf("unexpected ci mode: %q", got)
	}
	if got := contract.Files["github-workflow"]["release"].Mode; got != ModeTemplate {
		t.Fatalf("unexpected release mode: %q", got)
	}
}

func TestLoadContractRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":          "version = 2\nextra = true\n",
		"unknown Contract file field": "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"managed\", extra = true }\n",
		"invalid mode":                "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"copy\" }\n",
		"missing path":                "version = 2\n[files.github-workflow]\nci = { mode = \"managed\" }\n",
		"invalid group id":            "version = 2\n[files.\"github.workflow\"]\nci = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid file id":             "version = 2\n[files.github-workflow]\n\"bad/id\" = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid source path":         "version = 2\n[files.github-workflow]\nci = { path = \"../ci.yaml\", mode = \"managed\" }\n",
		"duplicate source path":       "version = 2\n[files.first]\nci = { path = \"./same.yaml\", mode = \"managed\" }\n[files.second]\nci = { path = \"same.yaml\", mode = \"template\" }\n",
		"old yaml shape":              "version: 2\nfiles:\n  - id: ci\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadContractBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadSyncManifestTOML(t *testing.T) {
	manifest, err := LoadSyncManifestBytes([]byte(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
`))
	if err != nil {
		t.Fatalf("load Sync manifest failed: %v", err)
	}
	if manifest.Version != 2 || manifest.Source.Repository != "y-writings/source-repo" || manifest.Source.Ref != "main" {
		t.Fatalf("unexpected Sync manifest: %#v", manifest)
	}
	if got := manifest.Files["github-workflow"]["ci"]; got != ".github/workflows/project-ci.yaml" {
		t.Fatalf("unexpected target path: %q", got)
	}
}

func TestLoadSyncManifestRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":       "version = 2\nextra = true\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n",
		"unknown source field":     "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\nextra = true\n",
		"missing source":           "version = 2\n",
		"bad repository":           "version = 2\n[source]\nrepository = \"https://github.com/y-writings/source-repo\"\nref = \"main\"\n",
		"invalid group id":         "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.\"github.workflow\"]\nci = \"ci.yaml\"\n",
		"invalid file id":          "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\n\"bad/id\" = \"ci.yaml\"\n",
		"invalid target path":      "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\nci = \"../ci.yaml\"\n",
		"reserved target path":     "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.driftline]\ntarget = \".driftline-target.toml\"\n",
		"reserved old lock path":   "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.driftline]\nlock = \"driftline-lock.yaml\"\n",
		"duplicate target path":    "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.first]\nci = \"./same.yaml\"\n[files.second]\nci = \"same.yaml\"\n",
		"old path_overrides shape": "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[[files]]\nid = \"ci\"\npath_overrides = { ci = \"custom.yaml\" }\n",
		"old yaml shape":           "version: 2\nsource:\n  repository: y-writings/source-repo\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSyncManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMetadataErrorsUseContractAndSyncManifestLabels(t *testing.T) {
	tests := []struct {
		name    string
		load    func([]byte) error
		input   string
		wantErr string
	}{
		{
			name: "Contract parse error",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version =",
			wantErr: "parse Contract",
		},
		{
			name: "Contract unknown key",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version = 2\nextra = true\n",
			wantErr: `Contract contains unknown key "extra"`,
		},
		{
			name: "Contract version",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version = 1\n",
			wantErr: "unsupported Contract version 1",
		},
		{
			name: "Sync manifest parse error",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version =",
			wantErr: "parse Sync manifest",
		},
		{
			name: "Sync manifest unknown key",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version = 2\nextra = true\n",
			wantErr: `Sync manifest contains unknown key "extra"`,
		},
		{
			name: "Sync manifest version",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version = 1\n",
			wantErr: "unsupported Sync manifest version 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.load([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSyncManifestFromContractIncludesManagedFilesOnly(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}

	manifest, err := SyncManifestFromContract("y-writings/source-repo", "main", contract)
	if err != nil {
		t.Fatalf("create Sync manifest failed: %v", err)
	}
	if got := manifest.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("managed file default target mismatch: %q", got)
	}
	if _, ok := manifest.Files["github-workflow"]["release"]; ok {
		t.Fatalf("template file must not be recorded in Sync manifest: %#v", manifest.Files)
	}
}

func TestWriteSyncManifestWritesGroupedTOML(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	manifest := SyncManifest{
		Version: 2,
		Source:  SyncSource{Repository: "y-writings/source-repo", Ref: "main"},
		Files: map[string]map[string]string{
			"github-workflow": {"ci": ".github/workflows/project-ci.yaml"},
		},
	}

	if err := WriteTargetConfig(path, manifest); err != nil {
		t.Fatalf("write Sync manifest failed: %v", err)
	}
	written, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("written Sync manifest should parse: %v", err)
	}
	if got := written.Files["github-workflow"]["ci"]; got != ".github/workflows/project-ci.yaml" {
		t.Fatalf("unexpected round-trip target path: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat Sync manifest failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("Sync manifest should be readable, mode=%#o", got)
	}
}

func TestPrepareSyncManifestWriteDoesNotReplaceManifestBeforeCommit(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	if err := WriteTargetConfig(path, SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/old-source", Ref: "main"}}); err != nil {
		t.Fatalf("write old Sync manifest failed: %v", err)
	}
	commit, cleanup, err := PrepareTargetConfigWrite(path, SyncManifest{
		Version: 2,
		Source:  SyncSource{Repository: "y-writings/source-repo", Ref: "main"},
		Files: map[string]map[string]string{
			"github-workflow": {"ci": ".github/workflows/ci.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("prepare Sync manifest write failed: %v", err)
	}
	defer cleanup()
	beforeCommit, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("old Sync manifest should still parse: %v", err)
	}
	if beforeCommit.Source.Repository != "y-writings/old-source" {
		t.Fatalf("prepare should not replace Sync manifest before commit: %#v", beforeCommit)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit Sync manifest failed: %v", err)
	}
	afterCommit, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("new Sync manifest should parse: %v", err)
	}
	if got := afterCommit.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("unexpected committed Sync manifest: %#v", afterCommit)
	}
}

func TestPrepareSyncManifestWritePreservesExistingFileMode(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	if err := os.WriteFile(path, []byte(syncManifestText("y-writings/old-source")), 0o640); err != nil {
		t.Fatalf("write old Sync manifest failed: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod old Sync manifest failed: %v", err)
	}
	commit, cleanup, err := PrepareTargetConfigWrite(path, SyncManifest{
		Version: 2,
		Source:  SyncSource{Repository: "y-writings/source-repo", Ref: "main"},
	})
	if err != nil {
		t.Fatalf("prepare Sync manifest write failed: %v", err)
	}
	defer cleanup()
	if err := commit(); err != nil {
		t.Fatalf("commit Sync manifest failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat Sync manifest failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("Sync manifest mode should be preserved, mode=%#o", got)
	}
}

func TestValidateConfigPath(t *testing.T) {
	valid := []string{".github/workflows/ci.yml", "templates/my file.txt", "config/.env.example"}
	for _, path := range valid {
		if err := ValidateConfigPath(path, "test"); err != nil {
			t.Fatalf("expected %q to be valid: %v", path, err)
		}
	}

	invalid := []string{"", " ", "/abs", ".", "..", "../x", "a/../b", "a\\b", "templates/", " leading.txt", "trailing.txt "}
	for _, path := range invalid {
		if err := ValidateConfigPath(path, "test"); err == nil || !strings.Contains(err.Error(), "test") {
			t.Fatalf("expected labelled validation error for %q, got %v", path, err)
		}
	}
}

func syncManifestText(repository string) string {
	return `version = 2

[source]
repository = "` + repository + `"
ref = "main"
`
}
