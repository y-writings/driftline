# Driftline Metadata Layout Implementation Plan

<!-- markdownlint-disable MD010 MD013 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the root-level driftline metadata files with `.driftline/contract.toml` and `.driftline/sync.toml`, reserve the entire `.driftline/` subtree, and make metadata reads and writes safe around symlinks and unsupported file types.

**Architecture:** Keep TOML parsing and repository-path validation in `config.go`, and introduce `metadata.go` as the only local filesystem boundary for the Sync manifest. Planning reads validated metadata and decides all changes, application prepares any Sync rewrite before mutating Managed files, and commands only select exact artifacts and report results. Rename internal Source Config and Target manifest models directly to Contract and Sync manifest terminology without compatibility aliases.

**Tech Stack:** Go 1.26.3, standard library filesystem APIs, BurntSushi TOML 1.1 parser, existing `go test` suite.

---

## Authoritative Requirements

- Layout and naming: `docs/superpowers/specs/2026-07-18-driftline-metadata-layout-design.md`
- Managed/Template behavior and TOML shape: `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`
- Breaking-change policy: `AGENTS.md`

The implementation must not add old-path reads, aliases, migration commands, compatibility warnings, or dual writes. The TOML schema remains version 2, including the `[source]` table in the Sync manifest.

## File And Responsibility Map

**Create:**

- `src/internal/driftline/metadata.go`: canonical local metadata paths, no-follow directory/file inspection, safe Sync manifest load/create/rewrite preparation.
- `src/internal/driftline/metadata_test.go`: metadata-directory and Sync-manifest filesystem contract tests.
- `.driftline/sync.toml`: this repository's moved Sync manifest.

**Modify:**

- `src/internal/driftline/types.go`: Contract, Sync manifest, entry, plan-status vocabulary.
- `src/internal/driftline/config.go`: TOML parsing/formatting, schema validation, complete `.driftline/` reservation.
- `src/internal/driftline/config_test.go`: artifact vocabulary and reserved-path matrix.
- `src/internal/driftline/plan.go`: exact metadata reads, Contract/Sync model, Sync manifest change statuses.
- `src/internal/driftline/plan_test.go`: exact paths, no fallback, renamed plan output.
- `src/internal/driftline/initial_adoption.go`: safe Sync creation and dual-role preservation.
- `src/internal/driftline/initial_adoption_test.go`: metadata preflight and commit-last behavior.
- `src/internal/driftline/target_repository.go`: safe Sync rewrite before Managed-file mutations.
- `src/internal/driftline/target_repository_test.go`: rewrite preflight, ordering, and Contract preservation.
- `src/internal/driftline/commands/init.go`: early metadata preflight and exact Contract fetch.
- `src/internal/driftline/commands/run.go`: help text.
- `src/internal/driftline/commands/check.go`: conflict guidance.
- `src/internal/driftline/commands/commands_test.go`: end-to-end paths, output, no fallback, and dual-role behavior.
- `README.md`: current Contract and Sync manifest user documentation.
- `CONTEXT.md`: current artifact terminology.
- `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`: active body terminology and paths.
- `docs/superpowers/specs/2026-06-28-initial-adoption-module-design.md`: current module guarantees.
- `docs/superpowers/specs/2026-06-28-target-repository-apply-module-design.md`: current module guarantees.
- `docs/superpowers/specs/2026-07-03-init-force-adoption-design.md`: current init guarantees.

**Delete:**

- `.driftline-target.toml`: replaced directly by `.driftline/sync.toml`.

**Leave historical:**

- Dated files under `docs/superpowers/plans/` remain execution records and may retain old names.
- The migration-policy passages in `docs/superpowers/specs/2026-07-18-driftline-metadata-layout-design.md` intentionally retain old names.

## Responsibility Boundaries

| Boundary            | Owns                                                                                             | Does not own                    |
| ------------------- | ------------------------------------------------------------------------------------------------ | ------------------------------- |
| Parse/validate      | TOML, unknown fields, IDs, repository/ref, normalized duplicate paths, `.driftline/` reservation | Filesystem state or writes      |
| Metadata filesystem | Exact local metadata paths, `Lstat`, create-versus-rewrite policy, temp files, mode preservation | Managed-set decisions           |
| Plan                | Safe Sync load, exact Contract fetch, complete conflicts and desired changes                     | Directory or file mutation      |
| Apply               | Prepare metadata rewrite, delete/write Managed files, commit Sync last                           | New policy or reporting         |
| Report              | Exact artifact/path wording and conflict guidance                                                | Planning or filesystem behavior |

## Edge-Case Contract

| Case                                                                               | Required result                                      |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------- |
| Contract only in local repository                                                  | `init` may add Sync without modifying Contract       |
| Sync only in local repository                                                      | Target commands work; `init` rejects existing Sync   |
| Both artifacts                                                                     | Target commands never rewrite Contract               |
| Only old root Contract exists remotely                                             | Contract-not-found error for the new path            |
| Only old root Sync exists locally                                                  | Sync-not-found error for the new path                |
| `.driftline` missing during init                                                   | Create a real directory with mode `0755`             |
| `.driftline` missing during target command                                         | Report missing Sync; do not create the directory     |
| `.driftline` is file, symlink, or broken symlink                                   | Fail before source access or Managed/Template writes |
| `sync.toml` is directory, symlink, broken symlink, or other non-regular type       | Reject without reading through or replacing it       |
| Sync rewrite requested after Sync disappears                                       | Fail before Managed-file writes; do not recreate it  |
| `.driftline`, `.driftline/x`, or normalized descendants in either schema           | Reject before planning                               |
| `.driftline-file`, `nested/.driftline/x`, old root names, or `driftline-lock.yaml` | Treat as ordinary allowed repository paths           |

### Task 1: Align Internal Artifact Vocabulary

**Files:**

- Modify: `src/internal/driftline/types.go:12-59`
- Modify: `src/internal/driftline/config.go:24-289`
- Modify: `src/internal/driftline/plan.go:18-194, 241-345`
- Modify: `src/internal/driftline/initial_adoption.go:11-134`
- Modify: `src/internal/driftline/target_repository.go:15-84`
- Modify: `src/internal/driftline/config_test.go`
- Modify: `src/internal/driftline/plan_test.go`
- Modify: `src/internal/driftline/initial_adoption_test.go`
- Modify: `src/internal/driftline/target_repository_test.go`
- Modify: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Establish the green refactor baseline**

Run:

```sh
go test ./...
```

Expected: all packages pass before any rename.

- [ ] **Step 2: Replace the model and status declarations without aliases**

Replace the artifact-oriented declarations in `types.go` with:

```go
type Contract struct {
	Version int                                `toml:"version"`
	Files   map[string]map[string]ContractFile `toml:"files"`
}

type ContractFile struct {
	Path string   `toml:"path"`
	Mode FileMode `toml:"mode"`
}

type SyncManifest struct {
	Version int                          `toml:"version"`
	Source  SyncSource                   `toml:"source"`
	Files   map[string]map[string]string `toml:"files"`
}

type SyncSource struct {
	Repository string `toml:"repository"`
	Ref        string `toml:"ref"`
}

type ContractEntry struct {
	Group string
	File  string
	Key   string
	Path  string
	Mode  FileMode
}

type SyncEntry struct {
	Group string
	File  string
	Key   string
	Path  string
}

const (
	StatusSynced            Status = "synced"
	StatusAdd               Status = "add"
	StatusUpdate            Status = "update"
	StatusRemove            Status = "remove"
	StatusSyncManifestAdd   Status = "sync-manifest-add"
	StatusSyncManifestRemove Status = "sync-manifest-remove"
	StatusModeTransition    Status = "mode-transition"
	StatusConflict          Status = "conflict"
)
```

Run `gofmt` after adding the final declarations so the constant alignment is canonical.

- [ ] **Step 3: Rename parser and formatter APIs directly**

Apply this exact mapping in `config.go` and all callers:

```text
LoadSourceManifestBytes        -> LoadContractBytes
LoadTargetConfigBytes          -> LoadSyncManifestBytes
FormatTargetConfig             -> FormatSyncManifest
TargetConfigFromSourceManifest -> SyncManifestFromContract
SourceEntries                  -> ContractEntries
TargetEntries                  -> SyncEntries
validateSourceManifest         -> validateContract
validateTargetConfig           -> validateSyncManifest
ensureTargetGroup              -> ensureSyncGroup
```

Leave `LoadTargetConfig`, `WriteTargetConfig`, `PrepareTargetConfigWrite`, and the old path constants in place during this green refactor, changing only their model parameter and return types to `SyncManifest`. Task 2 introduces the safe root-based APIs beside them; Task 4 switches every caller and deletes the arbitrary-path functions and old constants. They are temporary internals, not compatibility surfaces.

Use artifact-specific decode labels and errors:

```go
func LoadContractBytes(data []byte) (Contract, error) {
	var contract Contract
	metadata, err := toml.Decode(string(data), &contract)
	if err != nil {
		return contract, fmt.Errorf("parse Contract: %w", err)
	}
	if err := rejectUndecoded("Contract", metadata.Undecoded()); err != nil {
		return contract, err
	}
	return contract, validateContract(contract)
}

func LoadSyncManifestBytes(data []byte) (SyncManifest, error) {
	var manifest SyncManifest
	metadata, err := toml.Decode(string(data), &manifest)
	if err != nil {
		return manifest, fmt.Errorf("parse Sync manifest: %w", err)
	}
	if err := rejectUndecoded("Sync manifest", metadata.Undecoded()); err != nil {
		return manifest, err
	}
	return manifest, validateSyncManifest(manifest)
}
```

Keep the `[source]` field and TOML tag unchanged. Rename only the artifact model, not the approved schema.

- [ ] **Step 4: Rename plan and application fields**

Use this final `Plan` shape:

```go
type Plan struct {
	Repository       string
	Ref              string
	Commit           string
	SyncManifest     SyncManifest
	Contract         Contract
	Changes          []Change
	NextSyncManifest SyncManifest
}
```

Apply these exact internal replacements:

```text
planBuilder.config/manifest                 -> syncManifest/contract
resolvedManagedFile.SourceEntry             -> ContractEntry
sourceByKey/managed                         -> contractByKey/managed
targetByKey/declaredTargets                 -> syncByKey/declaredTargets
targetConfigRemoveChange                    -> syncManifestRemoveChange
planHasTargetConfigChanges                  -> planHasSyncManifestChanges
InitialAdoptionOptions.Manifest             -> Contract
InitialAdoptionOptions.TargetConfig         -> SyncManifest
```

Keep the existing injected prepare-writer field until Task 4, updating only its model parameter type to `SyncManifest`. Task 4 replaces that arbitrary-path seam with `prepareSyncManifestCreate(root, manifest)`.

Change plan reasons to:

```go
Reason: "add Sync manifest entry"
Reason: "remove Sync manifest entry"
Reason: "managed file removed from Contract"
```

- [ ] **Step 5: Update test types, helpers, names, statuses, and expected reasons**

Rename helpers consistently:

```text
targetConfigTOML                 -> syncManifestTOML
targetConfigText                 -> syncManifestText
targetConfigTOMLForApplyTest     -> syncManifestTOMLForApplyTest
initialAdoptionManifest          -> initialAdoptionContract
initialAdoptionTargetConfig      -> initialAdoptionSyncManifest
newPlanSourceClient              -> newPlanSourceClient (keep endpoint name)
newCommandSourceClient           -> newCommandSourceClient (keep endpoint name)
```

Do not keep old helper wrappers. Update expected status strings to `sync-manifest-add` and `sync-manifest-remove`.

- [ ] **Step 6: Format and verify the green refactor**

Run:

```sh
gofmt -w src/internal/driftline src/internal/driftline/commands
go test ./...
```

Expected: all packages pass with no old model, entry, status, parser, formatter, or plan-field identifiers remaining. The arbitrary-path filesystem function names and old path constants remain temporarily until Task 4.

- [ ] **Step 7: Commit the vocabulary refactor**

```sh
git add src/internal/driftline
git commit -m "refactor: align contract and sync terminology"
```

### Task 2: Add Safe Sync Metadata Filesystem Operations

**Files:**

- Create: `src/internal/driftline/metadata.go`
- Create: `src/internal/driftline/metadata_test.go`
- Modify: `src/internal/driftline/config.go:3-109`

- [ ] **Step 1: Write failing create/load/rewrite tests**

Create `metadata_test.go` with table-driven coverage using this common manifest:

```go
func testSyncManifest() SyncManifest {
	return SyncManifest{
		Version: 2,
		Source:  SyncSource{Repository: "y-writings/source-repo", Ref: "main"},
		Files:   map[string]map[string]string{"tool": {"config": "tool.toml"}},
	}
}

func TestPrepareSyncManifestCreateCreatesMetadataDirectoryAndWaitsToCommitManifest(t *testing.T) {
	root := t.TempDir()
	commit, cleanup, err := PrepareSyncManifestCreate(root, testSyncManifest())
	if err != nil {
		t.Fatalf("prepare create failed: %v", err)
	}
	defer cleanup()
	if _, err := os.Lstat(filepath.Join(root, SyncManifestPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Sync manifest must be absent before commit: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if _, err := LoadSyncManifest(root); err != nil {
		t.Fatalf("committed Sync manifest should load: %v", err)
	}
}

func TestSyncMetadataRejectsUnsafeDirectoryAndManifestPaths(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{"metadata file", func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, MetadataDirectoryPath), []byte("blocked"), 0o644); err != nil { t.Fatal(err) }
		}},
		{"metadata symlink", func(t *testing.T, root string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, MetadataDirectoryPath)); err != nil { t.Fatal(err) }
		}},
		{"Sync directory", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, SyncManifestPath), 0o755); err != nil { t.Fatal(err) }
		}},
		{"Sync symlink", func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, MetadataDirectoryPath), 0o755); err != nil { t.Fatal(err) }
			if err := os.Symlink(filepath.Join(t.TempDir(), "outside.toml"), filepath.Join(root, SyncManifestPath)); err != nil { t.Fatal(err) }
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			if err := ValidateSyncManifestCreation(root); err == nil {
				t.Fatal("expected unsafe metadata error")
			}
			if _, err := LoadSyncManifest(root); err == nil {
				t.Fatal("expected unsafe metadata load error")
			}
		})
	}
}

func TestPrepareSyncManifestRewritePreservesModeAndWaitsForCommit(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, MetadataDirectoryPath), 0o755); err != nil { t.Fatal(err) }
	path := filepath.Join(root, SyncManifestPath)
	if err := os.WriteFile(path, []byte(syncManifestText("y-writings/old-source")), 0o640); err != nil { t.Fatal(err) }
	if err := os.Chmod(path, 0o640); err != nil { t.Fatal(err) }

	commit, cleanup, err := PrepareSyncManifestRewrite(root, testSyncManifest())
	if err != nil { t.Fatalf("prepare rewrite failed: %v", err) }
	defer cleanup()
	before, err := LoadSyncManifest(root)
	if err != nil || before.Source.Repository != "y-writings/old-source" {
		t.Fatalf("rewrite must wait for commit: manifest=%#v err=%v", before, err)
	}
	if err := commit(); err != nil { t.Fatalf("commit failed: %v", err) }
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("rewrite must preserve mode: info=%#v err=%v", info, err)
	}
}

func TestPrepareSyncManifestRewriteRequiresExistingRegularManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, MetadataDirectoryPath), 0o755); err != nil { t.Fatal(err) }
	if _, _, err := PrepareSyncManifestRewrite(root, testSyncManifest()); err == nil {
		t.Fatal("rewrite must not recreate a missing Sync manifest")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run:

```sh
go test ./src/internal/driftline -run 'Test(PrepareSyncManifest|SyncMetadata)'
```

Expected: build failure because the metadata constants and APIs do not exist.

- [ ] **Step 3: Add canonical path constants and no-follow inspection**

Add the canonical constants beside the temporarily retained old path constants in `config.go`:

```go
const (
	MetadataDirectoryPath = ".driftline"
	ContractPath          = MetadataDirectoryPath + "/contract.toml"
	SyncManifestPath      = MetadataDirectoryPath + "/sync.toml"
)
```

Create `metadata.go` with this API and policy:

```go
package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ValidateSyncManifestCreation(root string) error {
	dir, exists, err := inspectMetadataDirectory(root)
	if err != nil || !exists {
		return err
	}
	path := filepath.Join(dir, "sync.toml")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Sync manifest %s: %w", SyncManifestPath, err)
	}
	if info.Mode().IsRegular() {
		return fmt.Errorf("Sync manifest already exists: %s", SyncManifestPath)
	}
	return fmt.Errorf("Sync manifest path is not a regular file: %s", SyncManifestPath)
}

func LoadSyncManifest(root string) (SyncManifest, error) {
	dir, exists, err := inspectMetadataDirectory(root)
	if err != nil {
		return SyncManifest{}, err
	}
	if !exists {
		return SyncManifest{}, fmt.Errorf("Sync manifest not found: %s", SyncManifestPath)
	}
	path := filepath.Join(dir, "sync.toml")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return SyncManifest{}, fmt.Errorf("Sync manifest not found: %s", SyncManifestPath)
	}
	if err != nil {
		return SyncManifest{}, fmt.Errorf("inspect Sync manifest %s: %w", SyncManifestPath, err)
	}
	if !info.Mode().IsRegular() {
		return SyncManifest{}, fmt.Errorf("Sync manifest path is not a regular file: %s", SyncManifestPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SyncManifest{}, fmt.Errorf("read Sync manifest %s: %w", SyncManifestPath, err)
	}
	return LoadSyncManifestBytes(data)
}

func inspectMetadataDirectory(root string) (string, bool, error) {
	if root == "" {
		root = "."
	}
	dir := filepath.Join(root, MetadataDirectoryPath)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return dir, false, nil
	}
	if err != nil {
		return dir, false, fmt.Errorf("inspect driftline metadata directory %s: %w", MetadataDirectoryPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return dir, false, fmt.Errorf("driftline metadata path is not a real directory: %s", MetadataDirectoryPath)
	}
	return dir, true, nil
}
```

- [ ] **Step 4: Implement separate create and rewrite preparation**

Add these functions to `metadata.go`. Both must validate the manifest, create a temp file in `.driftline/`, write `FormatSyncManifest(manifest)`, apply the selected mode, close it, and return commit/cleanup closures.

```go
func PrepareSyncManifestCreate(root string, manifest SyncManifest) (func() error, func() error, error) {
	if err := validateSyncManifest(manifest); err != nil {
		return nil, nil, err
	}
	if err := ValidateSyncManifestCreation(root); err != nil {
		return nil, nil, err
	}
	dir, exists, err := inspectMetadataDirectory(root)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create driftline metadata directory %s: %w", MetadataDirectoryPath, err)
		}
	}
	if err := ValidateSyncManifestCreation(root); err != nil {
		return nil, nil, err
	}
	return prepareSyncManifest(root, manifest, 0o644, true)
}

func PrepareSyncManifestRewrite(root string, manifest SyncManifest) (func() error, func() error, error) {
	if err := validateSyncManifest(manifest); err != nil {
		return nil, nil, err
	}
	_, info, err := existingSyncManifest(root)
	if err != nil {
		return nil, nil, err
	}
	return prepareSyncManifest(root, manifest, info.Mode().Perm(), false)
}

func existingSyncManifest(root string) (string, os.FileInfo, error) {
	dir, exists, err := inspectMetadataDirectory(root)
	if err != nil {
		return "", nil, err
	}
	if !exists {
		return "", nil, fmt.Errorf("Sync manifest not found: %s", SyncManifestPath)
	}
	path := filepath.Join(dir, "sync.toml")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("Sync manifest not found: %s", SyncManifestPath)
	}
	if err != nil {
		return "", nil, fmt.Errorf("inspect Sync manifest %s: %w", SyncManifestPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("Sync manifest path is not a regular file: %s", SyncManifestPath)
	}
	return path, info, nil
}
```

Implement `prepareSyncManifest` so the commit closure re-runs `ValidateSyncManifestCreation` for create or `existingSyncManifest` for rewrite immediately before `os.Rename`. Use `os.CreateTemp(dir, ".sync-*.toml")`; do not use `MkdirAll` or `Stat` anywhere in metadata operations. Cleanup must ignore `os.ErrNotExist` after a successful rename.

Use this complete helper shape:

```go
func prepareSyncManifest(root string, manifest SyncManifest, mode os.FileMode, create bool) (func() error, func() error, error) {
	dir, exists, err := inspectMetadataDirectory(root)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, fmt.Errorf("driftline metadata directory not found: %s", MetadataDirectoryPath)
	}
	temp, err := os.CreateTemp(dir, ".sync-*.toml")
	if err != nil {
		return nil, nil, fmt.Errorf("create Sync manifest temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() error {
		err := os.Remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	fail := func(err error) (func() error, func() error, error) {
		temp.Close()
		cleanup()
		return nil, nil, err
	}
	if _, err := temp.WriteString(FormatSyncManifest(manifest)); err != nil {
		return fail(fmt.Errorf("write Sync manifest temp file: %w", err))
	}
	if err := temp.Chmod(mode); err != nil {
		return fail(fmt.Errorf("chmod Sync manifest temp file: %w", err))
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close Sync manifest temp file: %w", err)
	}
	commit := func() error {
		if create {
			if err := ValidateSyncManifestCreation(root); err != nil {
				return err
			}
		} else if _, _, err := existingSyncManifest(root); err != nil {
			return err
		}
		return os.Rename(tempName, filepath.Join(root, SyncManifestPath))
	}
	return commit, cleanup, nil
}
```

- [ ] **Step 5: Keep old filesystem callers isolated until the atomic switch**

Do not integrate the new APIs yet. Keep the existing arbitrary-path functions only until Task 4 so every intermediate commit compiles and preserves current behavior. `metadata.go` is the tested final boundary that Task 4 will adopt.

- [ ] **Step 6: Run focused and package tests**

Run:

```sh
gofmt -w src/internal/driftline/metadata.go src/internal/driftline/metadata_test.go src/internal/driftline/config.go
go test ./src/internal/driftline -run 'Test(PrepareSyncManifest|SyncMetadata|LoadSyncManifest)'
go test ./src/internal/driftline
```

Expected: all focused and package tests pass.

- [ ] **Step 7: Commit the metadata filesystem boundary**

```sh
git add src/internal/driftline/config.go src/internal/driftline/metadata.go src/internal/driftline/metadata_test.go src/internal/driftline/config_test.go
git commit -m "feat: add safe sync metadata operations"
```

### Task 3: Reserve The Complete Metadata Subtree

**Files:**

- Modify: `src/internal/driftline/config.go:212-289, 316-329`
- Modify: `src/internal/driftline/config_test.go:64-128, 248-262`
- Modify: `src/internal/driftline/plan.go:362-369`

- [ ] **Step 1: Add the failing Contract reservation matrix**

Add:

```go
func TestLoadContractRejectsReservedMetadataPaths(t *testing.T) {
	for _, mode := range []FileMode{ModeManaged, ModeTemplate} {
		for _, path := range []string{".driftline", ".driftline/contract.toml", ".driftline/future/file", "./.driftline/future", ".driftline/./future"} {
			t.Run(string(mode)+" "+path, func(t *testing.T) {
				input := fmt.Sprintf("version = 2\n[files.tool]\nconfig = { path = %q, mode = %q }\n", path, mode)
				_, err := LoadContractBytes([]byte(input))
				if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "reserved driftline metadata path") {
					t.Fatalf("expected exact reserved path error for %q, got %v", path, err)
				}
			})
		}
	}
}
```

- [ ] **Step 2: Add the failing Sync and near-miss matrix**

Add:

```go
func TestLoadSyncManifestRejectsReservedMetadataPaths(t *testing.T) {
	for _, path := range []string{".driftline", ".driftline/sync.toml", ".driftline/future/file", "./.driftline/future", ".driftline/./future"} {
		t.Run(path, func(t *testing.T) {
			input := fmt.Sprintf("version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.tool]\nconfig = %q\n", path)
			_, err := LoadSyncManifestBytes([]byte(input))
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "reserved driftline metadata path") {
				t.Fatalf("expected exact reserved path error for %q, got %v", path, err)
			}
		})
	}
}

func TestMetadataNearMissesAreOrdinaryPaths(t *testing.T) {
	for _, path := range []string{".driftline-file", ".driftliner/file", "nested/.driftline/file"} {
		contract := fmt.Sprintf("version = 2\n[files.tool]\nconfig = { path = %q, mode = \"managed\" }\n", path)
		if _, err := LoadContractBytes([]byte(contract)); err != nil {
			t.Fatalf("Contract path %q should be ordinary: %v", path, err)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify the current exact-name policy fails**

Run:

```sh
go test ./src/internal/driftline -run 'Test(LoadContractRejectsReserved|LoadSyncManifestRejectsReserved|MetadataNearMisses)'
```

Expected: Contract cases and descendant Sync cases fail because only old exact root names are currently reserved.

- [ ] **Step 4: Implement one normalized reservation policy in schema validation**

Add:

```go
func IsReservedMetadataPath(name string) bool {
	name = normalizedConfigPath(name)
	return name == MetadataDirectoryPath || strings.HasPrefix(name, MetadataDirectoryPath+"/")
}

func validateUnreservedMetadataPath(name string, label string) error {
	if IsReservedMetadataPath(name) {
		return fmt.Errorf("%s uses reserved driftline metadata path: %s", label, name)
	}
	return nil
}
```

Call it after syntax validation and before duplicate checks:

```go
if err := validateUnreservedMetadataPath(item.Path, fmt.Sprintf("Contract file %q", key)); err != nil {
	return err
}
```

```go
if err := validateUnreservedMetadataPath(targetPath, fmt.Sprintf("Sync manifest file %q", key)); err != nil {
	return err
}
```

Keep the old exact root-path and removed-lock reservations temporarily because those files are still active before Task 4. Replace duplicate planner and Initial adoption checks with the new subtree predicate plus the temporary exact-root checks. Task 4 removes the old reservations in the same change that switches every caller to canonical metadata and adds tests proving the old names are ordinary paths.

- [ ] **Step 5: Keep generic path validation syntax-only**

Do not add the reservation to `ValidateConfigPath`; the source client must be able to fetch `.driftline/contract.toml`. The reservation applies only to paths declared inside Contract and Sync schemas.

- [ ] **Step 6: Verify validation and the full suite**

Run:

```sh
gofmt -w src/internal/driftline/config.go src/internal/driftline/config_test.go src/internal/driftline/plan.go src/internal/driftline/initial_adoption.go
go test ./src/internal/driftline -run 'Test(LoadContractRejectsReserved|LoadSyncManifestRejectsReserved|MetadataNearMisses)'
go test ./...
```

Expected: all tests pass; the complete `.driftline/` subtree is newly reserved while temporary old exact reservations remain until the atomic path switch in Task 4.

- [ ] **Step 7: Commit the reserved namespace**

```sh
git add src/internal/driftline/config.go src/internal/driftline/config_test.go src/internal/driftline/plan.go src/internal/driftline/initial_adoption.go
git commit -m "feat: reserve driftline metadata subtree"
```

### Task 4: Switch Init, Planning, And Apply To Canonical Metadata

**Files:**

- Modify: `src/internal/driftline/commands/init.go:13-76`
- Modify: `src/internal/driftline/initial_adoption.go:11-80`
- Modify: `src/internal/driftline/plan.go:37-70`
- Modify: `src/internal/driftline/target_repository.go:15-32`
- Modify: `src/internal/driftline/commands/commands_test.go:55-280, 724-746`
- Modify: `src/internal/driftline/initial_adoption_test.go`
- Modify: `src/internal/driftline/plan_test.go`
- Modify: `src/internal/driftline/target_repository_test.go`

- [ ] **Step 1: Convert the init happy path into a failing exact-path tracer test**

Rename the existing happy-path test to `TestInitReadsContractAndCreatesSyncManifest`. Change its assertions to:

```go
syncManifest := readFile(t, targetDir, driftline.SyncManifestPath)
for _, want := range []string{"version = 2", `[source]`, `repository = "y-writings/source-repo"`, `[files.github-workflow]`, `ci = ".github/workflows/ci.yaml"`} {
	if !strings.Contains(syncManifest, want) {
		t.Fatalf("generated Sync manifest missing %q:\n%s", want, syncManifest)
	}
}
if !strings.Contains(stdout.String(), "created Sync manifest .driftline/sync.toml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567") {
	t.Fatalf("unexpected stdout: %q", stdout.String())
}
```

Change fake-source keys from `SourceManifestPath` to `ContractPath`.

- [ ] **Step 2: Add no-fallback tests before changing production callers**

Add command/plan tests with only old files present:

```go
func TestInitDoesNotReadOldRootContract(t *testing.T) {
	targetDir := t.TempDir()
	client := newCommandSourceClient("main", "", nil)
	commit := "0123456789abcdef0123456789abcdef01234567"
	delete(client.files, "y-writings/source-repo@"+commit+":"+driftline.ContractPath)
	client.files["y-writings/source-repo@"+commit+":.driftline-source.toml"] = []byte("version = 2\n")
	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "Contract not found: .driftline/contract.toml") {
		t.Fatalf("expected canonical Contract error, got %v", err)
	}
	assertFileMissing(t, targetDir, driftline.SyncManifestPath)
}

func TestBuildPlanDoesNotReadOldRootSyncManifest(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.toml", syncManifestTOML(""))
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: fakeSourceClient{}})
	if err == nil || !strings.Contains(err.Error(), "Sync manifest not found: .driftline/sync.toml") {
		t.Fatalf("expected canonical Sync error, got %v", err)
	}
}
```

- [ ] **Step 3: Run exact-path tests and verify they fail**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestInit(ReadsContract|DoesNotReadOldRootContract)'
go test ./src/internal/driftline -run 'TestBuildPlanDoesNotReadOldRootSyncManifest'
```

Expected: failures show old constants/read paths or missing new integration.

- [ ] **Step 4: Integrate safe preflight and creation into init**

Use this command flow in `runInit`:

```go
if err := driftline.ValidateSyncManifestCreation(opts.TargetDir); err != nil {
	return err
}

// Resolve the requested ref exactly as today.

contractBytes, err := source.ReadFile(opts.Repository, commit, driftline.ContractPath)
if err != nil {
	return fmt.Errorf("Contract not found: %s: %w", driftline.ContractPath, err)
}
contract, err := driftline.LoadContractBytes(contractBytes)
if err != nil {
	return err
}
syncManifest, err := driftline.SyncManifestFromContract(opts.Repository, ref, contract)
if err != nil {
	return err
}
if err := driftline.AdoptInitialTargetRepository(driftline.InitialAdoptionOptions{
	Root:                        opts.TargetDir,
	Source:                      source,
	Repository:                  opts.Repository,
	Commit:                      commit,
	Contract:                    contract,
	SyncManifest:                syncManifest,
	AdoptExistingManagedTargets: opts.Force,
}); err != nil {
	return err
}
fmt.Fprintf(stdout, "created Sync manifest %s from %s@%s\n", driftline.SyncManifestPath, opts.Repository, commit)
```

In `initialAdoption.adopt`, validate the received Contract and Sync manifest before collecting Templates, call `ValidateSyncManifestCreation(root)` before any source reads, then call `PrepareSyncManifestCreate(root, opts.SyncManifest)`. Remove all arbitrary-path writer parameters.

- [ ] **Step 5: Integrate safe reads into planning**

Replace the opening of `BuildPlan` with:

```go
syncManifest, err := LoadSyncManifest(opts.TargetDir)
if err != nil {
	return Plan{}, err
}
commit, err := opts.Source.ResolveRef(syncManifest.Source.Repository, syncManifest.Source.Ref)
if err != nil {
	return Plan{}, err
}
contractBytes, err := opts.Source.ReadFile(syncManifest.Source.Repository, commit, ContractPath)
if err != nil {
	return Plan{}, fmt.Errorf("Contract not found: %s: %w", ContractPath, err)
}
contract, err := LoadContractBytes(contractBytes)
if err != nil {
	return Plan{}, err
}

builder := planBuilder{opts: opts, syncManifest: syncManifest, contract: contract, commit: commit}
return builder.build()
```

Update all plan test setup to write `SyncManifestPath` and all fake sources to provide `ContractPath`.

- [ ] **Step 6: Integrate strict rewrite into apply**

Replace Sync preparation in `TargetRepository.Apply` with:

```go
var commitSyncManifest func() error
if planHasSyncManifestChanges(plan.Changes) {
	commit, cleanup, err := PrepareSyncManifestRewrite(root, plan.NextSyncManifest)
	if err != nil {
		return err
	}
	defer cleanup()
	commitSyncManifest = commit
}
```

Keep the existing delete-before-write loops, then commit last:

```go
if commitSyncManifest != nil {
	if err := commitSyncManifest(); err != nil {
		return err
	}
}
```

Do not call metadata write APIs for file-only or already-synced plans.

- [ ] **Step 7: Update all positive test fixtures to canonical paths**

Replace every positive use of the old constants in current Go tests. Retain old root literals only in explicit no-fallback and ordinary-payload tests. Replace the old temp prefix assertion with `.sync-*.toml`.

Remove `SourceManifestPath`, `TargetConfigPath`, `removedLockPath`, `LoadTargetConfig`, `WriteTargetConfig`, and `PrepareTargetConfigWrite` after their callers have moved. Add final Contract and Sync parsing tests proving `.driftline-source.toml`, `.driftline-target.toml`, and `driftline-lock.yaml` are ordinary paths. This keeps old reservation removal in the same tested change as the canonical-path switch.

- [ ] **Step 8: Run targeted integration tests and the full suite**

Run:

```sh
gofmt -w src/internal/driftline src/internal/driftline/commands
go test ./src/internal/driftline/commands -run 'TestInit'
go test ./src/internal/driftline -run 'Test(BuildPlan|AdoptInitialTargetRepository|TargetRepositoryApply)'
go test ./...
```

Expected: canonical-path, no-fallback, init, plan, and apply tests all pass.

- [ ] **Step 9: Commit the canonical path integration**

```sh
git add src/internal/driftline
git commit -m "feat: adopt canonical driftline metadata layout"
```

### Task 5: Harden Metadata Safety At Module Boundaries

**Files:**

- Modify: `src/internal/driftline/commands/commands_test.go`
- Modify: `src/internal/driftline/initial_adoption_test.go`
- Modify: `src/internal/driftline/target_repository_test.go`
- Modify: `src/internal/driftline/metadata.go`

- [ ] **Step 1: Add init preflight tests for every metadata-directory shape**

Add a table that creates `.driftline` as a regular file, directory symlink, and broken symlink, runs `init` with `sourceAccessFailingClient`, and asserts:

```go
if err == nil {
	t.Fatal("expected unsafe metadata error")
}
if strings.Contains(err.Error(), "source should not be accessed") {
	t.Fatalf("source was accessed before metadata rejection: %v", err)
}
assertFileMissing(t, outsideDir, "sync.toml")
```

Add equivalent cases where `.driftline` is a real directory but `sync.toml` is a directory, live symlink, or broken symlink.

- [ ] **Step 2: Add direct Initial adoption safety and dual-role tests**

Add:

```go
func TestAdoptInitialTargetRepositoryPreservesContract(t *testing.T) {
	root := t.TempDir()
	contractBytes := []byte("version = 2\n# local provider declaration\n")
	writeInitialAdoptionTestFile(t, root, ContractPath, string(contractBytes))
	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root: root, Source: &fakeInitialAdoptionSource{}, Repository: "y-writings/source-repo", Commit: "abc123",
		Contract: initialAdoptionManagedOnlyContract(), SyncManifest: initialAdoptionManagedOnlySyncManifest(),
	})
	if err != nil { t.Fatalf("adoption failed: %v", err) }
	if got := readInitialAdoptionTestFile(t, root, ContractPath); got != string(contractBytes) {
		t.Fatalf("init rewrote Contract: %q", got)
	}
}
```

Also call `AdoptInitialTargetRepository` directly with unsafe metadata and assert zero source reads, no Template writes, and no Sync temp file.

- [ ] **Step 3: Add apply-time rewrite preflight tests**

Add a test where a plan contains a Sync manifest addition and a Managed write, but `sync.toml` has been removed or replaced with a symlink before `Apply`:

```go
err := (TargetRepository{Root: root}).Apply(plan)
if err == nil {
	t.Fatal("expected Sync rewrite preflight failure")
}
if got := readTargetRepositoryTestFile(t, root, "managed.txt"); got != "old\n" {
	t.Fatalf("Managed file changed before Sync preflight: %q", got)
}
```

Add a sibling test proving a Contract adjacent to Sync is byte-for-byte unchanged after a successful rewrite.

- [ ] **Step 4: Verify creation and rewrite recheck their observed path state**

Ensure the commit closures in `metadata.go` call `ValidateSyncManifestCreation` or `existingSyncManifest` immediately before `os.Rename`. This does not promise protection against adversarial concurrent path swaps; it prevents normal stale prepared operations from overwriting or recreating metadata.

- [ ] **Step 5: Run hardening tests and the full suite**

Run:

```sh
gofmt -w src/internal/driftline src/internal/driftline/commands
go test ./src/internal/driftline -run 'Test(AdoptInitialTargetRepositoryPreservesContract|TargetRepositoryApply.*Sync|PrepareSyncManifest)'
go test ./src/internal/driftline/commands -run 'TestInit.*Metadata'
go test ./...
```

Expected: unsafe metadata fails before network/source access and before Managed/Template writes; dual-role Contract bytes remain unchanged.

- [ ] **Step 6: Commit the boundary hardening**

```sh
git add src/internal/driftline
git commit -m "test: harden metadata safety boundaries"
```

### Task 6: Update CLI Output And Artifact Diagnostics

**Files:**

- Modify: `src/internal/driftline/commands/run.go:192-215`
- Modify: `src/internal/driftline/commands/check.go:34-51`
- Modify: `src/internal/driftline/commands/commands_test.go:282-353, 523-648`
- Modify: `src/internal/driftline/plan.go:153-187, 241-253, 339-345`

- [ ] **Step 1: Write failing exact-output tests**

Update or add tests that require all of:

```text
create .driftline/sync.toml from a GitHub Source Repository
sync managed files and refresh .driftline/sync.toml
init-only ref to preserve in .driftline/sync.toml
created Sync manifest .driftline/sync.toml from owner/repo@commit
sync-manifest-add github-workflow.ci: add Sync manifest entry
sync-manifest-remove github-workflow.ci: remove Sync manifest entry
managed file removed from Contract
set another target path in .driftline/sync.toml
```

Assert help does not advertise a `sync` command and contains no old root path.

- [ ] **Step 2: Run output tests to verify failures**

Run:

```sh
go test ./src/internal/driftline/commands -run 'Test(Help|InitReports|CheckReports|DiffReports|UpdateConflict)'
```

Expected: failures identify remaining old artifact names or paths.

- [ ] **Step 3: Replace command help and conflict guidance**

Use this command section:

```text
commands:
  init owner/repo  create .driftline/sync.toml from a GitHub Source Repository
  check            check whether target files match the Source Repository
  diff             show diffs for files that would be added or updated
  update           sync managed files and refresh .driftline/sync.toml
```

Use:

```go
fmt.Fprintln(w, "  1. set another target path in .driftline/sync.toml")
```

Keep Source Repository and target-path terminology for relationship endpoints; only artifact names change.

- [ ] **Step 4: Use exact role-and-path errors**

Standardize these forms:

```text
Contract not found: .driftline/contract.toml
Sync manifest not found: .driftline/sync.toml
Sync manifest already exists: .driftline/sync.toml
Sync manifest path is not a regular file: .driftline/sync.toml
reserved driftline metadata path: .driftline/example.toml
```

Do not add warnings about old paths.

- [ ] **Step 5: Verify command output**

Run:

```sh
gofmt -w src/internal/driftline/commands src/internal/driftline/plan.go
go test ./src/internal/driftline/commands
go test ./...
```

Expected: all command and full-suite tests pass with new output.

- [ ] **Step 6: Commit output changes**

```sh
git add src/internal/driftline/commands src/internal/driftline/plan.go src/internal/driftline/plan_test.go
git commit -m "feat: report contract and sync artifacts"
```

### Task 7: Update Current Documentation And Repository Metadata

**Files:**

- Modify: `README.md`
- Modify: `CONTEXT.md`
- Modify: `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`
- Modify: `docs/superpowers/specs/2026-06-28-initial-adoption-module-design.md`
- Modify: `docs/superpowers/specs/2026-06-28-target-repository-apply-module-design.md`
- Modify: `docs/superpowers/specs/2026-07-03-init-force-adoption-design.md`
- Create: `.driftline/sync.toml`
- Delete: `.driftline-target.toml`

- [ ] **Step 1: Rewrite README around the final artifacts**

Use headings `## Contract` and `## Sync Manifest`. State:

```markdown
The Source Repository owns `.driftline/contract.toml`. The Contract declares stable file identities, source paths, and `managed` or `template` mode.

`driftline init owner/repo` creates `.driftline/sync.toml` in the Target Repository. The Sync manifest records the provider repository/ref and local paths for currently Managed files only.

A repository may contain both files. In that case it provides one Contract outward and maintains an independent inbound Sync relationship.

The complete `.driftline/` subtree is reserved for driftline metadata and cannot be used as a Managed or Template source or target path.
```

Retain the current TOML schema examples, changing only their paths and artifact terminology.

- [ ] **Step 2: Update the domain context without renaming endpoint roles**

Replace the artifact definitions in `CONTEXT.md` with:

```markdown
**Contract**:
The Source Repository's ref-scoped declaration of file groups, stable file identifiers, source paths, and file modes.
_Avoid_: Compatibility contract, package manifest, export receipt.

**Sync manifest**:
The Target Repository's human-editable, driftline-updatable record of one Source Repository/ref and target paths for currently Managed files.
_Avoid_: Lock file, state file, import receipt, bidirectional sync configuration.
```

Update the example dialogue to use Contract and Sync manifest. Keep Source Repository, Target Repository, Managed file, Template file, File key, and Sync plan as endpoint/behavior terms.

- [ ] **Step 3: Reconcile active design documents**

In the active body of the Managed/Template spec and the three supporting module specs:

```text
.driftline-source.toml -> .driftline/contract.toml
.driftline-target.toml -> .driftline/sync.toml
Source Config artifact -> Contract
Target manifest artifact -> Sync manifest
```

Preserve historical migration passages that explicitly say the old names are replaced. Do not edit dated implementation plans.

- [ ] **Step 4: Move this repository's Sync manifest destructively**

Create `.driftline/sync.toml` with the exact current contents of `.driftline-target.toml`, then delete `.driftline-target.toml`. The resulting file begins:

```toml
version = 2

[source]
repository = "y-writings/templates"
ref = "main"
```

Do not leave an alias, symlink, or duplicate root file.

- [ ] **Step 5: Audit remaining old-path occurrences**

Run:

```sh
git grep -nE '\.driftline-(source|target)\.toml'
```

Classify every result. Allowed results are only:

- migration/no-compatibility text in the canonical metadata-layout spec,
- explicit negative tests proving no fallback or ordinary-path behavior,
- dated historical implementation plans.

Update any current user-facing or active agent-facing result outside that allowlist.

- [ ] **Step 6: Verify documentation consistency and tests**

Run:

```sh
git diff --check
go test ./...
```

Expected: no whitespace errors and all tests pass after moving repository metadata.

- [ ] **Step 7: Commit current docs and metadata**

```sh
git add README.md CONTEXT.md AGENTS.md .driftline/sync.toml .driftline-target.toml docs/superpowers/specs docs/superpowers/plans/2026-07-18-driftline-metadata-layout.md
git commit -m "docs: adopt driftline metadata layout"
```

### Task 8: Final Configuration-Drift Audit And Verification

**Files:**

- Verify all files changed by Tasks 1-7

- [ ] **Step 1: Scan implementation identifiers and output vocabulary**

Run:

```sh
git grep -nE 'SourceManifest|TargetConfig|SourceManifestPath|TargetConfigPath|StatusTargetConfig|target-config-(add|remove)' -- '*.go'
```

Expected: no matches.

- [ ] **Step 2: Scan removed behavior and old paths**

Run:

```sh
git grep -nE 'driftline-lock\.yaml|path_overrides|if_not_exists|\.driftline-(source|target)\.toml'
```

Expected: every match is intentional historical text, an explicit removal statement, or a negative test. There are no runtime branches, fallback readers, writers, aliases, warnings, or current examples.

- [ ] **Step 3: Inspect the final metadata layout**

Run:

```sh
git status --short
git diff --stat
```

Expected: `.driftline/sync.toml` is present, `.driftline-target.toml` is deleted, and only intended implementation, tests, docs, and design/plan files changed.

- [ ] **Step 4: Run formatting and static verification**

Run:

```sh
gofmt -w src/internal/driftline src/internal/driftline/commands
git diff --check
go vet ./...
go build ./src/cmd/driftline
```

Expected: every command exits 0 with no diagnostics.

- [ ] **Step 5: Run the complete test suite freshly**

Run:

```sh
go test ./...
```

Expected:

```text
?    github.com/y-writings/driftline/src/cmd/driftline [no test files]
ok   github.com/y-writings/driftline/src/internal/driftline
ok   github.com/y-writings/driftline/src/internal/driftline/commands
```

- [ ] **Step 6: Record any audit-only corrections in the commit that owns the affected responsibility**

If Steps 1-5 require corrections, add them to the pending commit for the task that owns that responsibility. If all task commits already exist, create a non-empty `chore: complete metadata layout migration` commit containing only the explicitly inspected correction files. If no corrections are required, do not create an empty commit.
