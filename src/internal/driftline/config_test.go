package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSourceManifestTOML(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 2 {
		t.Fatalf("unexpected version: %d", manifest.Version)
	}
	ci := manifest.Files["github-workflow"]["ci"]
	if ci.Path != ".github/workflows/ci.yaml" || ci.Mode != ModeManaged {
		t.Fatalf("unexpected ci entry: %#v", ci)
	}
	release := manifest.Files["github-workflow"]["release"]
	if release.Path != ".github/workflows/release.yaml" || release.Mode != ModeTemplate {
		t.Fatalf("unexpected release entry: %#v", release)
	}
	config := manifest.Files["mise"]["config"]
	if config.Path != ".mise/config.toml" || config.Mode != ModeTemplate {
		t.Fatalf("unexpected mise config entry: %#v", config)
	}
}

func TestLoadSourceManifestRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":        "version = 2\nextra = true\n",
		"unknown source file field": "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"managed\", extra = true }\n",
		"invalid mode":              "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"copy\" }\n",
		"missing path":              "version = 2\n[files.github-workflow]\nci = { mode = \"managed\" }\n",
		"invalid group id":          "version = 2\n[files.\"github.workflow\"]\nci = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid file id":           "version = 2\n[files.github-workflow]\n\"bad/id\" = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid source path":       "version = 2\n[files.github-workflow]\nci = { path = \"../ci.yaml\", mode = \"managed\" }\n",
		"duplicate source path":     "version = 2\n[files.first]\nci = { path = \"./same.yaml\", mode = \"managed\" }\n[files.second]\nci = { path = \"same.yaml\", mode = \"template\" }\n",
		"old yaml shape":            "version: 2\nfiles:\n  - id: ci\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSourceManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadTargetConfigTOML(t *testing.T) {
	config, err := LoadTargetConfigBytes([]byte(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
`))
	if err != nil {
		t.Fatalf("load target config failed: %v", err)
	}
	if config.Version != 2 || config.Source.Repository != "y-writings/source-repo" || config.Source.Ref != "main" {
		t.Fatalf("unexpected target config: %#v", config)
	}
	if got := config.Files["github-workflow"]["ci"]; got != ".github/workflows/project-ci.yaml" {
		t.Fatalf("unexpected target path: %q", got)
	}
}

func TestLoadTargetConfigRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":       "version = 2\nextra = true\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n",
		"unknown source field":     "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\nextra = true\n",
		"missing source":           "version = 2\n",
		"bad repository":           "version = 2\n[source]\nrepository = \"https://github.com/y-writings/source-repo\"\nref = \"main\"\n",
		"invalid group id":         "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.\"github.workflow\"]\nci = \"ci.yaml\"\n",
		"invalid file id":          "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\n\"bad/id\" = \"ci.yaml\"\n",
		"invalid target path":      "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\nci = \"../ci.yaml\"\n",
		"reserved target path":     "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.driftline]\ntarget = \".driftline-target.toml\"\n",
		"duplicate target path":    "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.first]\nci = \"./same.yaml\"\n[files.second]\nci = \"same.yaml\"\n",
		"old path_overrides shape": "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[[files]]\nid = \"ci\"\npath_overrides = { ci = \"custom.yaml\" }\n",
		"old yaml shape":           "version: 2\nsource:\n  repository: y-writings/source-repo\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadTargetConfigBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTargetConfigFromSourceManifestIncludesManagedFilesOnly(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}

	config, err := TargetConfigFromSourceManifest("y-writings/source-repo", "main", manifest)
	if err != nil {
		t.Fatalf("create target config failed: %v", err)
	}
	if got := config.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("managed file default target mismatch: %q", got)
	}
	if _, ok := config.Files["github-workflow"]["release"]; ok {
		t.Fatalf("template file must not be recorded in target config: %#v", config.Files)
	}
}

func TestWriteTargetConfigWritesGroupedTOML(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	config := TargetConfig{
		Version: 2,
		Source:  TargetSource{Repository: "y-writings/source-repo", Ref: "main"},
		Files: map[string]map[string]string{
			"github-workflow": {"ci": ".github/workflows/project-ci.yaml"},
		},
	}

	if err := WriteTargetConfig(path, config); err != nil {
		t.Fatalf("write target config failed: %v", err)
	}
	written, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("written target config should parse: %v", err)
	}
	if got := written.Files["github-workflow"]["ci"]; got != ".github/workflows/project-ci.yaml" {
		t.Fatalf("unexpected round-trip target path: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat target config failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("target config should be readable, mode=%#o", got)
	}
}

func TestPrepareTargetConfigWriteDoesNotReplaceConfigBeforeCommit(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	if err := WriteTargetConfig(path, TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/old-source", Ref: "main"}}); err != nil {
		t.Fatalf("write old target config failed: %v", err)
	}
	commit, cleanup, err := PrepareTargetConfigWrite(path, TargetConfig{
		Version: 2,
		Source:  TargetSource{Repository: "y-writings/source-repo", Ref: "main"},
		Files: map[string]map[string]string{
			"github-workflow": {"ci": ".github/workflows/ci.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("prepare target config write failed: %v", err)
	}
	defer cleanup()
	beforeCommit, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("old target config should still parse: %v", err)
	}
	if beforeCommit.Source.Repository != "y-writings/old-source" {
		t.Fatalf("prepare should not replace target config before commit: %#v", beforeCommit)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit target config failed: %v", err)
	}
	afterCommit, err := LoadTargetConfig(path)
	if err != nil {
		t.Fatalf("new target config should parse: %v", err)
	}
	if got := afterCommit.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("unexpected committed config: %#v", afterCommit)
	}
}

func TestPrepareTargetConfigWritePreservesExistingFileMode(t *testing.T) {
	targetDir := t.TempDir()
	path := filepath.Join(targetDir, TargetConfigPath)
	if err := os.WriteFile(path, []byte(targetConfigText("y-writings/old-source")), 0o640); err != nil {
		t.Fatalf("write old target config failed: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod old target config failed: %v", err)
	}
	commit, cleanup, err := PrepareTargetConfigWrite(path, TargetConfig{
		Version: 2,
		Source:  TargetSource{Repository: "y-writings/source-repo", Ref: "main"},
	})
	if err != nil {
		t.Fatalf("prepare target config write failed: %v", err)
	}
	defer cleanup()
	if err := commit(); err != nil {
		t.Fatalf("commit target config failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat target config failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("target config mode should be preserved, mode=%#o", got)
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

func targetConfigText(repository string) string {
	return `version = 2

[source]
repository = "` + repository + `"
ref = "main"
`
}
