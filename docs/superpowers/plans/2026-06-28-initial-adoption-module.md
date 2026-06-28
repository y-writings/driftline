# Initial Adoption Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Target Repository initial adoption sequencing out of `runInit` into a Deep Module with commit-last Target manifest safety.

**Architecture:** Add an Initial adoption Module in the `driftline` package. `runInit` continues to own CLI validation, Source Repository ref resolution, Source Config loading, Target manifest derivation, and stdout. The new Module owns Target Repository preflight, missing Template file placement, Target manifest temp preparation, and final Target manifest commit.

**Tech Stack:** Go, standard library filesystem APIs, existing TOML config writer, existing `SourceClient`, existing `go test ./...` suite.

---

## File Structure

- Create `src/internal/driftline/initial_adoption.go`: `InitialAdoptionOptions`, `AdoptInitialTargetRepository`, and unexported implementation helpers for Target Repository initial adoption.
- Create `src/internal/driftline/initial_adoption_test.go`: focused tests for preflight, Template placement, commit-last behavior, and no rollback after commit failure.
- Modify `src/internal/driftline/commands/init.go`: replace Target Repository write/preflight/template logic with `driftline.AdoptInitialTargetRepository(...)` while preserving current CLI output.
- Modify `src/internal/driftline/commands/commands_test.go`: keep existing integration coverage; only adjust if command-private helper removal requires it.
- Keep `src/internal/driftline/target_repository.go` unchanged in this work.

Do not commit during execution unless the user explicitly requests it.

## Task 1: Add Initial Adoption Module

**Files:**
- Create: `src/internal/driftline/initial_adoption.go`
- Test: `src/internal/driftline/initial_adoption_test.go`

- [ ] **Step 1: Write focused failing tests for Initial adoption**

Create `src/internal/driftline/initial_adoption_test.go` with:

```go
package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const initialAdoptionTestRepository = "y-writings/source-repo"
const initialAdoptionTestCommit = "0123456789abcdef0123456789abcdef01234567"

type initialAdoptionFakeSource struct {
	files map[string][]byte
}

func (f initialAdoptionFakeSource) ResolveDefaultRef(repository string) (string, string, error) {
	return "", "", errors.New("not used")
}

func (f initialAdoptionFakeSource) ResolveRef(repository string, ref string) (string, error) {
	return "", errors.New("not used")
}

func (f initialAdoptionFakeSource) ReadFile(repository string, commit string, path string) ([]byte, error) {
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestAdoptInitialTargetRepositoryWritesConfigAndPlacesTemplates(t *testing.T) {
	targetDir := t.TempDir()
	writeInitialAdoptionTestFile(t, targetDir, ".mise/config.toml", "target-owned\n")
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {
			"ci":      {Path: ".github/workflows/ci.yaml", Mode: ModeManaged},
			"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate},
		},
		"mise": {
			"config": {Path: ".mise/config.toml", Mode: ModeTemplate},
		},
	}}
	config := initialAdoptionTargetConfig(t, manifest)
	source := newInitialAdoptionFakeSource(map[string]string{
		".github/workflows/release.yaml": "release\n",
		".mise/config.toml":              "source-template\n",
	})

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         targetDir,
		Source:       source,
		Repository:   initialAdoptionTestRepository,
		Commit:       initialAdoptionTestCommit,
		Manifest:     manifest,
		TargetConfig: config,
	})
	if err != nil {
		t.Fatalf("adoption failed: %v", err)
	}

	targetConfig := readInitialAdoptionTestFile(t, targetDir, TargetConfigPath)
	for _, want := range []string{"version = 2", `[source]`, `repository = "y-writings/source-repo"`, `[files.github-workflow]`, `ci = ".github/workflows/ci.yaml"`} {
		if !strings.Contains(targetConfig, want) {
			t.Fatalf("target config missing %q:\n%s", want, targetConfig)
		}
	}
	for _, removed := range []string{"release", "mise", "template", "path_overrides", "if_not_exists"} {
		if strings.Contains(targetConfig, removed) {
			t.Fatalf("target config contains non-managed or old field %q:\n%s", removed, targetConfig)
		}
	}
	if got := readInitialAdoptionTestFile(t, targetDir, ".github/workflows/release.yaml"); got != "release\n" {
		t.Fatalf("expected missing template to be placed, got %q", got)
	}
	if got := readInitialAdoptionTestFile(t, targetDir, ".mise/config.toml"); got != "target-owned\n" {
		t.Fatalf("existing template target must be skipped, got %q", got)
	}
	assertInitialAdoptionFileMissing(t, targetDir, ".github/workflows/ci.yaml")
}

func TestAdoptInitialTargetRepositoryRejectsExistingTargetConfigBeforeWrites(t *testing.T) {
	targetDir := t.TempDir()
	writeInitialAdoptionTestFile(t, targetDir, TargetConfigPath, "existing\n")
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
	}}
	config := initialAdoptionTargetConfig(t, manifest)
	source := newInitialAdoptionFakeSource(map[string]string{".github/workflows/release.yaml": "release\n"})

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config})
	if err == nil || !strings.Contains(err.Error(), "target config already exists") {
		t.Fatalf("expected target config exists error, got %v", err)
	}
	assertInitialAdoptionFileMissing(t, targetDir, ".github/workflows/release.yaml")
}

func TestAdoptInitialTargetRepositoryRejectsExistingManagedTargetBeforeWrites(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, string){
		"regular file": func(t *testing.T, targetDir string) {
			writeInitialAdoptionTestFile(t, targetDir, ".github/workflows/ci.yaml", "existing\n")
		},
		"broken symlink": func(t *testing.T, targetDir string) {
			linkPath := filepath.Join(targetDir, ".github/workflows/ci.yaml")
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), linkPath); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			setup(t, targetDir)
			manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
				"github-workflow": {
					"ci":      {Path: ".github/workflows/ci.yaml", Mode: ModeManaged},
					"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate},
				},
			}}
			config := initialAdoptionTargetConfig(t, manifest)
			source := newInitialAdoptionFakeSource(map[string]string{".github/workflows/release.yaml": "release\n"})

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config})
			if err == nil || !strings.Contains(err.Error(), "managed target already exists") {
				t.Fatalf("expected managed target exists error, got %v", err)
			}
			assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
			assertInitialAdoptionFileMissing(t, targetDir, ".github/workflows/release.yaml")
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsReservedTargetsBeforeWrites(t *testing.T) {
	for name, mode := range map[string]FileMode{
		"managed":  ModeManaged,
		"template": ModeTemplate,
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
				"driftline": {"target": {Path: TargetConfigPath, Mode: mode}},
			}}
			config := TargetConfig{Version: 2, Source: TargetSource{Repository: initialAdoptionTestRepository, Ref: "main"}, Files: map[string]map[string]string{}}
			source := newInitialAdoptionFakeSource(map[string]string{TargetConfigPath: "template bytes\n"})

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config})
			if err == nil || !strings.Contains(err.Error(), "reserved target path") {
				t.Fatalf("expected reserved target path error, got %v", err)
			}
			assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsMissingTemplateSourceBeforeWrites(t *testing.T) {
	targetDir := t.TempDir()
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
	}}
	config := initialAdoptionTargetConfig(t, manifest)
	source := newInitialAdoptionFakeSource(nil)

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config})
	if err == nil || !strings.Contains(err.Error(), "source template not found") {
		t.Fatalf("expected missing source template error, got %v", err)
	}
	assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
	assertInitialAdoptionFileMissing(t, targetDir, ".github/workflows/release.yaml")
}

func TestAdoptInitialTargetRepositoryDoesNotWriteTemplatesWhenConfigPrepareFails(t *testing.T) {
	targetDir := t.TempDir()
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
	}}
	invalidConfig := TargetConfig{Version: 2, Source: TargetSource{Repository: "invalid", Ref: "main"}, Files: map[string]map[string]string{}}
	source := newInitialAdoptionFakeSource(map[string]string{".github/workflows/release.yaml": "release\n"})

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: invalidConfig})
	if err == nil || !strings.Contains(err.Error(), "repository must be owner/repo") {
		t.Fatalf("expected target config validation error, got %v", err)
	}
	assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
	assertInitialAdoptionFileMissing(t, targetDir, ".github/workflows/release.yaml")
}

func TestInitialAdoptionDoesNotCommitTargetConfigWhenTemplateWriteFails(t *testing.T) {
	targetDir := t.TempDir()
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
	}}
	config := initialAdoptionTargetConfig(t, manifest)
	source := newInitialAdoptionFakeSource(map[string]string{".github/workflows/release.yaml": "release\n"})
	writeAttempted := false
	adoption := initialAdoption{
		opts: InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config},
		writeFileBytes: func(target string, data []byte) error {
			writeAttempted = true
			return errors.New("write failed")
		},
	}

	err := adoption.adopt()
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected template write failure, got %v", err)
	}
	if !writeAttempted {
		t.Fatal("expected template write to be attempted")
	}
	assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
}

func TestInitialAdoptionDoesNotRollbackTemplateWhenConfigCommitFails(t *testing.T) {
	targetDir := t.TempDir()
	manifest := SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"release": {Path: ".github/workflows/release.yaml", Mode: ModeTemplate}},
	}}
	config := initialAdoptionTargetConfig(t, manifest)
	source := newInitialAdoptionFakeSource(map[string]string{".github/workflows/release.yaml": "release\n"})
	adoption := initialAdoption{
		opts: InitialAdoptionOptions{Root: targetDir, Source: source, Repository: initialAdoptionTestRepository, Commit: initialAdoptionTestCommit, Manifest: manifest, TargetConfig: config},
		prepareTargetConfigWrite: func(path string, config TargetConfig) (func() error, func() error, error) {
			_, cleanup, err := PrepareTargetConfigWrite(path, config)
			if err != nil {
				return nil, nil, err
			}
			return func() error { return errors.New("commit failed") }, cleanup, nil
		},
	}

	err := adoption.adopt()
	if err == nil || !strings.Contains(err.Error(), "commit failed") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if got := readInitialAdoptionTestFile(t, targetDir, ".github/workflows/release.yaml"); got != "release\n" {
		t.Fatalf("template should not be rolled back after commit failure, got %q", got)
	}
	assertInitialAdoptionFileMissing(t, targetDir, TargetConfigPath)
}

func newInitialAdoptionFakeSource(files map[string]string) initialAdoptionFakeSource {
	source := initialAdoptionFakeSource{files: map[string][]byte{}}
	for path, content := range files {
		source.files[initialAdoptionTestRepository+"@"+initialAdoptionTestCommit+":"+path] = []byte(content)
	}
	return source
}

func initialAdoptionTargetConfig(t *testing.T, manifest SourceManifest) TargetConfig {
	t.Helper()
	config, err := TargetConfigFromSourceManifest(initialAdoptionTestRepository, "main", manifest)
	if err != nil {
		t.Fatal(err)
	}
	return config
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

func assertInitialAdoptionFileMissing(t *testing.T, root, path string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be missing, stat err=%v", path, err)
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./src/internal/driftline -run 'TestAdoptInitialTargetRepository|TestInitialAdoption' -count=1`

Expected: FAIL because `AdoptInitialTargetRepository`, `InitialAdoptionOptions`, and `initialAdoption` are undefined.

- [ ] **Step 3: Implement the Initial adoption Module**

Create `src/internal/driftline/initial_adoption.go` with:

```go
package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type InitialAdoptionOptions struct {
	Root         string
	Source       SourceClient
	Repository   string
	Commit       string
	Manifest     SourceManifest
	TargetConfig TargetConfig
}

func AdoptInitialTargetRepository(opts InitialAdoptionOptions) error {
	return initialAdoption{opts: opts}.adopt()
}

type initialAdoption struct {
	opts InitialAdoptionOptions

	prepareTargetConfigWrite func(path string, config TargetConfig) (func() error, func() error, error)
	writeFileBytes           func(target string, data []byte) error
}

type initialTemplatePlacement struct {
	targetPath  string
	sourceBytes []byte
}

func (a initialAdoption) adopt() error {
	if a.opts.Source == nil {
		return fmt.Errorf("source client is required")
	}
	root := a.opts.Root
	if root == "" {
		root = "."
	}
	configPath := filepath.Join(root, TargetConfigPath)
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("target config already exists: %s", TargetConfigPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	templates, err := a.collectInitialTemplates(root)
	if err != nil {
		return err
	}

	prepareTargetConfigWrite := a.prepareTargetConfigWrite
	if prepareTargetConfigWrite == nil {
		prepareTargetConfigWrite = PrepareTargetConfigWrite
	}
	commitConfig, cleanupConfig, err := prepareTargetConfigWrite(configPath, a.opts.TargetConfig)
	if err != nil {
		return err
	}
	if cleanupConfig != nil {
		defer cleanupConfig()
	}

	writeFileBytes := a.writeFileBytes
	if writeFileBytes == nil {
		writeFileBytes = WriteFileBytes
	}
	for _, template := range templates {
		if err := writeFileBytes(template.targetPath, template.sourceBytes); err != nil {
			return err
		}
	}
	return commitConfig()
}

func (a initialAdoption) collectInitialTemplates(root string) ([]initialTemplatePlacement, error) {
	templates := []initialTemplatePlacement{}
	for _, entry := range SourceEntries(a.opts.Manifest) {
		if IsReservedTargetPath(entry.Path) {
			return nil, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := PathWithin(root, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return nil, err
		}
		exists, err := targetPathExists(targetPath)
		if err != nil {
			return nil, err
		}
		switch entry.Mode {
		case ModeManaged:
			if exists {
				return nil, fmt.Errorf("managed target already exists: %s", entry.Path)
			}
		case ModeTemplate:
			if exists {
				continue
			}
			data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("source template not found in source repository: %w", err)
			}
			templates = append(templates, initialTemplatePlacement{targetPath: targetPath, sourceBytes: data})
		}
	}
	return templates, nil
}

func targetPathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
```

- [ ] **Step 4: Run focused tests to verify they pass**

Run: `go test ./src/internal/driftline -run 'TestAdoptInitialTargetRepository|TestInitialAdoption' -count=1`

Expected: PASS.

## Task 2: Wire `runInit` Through Initial Adoption

**Files:**
- Modify: `src/internal/driftline/commands/init.go`
- Test: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Replace command-owned Target Repository adoption logic**

Replace `src/internal/driftline/commands/init.go` with:

```go
package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runInit(source driftline.SourceClient, opts InitOptions, stdout io.Writer) error {
	if opts.TargetDir == "" {
		opts.TargetDir = "."
	}
	if err := driftline.ValidateRepository(opts.Repository); err != nil {
		return err
	}
	info, err := os.Stat(opts.TargetDir)
	if err != nil {
		return fmt.Errorf("target directory must exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target directory must be a directory: %s", opts.TargetDir)
	}

	ref := opts.Ref
	commit := ""
	if ref == "" {
		var err error
		ref, commit, err = source.ResolveDefaultRef(opts.Repository)
		if err != nil {
			return err
		}
	} else {
		if err := driftline.ValidateRef(ref); err != nil {
			return err
		}
		var err error
		commit, err = source.ResolveRef(opts.Repository, ref)
		if err != nil {
			return err
		}
	}
	manifestBytes, err := source.ReadFile(opts.Repository, commit, driftline.SourceManifestPath)
	if err != nil {
		return fmt.Errorf(".driftline-source.toml not found in source repository: %w", err)
	}
	manifest, err := driftline.LoadSourceManifestBytes(manifestBytes)
	if err != nil {
		return err
	}
	config, err := driftline.TargetConfigFromSourceManifest(opts.Repository, ref, manifest)
	if err != nil {
		return err
	}
	if err := driftline.AdoptInitialTargetRepository(driftline.InitialAdoptionOptions{
		Root:         opts.TargetDir,
		Source:       source,
		Repository:   opts.Repository,
		Commit:       commit,
		Manifest:     manifest,
		TargetConfig: config,
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created .driftline-target.toml from %s@%s\n", opts.Repository, commit)
	return nil
}
```

- [ ] **Step 2: Run existing init command tests**

Run: `go test ./src/internal/driftline/commands -run TestInit -count=1`

Expected: PASS.

- [ ] **Step 3: Run command tests covering removed surfaces**

Run: `go test ./src/internal/driftline/commands -run 'TestInit|TestHelpOmitsPruneAndPruneCommandIsRemoved|TestParseOptions' -count=1`

Expected: PASS.

## Task 3: Verify Package Integration

**Files:**
- Verify: `src/internal/driftline/initial_adoption.go`
- Verify: `src/internal/driftline/commands/init.go`
- Verify: `src/internal/driftline/initial_adoption_test.go`

- [ ] **Step 1: Run driftline package tests**

Run: `go test ./src/internal/driftline -count=1`

Expected: PASS.

- [ ] **Step 2: Run command package tests**

Run: `go test ./src/internal/driftline/commands -count=1`

Expected: PASS.

- [ ] **Step 3: Run full Go test suite**

Run: `go test ./... -count=1`

Expected: PASS.

## Task 4: Final Review And Cleanup

**Files:**
- Review: `docs/superpowers/specs/2026-06-28-initial-adoption-module-design.md`
- Review: `docs/superpowers/plans/2026-06-28-initial-adoption-module.md`
- Review: `src/internal/driftline/initial_adoption.go`
- Review: `src/internal/driftline/initial_adoption_test.go`
- Review: `src/internal/driftline/commands/init.go`

- [ ] **Step 1: Verify spec coverage manually**

Check that implementation satisfies these requirements:

```text
runInit remains responsible for CLI validation, Source Repository ref resolution, Source Config loading, Target manifest derivation, and stdout
Initial adoption Module rejects existing Target manifest before writes
Initial adoption Module rejects existing Managed file targets before writes
Initial adoption Module rejects reserved paths before writes
Initial adoption Module rejects missing Template source bytes before writes
Initial adoption Module prepares Target manifest before Template placement
Initial adoption Module writes missing Template files before committing Target manifest
Initial adoption Module does not commit Target manifest when Template write fails
Initial adoption Module does not rollback Template files when Target manifest commit fails
init command output is unchanged
no YAML, lock-file, path_overrides, if_not_exists, or prune compatibility is introduced
```

- [ ] **Step 2: Inspect git diff**

Run: `git diff -- docs/superpowers/specs/2026-06-28-initial-adoption-module-design.md docs/superpowers/plans/2026-06-28-initial-adoption-module.md src/internal/driftline/initial_adoption.go src/internal/driftline/initial_adoption_test.go src/internal/driftline/commands/init.go`

Expected: diff only contains the Initial adoption design, the implementation plan, the new Initial adoption Module, its tests, and the simplified init command.

- [ ] **Step 3: Inspect untracked files**

Run: `git status --short`

Expected: only intended files are new or modified. The expected changed files are:

```text
?? docs/superpowers/specs/2026-06-28-initial-adoption-module-design.md
?? docs/superpowers/plans/2026-06-28-initial-adoption-module.md
?? src/internal/driftline/initial_adoption.go
?? src/internal/driftline/initial_adoption_test.go
 M src/internal/driftline/commands/init.go
```

If other files appear, inspect them before reporting.

- [ ] **Step 4: Run final verification**

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 5: Report results**

Report changed files and exact verification command output. Do not commit unless the user explicitly asks for a commit.
