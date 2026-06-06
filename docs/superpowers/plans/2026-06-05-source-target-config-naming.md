# Source Target Config Naming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename driftline's source manifest, target config, and path fields so Source Repository and Target Repository configuration are visibly distinct.

**Architecture:** This is a breaking config-format rename with no compatibility shims. Tests first describe the new file names and YAML keys, then constants, schema, parser allowed keys, Go struct fields, plan resolution, commands, and docs are updated to the new names. The lock file keeps its file name but changes its path field to `target_path`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, JSON Schema draft 2020-12, existing Go test suite.

---

## File Structure

- Modify `schema.json`: canonical Source Manifest schema for `.driftline-source.yaml`; require `source_path` instead of `source`.
- Modify `README.md`: user-facing examples and prose for `.driftline-source.yaml`, `.driftline-target.yaml`, `source_path`, and `target_path`.
- Modify `src/internal/driftline/types.go`: rename YAML-backed struct fields for source, target, and lock paths.
- Modify `src/internal/driftline/config.go`: add separate source/target config path constants, update strict allowed keys, validation, generated Target Config, and lock validation.
- Modify `src/internal/driftline/plan.go`: read the new file names, resolve source and target paths through renamed fields, and write lock items with `target_path`.
- Modify `src/internal/driftline/schema_test.go`: assert schema and parser agree on `source_path`.
- Modify `src/internal/driftline/config_test.go`: parser and validation tests for new keys and old-key rejection.
- Modify `src/internal/driftline/plan_test.go`: plan resolution, duplicate/reserved target checks, stale lock behavior, and lock identity tests with new keys.
- Modify `src/internal/driftline/commands/init.go`: read `.driftline-source.yaml` and write `.driftline-target.yaml`.
- Modify `src/internal/driftline/commands/run.go`: help text for the new Target Config file name.
- Modify `src/internal/driftline/commands/commands_test.go`: command integration tests for new file names and lock `target_path`.

Do not commit during execution unless the user explicitly asks for a commit. Use diff checkpoints instead.

### Task 1: Parser And Schema Tests Describe New Keys

**Files:**
- Modify: `src/internal/driftline/config_test.go`
- Modify: `src/internal/driftline/schema_test.go`

- [ ] **Step 1: Update Source Manifest parser tests**

In `src/internal/driftline/config_test.go`, replace `TestLoadSourceManifestStrictValidation` with this function:

```go
func TestLoadSourceManifestStrictValidation(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 1\ngitignore:\n  - ' .cache/tool '\n  - ''\nfiles:\n  - id: example\n    source_path: templates/example.txt\n  - id: local-config\n    source_path: templates/config.local\n    if_not_exists: true\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Files) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if manifest.Files[0].SourcePath != "templates/example.txt" {
		t.Fatalf("unexpected source path: %#v", manifest.Files[0])
	}
	if !manifest.Files[1].IfNotExists {
		t.Fatalf("expected if_not_exists true")
	}
	if len(manifest.GitIgnore) != 2 {
		t.Fatalf("gitignore entries should be preserved before write-time trimming: %#v", manifest.GitIgnore)
	}
}
```

- [ ] **Step 2: Update Source Manifest old-key rejection tests**

In `TestLoadSourceManifestRejectsUnknownAndDuplicateKeys`, replace the `input` map with this map:

```go
map[string]string{
	"unknown root":   "version: 1\nextra: true\nfiles: []\n",
	"duplicate root": "version: 1\nversion: 1\nfiles: []\n",
	"unknown file":   "version: 1\nfiles:\n  - id: sample\n    source_path: sample.txt\n    extra: true\n",
	"old source key": "version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n",
	"target file":    "version: 1\nfiles:\n  - id: sample\n    source_path: sample.txt\n    target: sample.txt\n",
}
```

- [ ] **Step 3: Update Target Config parser test**

In `TestLoadTargetConfigDistinguishesOmittedAndExplicitFalse`, replace the YAML input and target assertion with this code:

```go
config, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: inherited\n  - id: explicit\n    target_path: custom.txt\n    if_not_exists: false\n"))
if err != nil {
	t.Fatalf("load target config failed: %v", err)
}
if config.Files[0].IfNotExists != nil {
	t.Fatalf("expected omitted if_not_exists to stay nil")
}
if config.Files[1].TargetPath != "custom.txt" {
	t.Fatalf("expected target_path to decode, got %#v", config.Files[1])
}
if config.Files[1].IfNotExists == nil || *config.Files[1].IfNotExists {
	t.Fatalf("expected explicit false override, got %#v", config.Files[1].IfNotExists)
}
```

- [ ] **Step 4: Add Target Config old-key rejection test**

Add this test after `TestLoadTargetConfigDistinguishesOmittedAndExplicitFalse`:

```go
func TestLoadTargetConfigRejectsOldTargetKey(t *testing.T) {
	_, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: explicit\n    target: custom.txt\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target to be rejected as an unknown key, got %v", err)
	}
}
```

- [ ] **Step 5: Update generated Target Config duplicate test**

In `TestTargetConfigFromSourceManifestRejectsDuplicateDefaultTargets`, replace the manifest YAML with:

```go
manifest, err := LoadSourceManifestBytes([]byte("version: 1\nfiles:\n  - id: first\n    source_path: same.txt\n  - id: second\n    source_path: ./same.txt\n"))
```

- [ ] **Step 6: Update lock parser tests**

In `TestLoadLockFileRejectsDuplicateTarget`, replace the YAML input with:

```go
_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: first\n    target_path: same.txt\n  - id: second\n    target_path: same.txt\n"))
```

In `TestLoadLockFileRejectsHashFields`, replace both `target: sample.txt` occurrences with `target_path: sample.txt`.

- [ ] **Step 7: Add lock old-key rejection test**

Add this test after `TestLoadLockFileRejectsDuplicateTarget`:

```go
func TestLoadLockFileRejectsOldTargetKey(t *testing.T) {
	_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target: sample.txt\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target to be rejected as an unknown key, got %v", err)
	}
}
```

- [ ] **Step 8: Update schema parser consistency test**

In `src/internal/driftline/schema_test.go`, replace the required-key assertion and old-field guard with this code:

```go
assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}, "source_path": {}})
if _, ok := fileProperties["source"]; ok {
	t.Fatal("source manifest schema must not allow old file source key")
}
if _, ok := fileProperties["target"]; ok {
	t.Fatal("source manifest schema must not allow file target")
}
```

- [ ] **Step 9: Run parser/schema tests to verify RED**

Run: `go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestTargetConfigFromSourceManifest|TestLoadLock|TestSourceManifestSchema'`

Expected: FAIL. At least one failure should mention missing fields such as `SourcePath` or unknown key `source_path` before implementation.

### Task 2: Implement Parser, Types, Constants, And Schema

**Files:**
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/config.go`
- Modify: `schema.json`

- [ ] **Step 1: Rename YAML-backed struct fields**

In `src/internal/driftline/types.go`, replace the path-bearing structs with this code:

```go
type SourceManifestFile struct {
	ID          string `yaml:"id"`
	SourcePath  string `yaml:"source_path"`
	IfNotExists bool   `yaml:"if_not_exists,omitempty"`
}

type TargetConfigFile struct {
	ID          string `yaml:"id"`
	TargetPath  string `yaml:"target_path,omitempty"`
	IfNotExists *bool  `yaml:"if_not_exists,omitempty"`
}

type LockItem struct {
	ID         string `yaml:"id"`
	TargetPath string `yaml:"target_path"`
}
```

- [ ] **Step 2: Split source and target config file constants**

In `src/internal/driftline/config.go`, replace the constants block with:

```go
const (
	SourceManifestPath = ".driftline-source.yaml"
	TargetConfigPath   = ".driftline-target.yaml"
	LockFilePath       = "driftline-lock.yaml"
)
```

- [ ] **Step 3: Update generated Target Config logic**

In `TargetConfigFromSourceManifest`, replace `item.Source` with `item.SourcePath`:

```go
defaultTarget := normalizedTargetPath(item.SourcePath)
```

Keep the generated file body as `TargetConfigFile{ID: item.ID}` so generated Target Config entries omit `target_path` unless the user edits them.

- [ ] **Step 4: Update strict allowed keys**

In `allowedSourceManifestKeys`, replace the file key set with:

```go
"files": set("id", "source_path", "if_not_exists"),
```

In `allowedTargetConfigKeys`, replace the file key set with:

```go
"files": set("id", "target_path", "if_not_exists"),
```

In `allowedLockKeys`, replace the file key set with:

```go
"files": set("id", "target_path"),
```

- [ ] **Step 5: Update validation field references**

In `validateSourceManifest`, replace the source path validation line with:

```go
if err := ValidateConfigPath(item.SourcePath, fmt.Sprintf("source %q", item.ID)); err != nil {
	return err
}
```

In `validateTargetConfig`, replace the target path validation block with:

```go
if item.TargetPath != "" {
	if err := ValidateConfigPath(item.TargetPath, fmt.Sprintf("target %q", item.ID)); err != nil {
		return err
	}
}
```

In `validateLock`, replace target validation and duplicate tracking with:

```go
if err := ValidateConfigPath(item.TargetPath, fmt.Sprintf("target %q", item.ID)); err != nil {
	return err
}
identity := item.ID + "\x00" + item.TargetPath
if _, ok := seenIdentity[identity]; ok {
	return fmt.Errorf("duplicate lock item %q target %q", item.ID, item.TargetPath)
}
seenIdentity[identity] = struct{}{}
if _, ok := seenTarget[item.TargetPath]; ok {
	return fmt.Errorf("duplicate target %q", item.TargetPath)
}
seenTarget[item.TargetPath] = struct{}{}
```

- [ ] **Step 6: Update `schema.json` metadata and properties**

In `schema.json`, set the title and description to:

```json
"title": "driftline Source Manifest",
"description": "Schema for .driftline-source.yaml in a Source Repository. Target paths are configured by Target Repositories, not in this manifest.",
```

In `$defs.file`, replace the required list and `source` property with:

```json
"required": ["id", "source_path"],
"properties": {
  "id": {
    "description": "Stable file identifier used by Target Config entries.",
    "type": "string",
    "minLength": 1,
    "pattern": "\\S"
  },
  "source_path": {
    "description": "Relative path to the file in the Source Repository.",
    "$ref": "#/$defs/relativePath"
  },
  "if_not_exists": {
    "description": "When true, driftline does not overwrite an existing target file.",
    "type": "boolean"
  }
}
```

- [ ] **Step 7: Run parser/schema tests to verify GREEN**

Run: `go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestTargetConfigFromSourceManifest|TestLoadLock|TestSourceManifestSchema'`

Expected: PASS.

### Task 3: Plan Tests Describe New Paths And Lock Items

**Files:**
- Modify: `src/internal/driftline/plan_test.go`

- [ ] **Step 1: Replace target config fixture file names**

In `src/internal/driftline/plan_test.go`, replace every `writePlanFile(t, targetDir, "driftline.yaml", ...` call with `writePlanFile(t, targetDir, ".driftline-target.yaml", ...`.

- [ ] **Step 2: Replace source manifest lookup keys**

In `src/internal/driftline/plan_test.go`, replace every fake source map key suffix `:driftline.yaml` with `:.driftline-source.yaml`.

Example before:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n"),
```

Example after:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source_path: sample.txt\n"),
```

- [ ] **Step 3: Replace YAML path field names in plan fixtures**

In `src/internal/driftline/plan_test.go`, replace path fields by role:

```text
Source Manifest fixture key: source:      -> source_path:
Target Config fixture key:   target:      -> target_path:
Lock fixture key:            target:      -> target_path:
```

Apply this to every inline YAML string in `plan_test.go`. Root `source:` in Target Config must stay `source:` because it contains `repository` and `ref`.

- [ ] **Step 4: Update lock item assertions**

Replace `item.Target` assertions with `item.TargetPath` assertions. For example, in `TestBuildPlanDetectsMissingLockAndAdd`, use:

```go
if item := plan.NextLockItem("sample", "sample.txt"); item.TargetPath != "sample.txt" {
	t.Fatalf("expected omitted target config to default to source path, got %#v", item)
}
```

Apply the same change to the `local-config` and `foo.txt` lock item assertions.

- [ ] **Step 5: Update stale lock entry checks**

In `TestBuildPlanDropsStaleLockEntryWhenTargetIsActiveAgain`, keep checking the stale ID but read the renamed field when inspecting target paths. The loop should remain:

```go
for _, item := range plan.NextLock.Files {
	if item.ID == "old-id" {
		t.Fatalf("old lock entry for active target should be dropped: %#v", plan.NextLock.Files)
	}
}
```

This test does not need a direct target path assertion because the ID is the behavior under test.

- [ ] **Step 6: Run plan tests to verify RED**

Run: `go test ./src/internal/driftline -run 'TestBuildPlan'`

Expected: FAIL. Failures should mention missing `.driftline-target.yaml`, missing `.driftline-source.yaml`, or old struct fields before implementation.

### Task 4: Implement Plan Resolution And Lock Rename

**Files:**
- Modify: `src/internal/driftline/plan.go`
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/config.go`

- [ ] **Step 1: Make BuildPlan read the new file names**

In `BuildPlan`, keep the Target Config path as `TargetConfigPath` and replace the source manifest read path with `SourceManifestPath`:

```go
configPath := filepath.Join(opts.TargetDir, TargetConfigPath)
lockPath := filepath.Join(opts.TargetDir, LockFilePath)
config, err := LoadTargetConfig(configPath)
if err != nil {
	return Plan{}, err
}
commit, err := opts.Source.ResolveRef(config.Source.Repository, config.Source.Ref)
if err != nil {
	return Plan{}, err
}
manifestBytes, err := opts.Source.ReadFile(config.Source.Repository, commit, SourceManifestPath)
if err != nil {
	return Plan{}, fmt.Errorf(".driftline-source.yaml not found in source repository: %w", err)
}
```

- [ ] **Step 2: Update lock identity setup**

In `planBuilder.build`, replace lock target field references with `TargetPath`:

```go
for _, item := range b.lock.Files {
	lockByIdentity[lockIdentity(item.ID, normalizedTargetPath(item.TargetPath))] = item
}
```

The `lockByIdentity` map is currently populated but not read. Leave it in place for this task to keep the rename focused.

- [ ] **Step 3: Update resolved target defaulting**

Replace `resolveTargetConfigFile` with this function:

```go
func resolveTargetConfigFile(configured TargetConfigFile, manifestItem SourceManifestFile) resolvedFile {
	target := configured.TargetPath
	if target == "" {
		target = manifestItem.SourcePath
	}
	ifNotExists := manifestItem.IfNotExists
	if configured.IfNotExists != nil {
		ifNotExists = *configured.IfNotExists
	}
	target = normalizedTargetPath(target)
	return resolvedFile{id: configured.ID, source: manifestItem.SourcePath, target: target, ifNotExists: ifNotExists}
}
```

- [ ] **Step 4: Update active target comparison for stale locks**

Replace the stale lock loop's active target check with:

```go
for _, item := range b.lock.Files {
	if _, ok := activeTargets[normalizedTargetPath(item.TargetPath)]; ok {
		continue
	}
	plan.NextLock.Files = append(plan.NextLock.Files, item)
	change, err := b.staleChange(item)
	if err != nil {
		return Plan{}, err
	}
	plan.Changes = append(plan.Changes, change)
}
```

- [ ] **Step 5: Update lock item construction**

Replace `nextActiveLockItem` with:

```go
func nextActiveLockItem(file resolvedFile) LockItem {
	return LockItem{
		ID:         file.id,
		TargetPath: file.target,
	}
}
```

- [ ] **Step 6: Update stale change construction**

Replace `staleChange` with:

```go
func (b planBuilder) staleChange(item LockItem) (Change, error) {
	targetPath, err := PathWithin(b.opts.TargetDir, item.TargetPath, fmt.Sprintf("locked target %q", item.ID))
	if err != nil {
		return Change{}, err
	}
	change := Change{
		ID:         item.ID,
		Target:     item.TargetPath,
		TargetPath: targetPath,
		Status:     StatusPrune,
		Reason:     "target is no longer adopted",
	}
	return change, nil
}
```

- [ ] **Step 7: Update `Plan.NextLockItem`**

In `src/internal/driftline/plan.go`, replace `Plan.NextLockItem` with:

```go
func (p Plan) NextLockItem(id string, target string) LockItem {
	for _, item := range p.NextLock.Files {
		if item.ID == id && item.TargetPath == target {
			return item
		}
	}
	return LockItem{}
}
```

- [ ] **Step 8: Update prune lock removal**

In `src/internal/driftline/commands/prune.go`, replace `removeLockItem` with:

```go
func removeLockItem(items []driftline.LockItem, id string, target string) []driftline.LockItem {
	out := items[:0]
	for _, item := range items {
		if item.ID == id && item.TargetPath == target {
			continue
		}
		out = append(out, item)
	}
	return out
}
```

- [ ] **Step 9: Run plan tests to verify GREEN**

Run: `go test ./src/internal/driftline -run 'TestBuildPlan'`

Expected: PASS.

### Task 5: Command Tests Describe New File Names

**Files:**
- Modify: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Update init test fixture and output path**

In `TestInitCreatesTargetConfigFromSourceManifest`, change the fake source manifest key and YAML to:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: example\n    source_path: templates/example.txt\n  - id: local-config\n    source_path: templates/config.local\n    if_not_exists: true\n"),
```

Change the generated config read path to:

```go
got := readFile(t, targetDir, ".driftline-target.yaml")
```

Change the old target guard to:

```go
if strings.Contains(got, "target_path:") {
	t.Fatalf("target config must not copy source manifest paths as targets:\n%s", got)
}
```

- [ ] **Step 2: Update init ref test source manifest and output path**

In `TestInitRefPreservesInputRef`, replace the fake source file map with:

```go
files: map[string][]byte{"y-writings/source-repo@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:.driftline-source.yaml": []byte("version: 1\nfiles: []\n")},
```

Change the generated config read path to:

```go
if got := readFile(t, targetDir, ".driftline-target.yaml"); !strings.Contains(got, "ref: feature/foo") {
	t.Fatalf("expected input ref to be preserved:\n%s", got)
}
```

- [ ] **Step 3: Update existing config refusal test**

In `TestInitRefusesExistingConfigOrLock`, replace the file map with:

```go
for name, file := range map[string]string{"config": ".driftline-target.yaml", "lock": "driftline-lock.yaml"} {
```

- [ ] **Step 4: Update command fixtures for target config file name**

In `commands_test.go`, replace every Target Repository config fixture write from:

```go
writeFile(t, targetDir, "driftline.yaml", ...)
```

to:

```go
writeFile(t, targetDir, ".driftline-target.yaml", ...)
```

- [ ] **Step 5: Update command fake source manifest lookups and source path keys**

In `commands_test.go`, replace every fake source map key suffix `:driftline.yaml` with `:.driftline-source.yaml`.

In each fake source manifest YAML string, replace file-entry `source:` keys with `source_path:`. Keep root `source:` keys in Target Config YAML unchanged.

- [ ] **Step 6: Update command lock fixture and assertion keys**

In `commands_test.go`, replace every lock YAML `target:` path key with `target_path:`.

Change lock content expectations from `"target: sample.txt"` to `"target_path: sample.txt"`, and from `"target: old.txt"` to `"target_path: old.txt"`.

- [ ] **Step 7: Run command tests to verify RED**

Run: `go test ./src/internal/driftline/commands`

Expected: FAIL. Failures should mention missing `.driftline-source.yaml`, missing `.driftline-target.yaml`, or outdated help text before command implementation.

### Task 6: Implement Command File Name Rename

**Files:**
- Modify: `src/internal/driftline/commands/init.go`
- Modify: `src/internal/driftline/commands/run.go`
- Modify: `src/internal/driftline/config.go`

- [ ] **Step 1: Update init existing-file checks and source manifest read**

In `runInit`, use the existing `TargetConfigPath`, `LockFilePath`, and new `SourceManifestPath` constants. Replace the manifest read block with:

```go
manifestBytes, err := source.ReadFile(opts.Repository, commit, driftline.SourceManifestPath)
if err != nil {
	return fmt.Errorf(".driftline-source.yaml not found in source repository: %w", err)
}
manifest, err := driftline.LoadSourceManifestBytes(manifestBytes)
if err != nil {
	return err
}
```

Keep `configPath := filepath.Join(opts.TargetDir, driftline.TargetConfigPath)` so `init` writes `.driftline-target.yaml`.

- [ ] **Step 2: Update init success message**

Replace the final `fmt.Fprintf` call in `runInit` with:

```go
fmt.Fprintf(stdout, "created .driftline-target.yaml from %s@%s\n", opts.Repository, commit)
```

- [ ] **Step 3: Update help text**

In `printUsage`, replace the raw string with:

```go
fmt.Fprintln(w, `usage: driftline <command> [options]

commands:
  init owner/repo  create .driftline-target.yaml from a GitHub Source Repository
  check            check whether target files match the Source Repository
  diff             show diffs for files that would be added or updated
  update           copy added/updated files and refresh driftline-lock.yaml
  prune            remove stale files when they are unchanged locally

examples:
  driftline init owner/repo
  driftline init owner/repo --ref main --target-dir .
  driftline check --target-dir .

options:
  --target-dir string  target repository directory (default ".")
  --ref string         init-only ref to preserve in .driftline-target.yaml

authentication:
  set GITHUB_TOKEN for private repositories or higher rate limits`)
```

- [ ] **Step 4: Run command tests to verify GREEN**

Run: `go test ./src/internal/driftline/commands`

Expected: PASS.

### Task 7: Documentation Describes New Config Names

**Files:**
- Modify: `README.md`
- Modify: `schema.json`

- [ ] **Step 1: Update README Source Manifest section**

In `README.md`, replace the Source Manifest prose and example with:

````md
## Source Manifest

The Source Repository owns `.driftline-source.yaml` at its repository root.
Editors that support JSON Schema can validate it with the canonical schema.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/y-writings/driftline/main/schema.json
version: 1
gitignore:
  - .cache/tool
files:
  - id: example
    source_path: templates/example.txt
  - id: local-config
    source_path: templates/config.local
    if_not_exists: true
```

Source Manifest file entries do not define `target_path`; target paths belong to the Target Config.
````

- [ ] **Step 2: Update README Target Config section**

In `README.md`, replace the Target Config example and target override prose with:

````md
This creates `.driftline-target.yaml` in the Target Repository.

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: example
  - id: local-config
    if_not_exists: true
```

When a Target Config file entry omits `target_path`, driftline writes to the same relative path as the Source Manifest `source_path`. Add `target_path` only when the Target Repository wants a different destination path:

```yaml
files:
  - id: example
    target_path: example.txt
```
````

- [ ] **Step 3: Update README Lock File section**

In `README.md`, replace the Lock File example file entry with:

```yaml
files:
  - id: example
    target_path: example.txt
```

- [ ] **Step 4: Run docs-sensitive tests**

Run: `go test ./src/internal/driftline/...`

Expected: PASS.

### Task 8: Stale Reference Sweep And Full Verification

**Files:**
- Inspect: `README.md`
- Inspect: `schema.json`
- Inspect: `src/internal/driftline/**/*.go`
- Inspect: `docs/superpowers/specs/2026-06-05-source-target-config-naming-design.md`
- Inspect: `docs/superpowers/plans/2026-06-05-source-target-config-naming.md`

- [ ] **Step 1: Search for old config file name references**

Run: `rg 'driftline\.yaml' README.md schema.json src/internal/driftline docs/superpowers/specs/2026-06-05-source-target-config-naming-design.md docs/superpowers/plans/2026-06-05-source-target-config-naming.md`

Expected: matches only in historical context inside docs or none. Current README, schema, source, and tests must not instruct users or code to read `driftline.yaml`.

- [ ] **Step 2: Search for old path field keys in current examples and tests**

Run: `rg '(^|\\s)(source|target):' README.md schema.json src/internal/driftline docs/superpowers/specs/2026-06-05-source-target-config-naming-design.md docs/superpowers/plans/2026-06-05-source-target-config-naming.md`

Expected: allowed matches are Target Config root `source:` blocks and historical/spec text that explicitly discusses old keys. File path fields in current fixtures and examples must use `source_path:` or `target_path:`.

- [ ] **Step 3: Run package tests**

Run: `go test ./src/internal/driftline/...`

Expected: PASS.

- [ ] **Step 4: Run full tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Review final diff**

Run: `git diff -- README.md schema.json src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/plan.go src/internal/driftline/config_test.go src/internal/driftline/schema_test.go src/internal/driftline/plan_test.go src/internal/driftline/commands/init.go src/internal/driftline/commands/run.go src/internal/driftline/commands/prune.go src/internal/driftline/commands/commands_test.go docs/superpowers/specs/2026-06-05-source-target-config-naming-design.md docs/superpowers/plans/2026-06-05-source-target-config-naming.md`

Expected: diff shows the approved rename, tests, docs, schema, and this plan only. No compatibility lookup for `driftline.yaml`, and no parser support for old path keys `source` or `target`.
