# File Set Path Overrides Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace one-file-per-id adoption with file-set adoption using Source Manifest `paths` and Target Config `path_overrides`.

**Architecture:** Treat Source Manifest `files[].id` as an adoption unit that expands into concrete resolved files during planning. Keep source parsing, target selection, and lock writing in the existing Go package, but replace `source_path` and target-config `target_path` with the new YAML contract. Lock entries remain file-oriented as `id` plus `target_path`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, JSON Schema draft 2020-12, existing Go test suite.

**Execution Note:** Do not commit while executing this plan unless the user explicitly asks for commits. Checkpoint steps use `git diff` and `git status` instead.

---

## File Structure

- Modify `schema.json`: update the Source Manifest schema from `source_path` to non-empty `paths` arrays.
- Modify `README.md`: document adoption units, `paths`, and `path_overrides`.
- Modify `src/internal/driftline/types.go`: replace file path fields with `Paths []string`, `PathOverrides []PathOverride`, and `PathOverride`.
- Modify `src/internal/driftline/config.go`: update strict key allow-lists, validation, generated Target Config, and path normalization naming.
- Modify `src/internal/driftline/plan.go`: expand each Target Config `id` into one resolved file per source path and apply per-path overrides.
- Modify `src/internal/driftline/schema_test.go`: assert schema and parser agree on `paths`.
- Modify `src/internal/driftline/config_test.go`: cover new parser/validation contract and old-key rejection.
- Modify `src/internal/driftline/plan_test.go`: cover multi-file expansion, overrides, duplicate targets, unknown override paths, stale locks, and `if_not_exists` behavior.
- Modify `src/internal/driftline/commands/commands_test.go`: update command fixtures and cover update/lock behavior for a file set.
- Modify `src/internal/driftline/commands/changes.go`: make command output ordering deterministic when changes share the same `id`.

### Task 1: Tests Describe Config And Schema Contract

**Files:**
- Modify: `src/internal/driftline/config_test.go`
- Modify: `src/internal/driftline/schema_test.go`

- [ ] **Step 1: Update the successful Source Manifest decode test**

Replace `TestLoadSourceManifestStrictValidation` in `src/internal/driftline/config_test.go` with this version:

```go
func TestLoadSourceManifestStrictValidation(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 1\ngitignore:\n  - ' .cache/tool '\n  - ''\nfiles:\n  - id: example\n    paths:\n      - templates/example.txt\n      - templates/example-extra.txt\n  - id: local-config\n    paths:\n      - templates/config.local\n    if_not_exists: true\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Files) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if len(manifest.Files[0].Paths) != 2 || manifest.Files[0].Paths[0] != "templates/example.txt" || manifest.Files[0].Paths[1] != "templates/example-extra.txt" {
		t.Fatalf("unexpected source paths: %#v", manifest.Files[0])
	}
	if !manifest.Files[1].IfNotExists {
		t.Fatalf("expected if_not_exists true")
	}
	if len(manifest.GitIgnore) != 2 {
		t.Fatalf("gitignore entries should be preserved before write-time trimming: %#v", manifest.GitIgnore)
	}
}
```

- [ ] **Step 2: Update Source Manifest unknown-key rejection**

Replace the map in `TestLoadSourceManifestRejectsUnknownAndDuplicateKeys` with this map:

```go
for name, input := range map[string]string{
	"unknown root":    "version: 1\nextra: true\nfiles: []\n",
	"duplicate root":  "version: 1\nversion: 1\nfiles: []\n",
	"unknown file":    "version: 1\nfiles:\n  - id: sample\n    paths:\n      - sample.txt\n    extra: true\n",
	"old source path": "version: 1\nfiles:\n  - id: sample\n    source_path: sample.txt\n",
	"target file":     "version: 1\nfiles:\n  - id: sample\n    paths:\n      - sample.txt\n    target: sample.txt\n",
}
```

- [ ] **Step 3: Add Source Manifest path validation tests**

Add this test after `TestLoadSourceManifestRejectsUnknownAndDuplicateKeys`:

```go
func TestLoadSourceManifestRejectsInvalidPaths(t *testing.T) {
	for name, input := range map[string]string{
		"missing paths":    "version: 1\nfiles:\n  - id: sample\n",
		"empty paths":      "version: 1\nfiles:\n  - id: sample\n    paths: []\n",
		"invalid path":     "version: 1\nfiles:\n  - id: sample\n    paths:\n      - ../sample.txt\n",
		"duplicate path":   "version: 1\nfiles:\n  - id: sample\n    paths:\n      - same.txt\n      - ./same.txt\n",
		"duplicate file id": "version: 1\nfiles:\n  - id: sample\n    paths:\n      - one.txt\n  - id: sample\n    paths:\n      - two.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSourceManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
```

- [ ] **Step 4: Replace the Target Config path decode test**

Replace `TestLoadTargetConfigDistinguishesOmittedAndExplicitFalse` with this version:

```go
func TestLoadTargetConfigDecodesPathOverridesAndExplicitFalse(t *testing.T) {
	config, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: inherited\n  - id: explicit\n    path_overrides:\n      - from: source.txt\n        to: custom.txt\n    if_not_exists: false\n"))
	if err != nil {
		t.Fatalf("load target config failed: %v", err)
	}
	if config.Files[0].IfNotExists != nil {
		t.Fatalf("expected omitted if_not_exists to stay nil")
	}
	if len(config.Files[1].PathOverrides) != 1 || config.Files[1].PathOverrides[0].From != "source.txt" || config.Files[1].PathOverrides[0].To != "custom.txt" {
		t.Fatalf("expected path_overrides to decode, got %#v", config.Files[1])
	}
	if config.Files[1].IfNotExists == nil || *config.Files[1].IfNotExists {
		t.Fatalf("expected explicit false override, got %#v", config.Files[1].IfNotExists)
	}
}
```

- [ ] **Step 5: Update Target Config old-key rejection**

Replace `TestLoadTargetConfigRejectsOldTargetKey` with this version:

```go
func TestLoadTargetConfigRejectsOldTargetPathKey(t *testing.T) {
	_, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: explicit\n    target_path: custom.txt\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target_path to be rejected as an unknown key, got %v", err)
	}
}
```

- [ ] **Step 6: Add Target Config path override validation tests**

Add this test after `TestLoadTargetConfigRejectsOldTargetPathKey`:

```go
func TestLoadTargetConfigRejectsInvalidPathOverrides(t *testing.T) {
	for name, input := range map[string]string{
		"empty overrides":   "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides: []\n",
		"missing from":      "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - to: custom.txt\n",
		"missing to":        "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: source.txt\n",
		"invalid from":      "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: ../source.txt\n        to: custom.txt\n",
		"invalid to":        "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: source.txt\n        to: ../custom.txt\n",
		"duplicate from":    "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: source.txt\n        to: one.txt\n      - from: ./source.txt\n        to: two.txt\n",
		"duplicate file id": "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n  - id: sample\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadTargetConfigBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
```

- [ ] **Step 7: Update duplicate default target test input**

Replace the manifest string in `TestTargetConfigFromSourceManifestRejectsDuplicateDefaultTargets` with this shape:

```go
manifest, err := LoadSourceManifestBytes([]byte("version: 1\nfiles:\n  - id: first\n    paths:\n      - same.txt\n  - id: second\n    paths:\n      - ./same.txt\n"))
```

- [ ] **Step 8: Update schema test assertions**

In `TestSourceManifestSchemaMatchesParserAllowedKeys`, replace the required assertion and old-key assertion block with this:

```go
assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}, "paths": {}})
if _, ok := fileProperties["source_path"]; ok {
	t.Fatal("source manifest schema must not allow old file source_path key")
}
if _, ok := fileProperties["target"]; ok {
	t.Fatal("source manifest schema must not allow file target")
}
pathsSchema := objectValue(fileItemSchema, "properties")["paths"].(map[string]any)
if got := numberValue(pathsSchema, "minItems"); got != 1 {
	t.Fatalf("paths must require at least one item, got %v", got)
}
```

Add this helper near the other schema helpers:

```go
func numberValue(values map[string]any, key string) float64 {
	value, _ := values[key].(float64)
	return value
}
```

- [ ] **Step 9: Run focused tests to verify RED**

Run: `go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestTargetConfigFromSourceManifest|TestSourceManifestSchema'`

Expected: FAIL because the code still has `SourcePath`, `TargetPath`, and a schema requiring `source_path`.

- [ ] **Step 10: Review diff checkpoint**

Run: `git diff -- src/internal/driftline/config_test.go src/internal/driftline/schema_test.go`

Expected: Diff only changes tests for the new config and schema contract.

### Task 2: Implement Config, Types, And Schema

**Files:**
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/config.go`
- Modify: `schema.json`

- [ ] **Step 1: Replace YAML-backed types**

In `src/internal/driftline/types.go`, replace `SourceManifestFile` and `TargetConfigFile` with these definitions and add `PathOverride`:

```go
type SourceManifestFile struct {
	ID          string   `yaml:"id"`
	Paths       []string `yaml:"paths"`
	IfNotExists bool     `yaml:"if_not_exists,omitempty"`
}

type TargetConfigFile struct {
	ID            string         `yaml:"id"`
	PathOverrides []PathOverride `yaml:"path_overrides,omitempty"`
	IfNotExists   *bool          `yaml:"if_not_exists,omitempty"`
}

type PathOverride struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}
```

- [ ] **Step 2: Update Source Manifest schema**

In `schema.json`, replace the file definition with this shape:

```json
"file": {
  "type": "object",
  "additionalProperties": false,
  "required": ["id", "paths"],
  "properties": {
    "id": {
      "description": "Stable adoption unit identifier used by Target Config entries.",
      "type": "string",
      "minLength": 1,
      "pattern": "\\S"
    },
    "paths": {
      "description": "Relative source repository paths included in this adoption unit.",
      "type": "array",
      "minItems": 1,
      "items": {
        "$ref": "#/$defs/relativePath"
      }
    },
    "if_not_exists": {
      "description": "When true, driftline does not overwrite existing target files in this adoption unit.",
      "type": "boolean"
    }
  }
}
```

- [ ] **Step 3: Update strict allowed keys**

Replace `allowedSourceManifestKeys` and `allowedTargetConfigKeys` in `src/internal/driftline/config.go` with:

```go
func allowedSourceManifestKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":      set("version", "gitignore", "files"),
		"files": set("id", "paths", "if_not_exists"),
	}
}

func allowedTargetConfigKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":               set("version", "source", "files"),
		"source":         set("repository", "ref"),
		"files":          set("id", "path_overrides", "if_not_exists"),
		"path_overrides": set("from", "to"),
	}
}
```

- [ ] **Step 4: Add mapping-node helpers for explicit empty `path_overrides`**

Add these helpers near `configMappingValue`:

```go
func configSequenceItems(root *yaml.Node, key string) []*yaml.Node {
	node := configMappingValue(root, key)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

func mappingHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Rename normalization helper for shared source and target use**

Rename `normalizedTargetPath` to `normalizedConfigPath` in `src/internal/driftline/config.go` and `src/internal/driftline/plan.go`:

```go
func normalizedConfigPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
```

Update every existing call site from `normalizedTargetPath(...)` to `normalizedConfigPath(...)`.

- [ ] **Step 6: Update Source Manifest validation**

Replace `validateSourceManifest` with:

```go
func validateSourceManifest(manifest SourceManifest, root *yaml.Node) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported source manifest version %d", manifest.Version)
	}
	if err := requireSequence(root, "files", "source manifest"); err != nil {
		return err
	}
	seenIDs := map[string]struct{}{}
	for _, item := range manifest.Files {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("source manifest contains file without id")
		}
		if _, ok := seenIDs[item.ID]; ok {
			return fmt.Errorf("duplicate source manifest file id %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		if len(item.Paths) == 0 {
			return fmt.Errorf("source manifest file %q must define paths", item.ID)
		}
		seenPaths := map[string]struct{}{}
		for _, sourcePath := range item.Paths {
			if err := ValidateConfigPath(sourcePath, fmt.Sprintf("source %q", item.ID)); err != nil {
				return err
			}
			normalized := normalizedConfigPath(sourcePath)
			if _, ok := seenPaths[normalized]; ok {
				return fmt.Errorf("duplicate source path %q in source manifest file id %q", normalized, item.ID)
			}
			seenPaths[normalized] = struct{}{}
		}
	}
	return nil
}
```

- [ ] **Step 7: Update Target Config validation**

Replace the `for _, item := range config.Files` loop in `validateTargetConfig` with this indexed loop:

```go
	seen := map[string]struct{}{}
	fileNodes := configSequenceItems(root, "files")
	for i, item := range config.Files {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("target config contains file without id")
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate target config file id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if i < len(fileNodes) && mappingHasKey(fileNodes[i], "path_overrides") && len(item.PathOverrides) == 0 {
			return fmt.Errorf("target config file %q path_overrides must not be empty", item.ID)
		}
		seenFrom := map[string]struct{}{}
		for _, override := range item.PathOverrides {
			if err := ValidateConfigPath(override.From, fmt.Sprintf("target %q override from", item.ID)); err != nil {
				return err
			}
			if err := ValidateConfigPath(override.To, fmt.Sprintf("target %q override to", item.ID)); err != nil {
				return err
			}
			from := normalizedConfigPath(override.From)
			if _, ok := seenFrom[from]; ok {
				return fmt.Errorf("duplicate path override from %q in target config file id %q", from, item.ID)
			}
			seenFrom[from] = struct{}{}
		}
	}
```

- [ ] **Step 8: Update Target Config generation from Source Manifest**

Replace the body of the manifest file loop in `TargetConfigFromSourceManifest` with:

```go
	seenDefaultTargets := map[string]struct{}{}
	for _, item := range manifest.Files {
		for _, sourcePath := range item.Paths {
			defaultTarget := normalizedConfigPath(sourcePath)
			if _, ok := seenDefaultTargets[defaultTarget]; ok {
				return TargetConfig{}, fmt.Errorf("duplicate target %q", defaultTarget)
			}
			seenDefaultTargets[defaultTarget] = struct{}{}
		}
		file := TargetConfigFile{ID: item.ID}
		if item.IfNotExists {
			v := true
			file.IfNotExists = &v
		}
		config.Files = append(config.Files, file)
	}
```

- [ ] **Step 9: Run focused tests to verify GREEN**

Run: `go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestTargetConfigFromSourceManifest|TestSourceManifestSchema'`

Expected: PASS.

- [ ] **Step 10: Review diff checkpoint**

Run: `git diff -- schema.json src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/config_test.go src/internal/driftline/schema_test.go`

Expected: Diff shows the new YAML contract, schema, and validation only.

### Task 3: Tests Describe Plan Expansion

**Files:**
- Modify: `src/internal/driftline/plan_test.go`

- [ ] **Step 1: Update existing plan fixtures from `source_path` to `paths`**

In every Source Manifest YAML string in `src/internal/driftline/plan_test.go`, replace this shape:

```yaml
files:
  - id: sample
    source_path: sample.txt
```

with this shape:

```yaml
files:
  - id: sample
    paths:
      - sample.txt
```

For existing fixtures that use `source_path: templates/config.local`, use:

```yaml
files:
  - id: local-config
    paths:
      - templates/config.local
    if_not_exists: true
```

- [ ] **Step 2: Update existing target config override fixtures**

In existing Target Config YAML strings in `src/internal/driftline/plan_test.go`, replace this shape:

```yaml
files:
  - id: local-config
    target_path: config.local
```

with this shape:

```yaml
files:
  - id: local-config
    path_overrides:
      - from: templates/config.local
        to: config.local
```

For tests that override `sample.txt` to `foo.txt`, use:

```yaml
files:
  - id: sample
    path_overrides:
      - from: sample.txt
        to: foo.txt
```

- [ ] **Step 3: Add a multi-file default expansion test**

Add this test before `TestBuildPlanRejectsUnknownSourceID`:

```go
func TestBuildPlanExpandsFileSetToMultipleDefaultTargets(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: github-workflow\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml":      []byte("version: 1\nfiles:\n  - id: github-workflow\n    paths:\n      - .github/workflows/ci.yaml\n      - .github/workflows/release.yaml\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/ci.yaml":      []byte("ci\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/release.yaml": []byte("release\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if item := plan.NextLockItem("github-workflow", ".github/workflows/ci.yaml"); item.TargetPath != ".github/workflows/ci.yaml" {
		t.Fatalf("expected ci workflow lock item, got %#v", item)
	}
	if item := plan.NextLockItem("github-workflow", ".github/workflows/release.yaml"); item.TargetPath != ".github/workflows/release.yaml" {
		t.Fatalf("expected release workflow lock item, got %#v", item)
	}
	assertPlanHasChange(t, plan, StatusAdd, "github-workflow", "target file is missing")
	if got := countChanges(plan, StatusAdd, "github-workflow"); got != 2 {
		t.Fatalf("expected two add changes for github-workflow, got %d in %#v", got, plan.Changes)
	}
}
```

- [ ] **Step 4: Add a path override test**

Add this test after the multi-file default expansion test:

```go
func TestBuildPlanAppliesPathOverridesInsideFileSet(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: github-workflow\n    path_overrides:\n      - from: .github/workflows/ci.yaml\n        to: .github/workflows/project-ci.yaml\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml":      []byte("version: 1\nfiles:\n  - id: github-workflow\n    paths:\n      - .github/workflows/ci.yaml\n      - .github/workflows/release.yaml\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/ci.yaml":      []byte("ci\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/release.yaml": []byte("release\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if item := plan.NextLockItem("github-workflow", ".github/workflows/project-ci.yaml"); item.TargetPath != ".github/workflows/project-ci.yaml" {
		t.Fatalf("expected overridden ci workflow lock item, got %#v", item)
	}
	if item := plan.NextLockItem("github-workflow", ".github/workflows/release.yaml"); item.TargetPath != ".github/workflows/release.yaml" {
		t.Fatalf("expected default release workflow lock item, got %#v", item)
	}
}
```

- [ ] **Step 5: Add unknown override source test**

Add this test after the path override test:

```go
func TestBuildPlanRejectsUnknownPathOverrideSource(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: missing.txt\n        to: custom.txt\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    paths:\n      - sample.txt\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "path override") || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("expected unknown path override error, got %v", err)
	}
}
```

- [ ] **Step 6: Add normalized source read test**

Add this test after the unknown override source test:

```go
func TestBuildPlanReadsNormalizedSourcePaths(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    paths:\n      - ./sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("hello\n"),
		},
	}
	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if item := plan.NextLockItem("sample", "sample.txt"); item.TargetPath != "sample.txt" {
		t.Fatalf("expected normalized target lock item, got %#v", item)
	}
}
```

- [ ] **Step 7: Add test helper for counting same-id changes**

Add this helper near `assertPlanHasChange`:

```go
func countChanges(plan Plan, status Status, id string) int {
	count := 0
	for _, change := range plan.Changes {
		if change.Status == status && change.ID == id {
			count++
		}
	}
	return count
}
```

- [ ] **Step 8: Run focused tests to verify RED**

Run: `go test ./src/internal/driftline -run 'TestBuildPlan'`

Expected: FAIL because `plan.go` still resolves one source path per `id` and still expects `TargetPath`.

- [ ] **Step 9: Review diff checkpoint**

Run: `git diff -- src/internal/driftline/plan_test.go`

Expected: Diff shows file-set plan tests and fixture updates only.

### Task 4: Implement Plan Expansion

**Files:**
- Modify: `src/internal/driftline/plan.go`

- [ ] **Step 1: Expand configured ids into multiple resolved files**

In `planBuilder.build`, replace this block:

```go
		resolved := resolveTargetConfigFile(configured, manifestItem)
		if isReservedTargetPath(resolved.target) {
			return Plan{}, fmt.Errorf("reserved target path %q", resolved.target)
		}
		if _, ok := activeTargets[resolved.target]; ok {
			return Plan{}, fmt.Errorf("duplicate target %q", resolved.target)
		}
		activeTargets[resolved.target] = struct{}{}
		resolvedFiles = append(resolvedFiles, resolved)
```

with this block:

```go
		resolvedForID, err := resolveTargetConfigFile(configured, manifestItem)
		if err != nil {
			return Plan{}, err
		}
		for _, resolved := range resolvedForID {
			if isReservedTargetPath(resolved.target) {
				return Plan{}, fmt.Errorf("reserved target path %q", resolved.target)
			}
			if _, ok := activeTargets[resolved.target]; ok {
				return Plan{}, fmt.Errorf("duplicate target %q", resolved.target)
			}
			activeTargets[resolved.target] = struct{}{}
			resolvedFiles = append(resolvedFiles, resolved)
		}
```

- [ ] **Step 2: Replace single-file resolver with file-set resolver**

Replace `resolveTargetConfigFile` with:

```go
func resolveTargetConfigFile(configured TargetConfigFile, manifestItem SourceManifestFile) ([]resolvedFile, error) {
	ifNotExists := manifestItem.IfNotExists
	if configured.IfNotExists != nil {
		ifNotExists = *configured.IfNotExists
	}

	sourcePaths := map[string]struct{}{}
	for _, sourcePath := range manifestItem.Paths {
		sourcePaths[normalizedConfigPath(sourcePath)] = struct{}{}
	}

	overrides := map[string]string{}
	for _, override := range configured.PathOverrides {
		from := normalizedConfigPath(override.From)
		if _, ok := sourcePaths[from]; !ok {
			return nil, fmt.Errorf("target config file id %q path override from %q does not match a source path", configured.ID, override.From)
		}
		overrides[from] = normalizedConfigPath(override.To)
	}

	resolved := make([]resolvedFile, 0, len(manifestItem.Paths))
	for _, sourcePath := range manifestItem.Paths {
		source := normalizedConfigPath(sourcePath)
		target := source
		if overrideTarget, ok := overrides[source]; ok {
			target = overrideTarget
		}
		resolved = append(resolved, resolvedFile{id: configured.ID, source: source, target: target, ifNotExists: ifNotExists})
	}
	return resolved, nil
}
```

- [ ] **Step 3: Remove the unused lock identity map**

In `planBuilder.build`, remove this unused block:

```go
	lockByIdentity := map[string]LockItem{}
	for _, item := range b.lock.Files {
		lockByIdentity[lockIdentity(item.ID, normalizedConfigPath(item.TargetPath))] = item
	}
```

Remove the `lockIdentity` function if no references remain:

```go
func lockIdentity(id string, target string) string {
	return id + "\x00" + target
}
```

- [ ] **Step 4: Finish normalization rename in plan helpers**

Ensure these functions use `normalizedConfigPath`:

```go
func isReservedTargetPath(target string) bool {
	target = normalizedConfigPath(target)
	return target == TargetConfigPath || target == LockFilePath
}

func normalizedConfigPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
```

- [ ] **Step 5: Run focused tests to verify GREEN**

Run: `go test ./src/internal/driftline -run 'TestBuildPlan'`

Expected: PASS.

- [ ] **Step 6: Run package tests**

Run: `go test ./src/internal/driftline`

Expected: PASS.

- [ ] **Step 7: Review diff checkpoint**

Run: `git diff -- src/internal/driftline/plan.go src/internal/driftline/plan_test.go`

Expected: Diff shows resolved-file expansion, path override handling, and plan tests.

### Task 5: Update Command Tests And Deterministic Sorting

**Files:**
- Modify: `src/internal/driftline/commands/commands_test.go`
- Modify: `src/internal/driftline/commands/changes.go`

- [ ] **Step 1: Update command test fixtures from `source_path` to `paths`**

In every Source Manifest YAML string in `src/internal/driftline/commands/commands_test.go`, replace single `source_path` entries with `paths` arrays. For example, replace:

```yaml
files:
  - id: sample
    source_path: sample.txt
```

with:

```yaml
files:
  - id: sample
    paths:
      - sample.txt
```

For the `local-config` fixture with `if_not_exists`, use:

```yaml
files:
  - id: local-config
    paths:
      - config.local
    if_not_exists: true
```

- [ ] **Step 2: Update init command fixture**

In `TestInitCreatesTargetConfigFromSourceManifest`, replace the source manifest fixture with:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: example\n    paths:\n      - templates/example.txt\n      - templates/example-extra.txt\n  - id: local-config\n    paths:\n      - templates/config.local\n    if_not_exists: true\n"),
```

Replace the old target path assertion with:

```go
if strings.Contains(got, "path_overrides:") || strings.Contains(got, "target_path:") {
	t.Fatalf("target config must not copy source manifest paths as targets:\n%s", got)
}
```

- [ ] **Step 3: Update update command test to copy two files from one id**

In `TestCheckReportsMissingLockAndUpdateCreatesIt`, replace the target config string with:

```go
writeFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: github-workflow\n")
```

Replace the source files map entries for the manifest and source files with:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml":       []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: github-workflow\n    paths:\n      - .github/workflows/ci.yaml\n      - .github/workflows/release.yaml\n"),
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/ci.yaml":      []byte("ci\n"),
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.github/workflows/release.yaml": []byte("release\n"),
```

Replace the check output assertion with:

```go
if !strings.Contains(stdout.String(), "update lock: lock file is missing") || strings.Count(stdout.String(), "add github-workflow") != 2 {
	t.Fatalf("unexpected check output: %q", stdout.String())
}
```

Replace the copied file assertion with:

```go
if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "ci\n" {
	t.Fatalf("unexpected copied ci workflow: %q", got)
}
if got := readFile(t, targetDir, ".github/workflows/release.yaml"); got != "release\n" {
	t.Fatalf("unexpected copied release workflow: %q", got)
}
```

Replace the lock assertion list with:

```go
for _, want := range []string{"version: 1", "repository: y-writings/source-repo", "ref: main", "commit: 0123456789abcdef0123456789abcdef01234567", "target_path: .github/workflows/ci.yaml", "target_path: .github/workflows/release.yaml"} {
	if !strings.Contains(lock, want) {
		t.Fatalf("lock missing %q:\n%s", want, lock)
	}
}
```

- [ ] **Step 4: Make sorted changes deterministic for shared ids**

Replace `sortedChanges` in `src/internal/driftline/commands/changes.go` with:

```go
func sortedChanges(changes []driftline.Change) []driftline.Change {
	out := append([]driftline.Change(nil), changes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Target < out[j].Target
	})
	return out
}
```

- [ ] **Step 5: Run command tests to verify GREEN**

Run: `go test ./src/internal/driftline/commands`

Expected: PASS.

- [ ] **Step 6: Run internal tests**

Run: `go test ./src/internal/driftline/...`

Expected: PASS.

- [ ] **Step 7: Review diff checkpoint**

Run: `git diff -- src/internal/driftline/commands/commands_test.go src/internal/driftline/commands/changes.go`

Expected: Diff shows command fixtures using `paths` and deterministic sorting by target.

### Task 6: Update README And Run Final Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace Source Manifest README example**

Replace the Source Manifest YAML example in `README.md` with:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/y-writings/driftline/main/schema.json
version: 1
gitignore:
  - .cache/tool
files:
  - id: github-workflow
    paths:
      - .github/workflows/ci.yaml
      - .github/workflows/release.yaml
  - id: local-config
    paths:
      - templates/config.local
    if_not_exists: true
```

- [ ] **Step 2: Replace Source Manifest prose**

Replace the sentence after the Source Manifest example with:

```md
Source Manifest file entries define adoption units. Each `id` can expose one or more source-side `paths`; target paths belong to the Target Config.
```

- [ ] **Step 3: Replace Target Config README example**

Replace the Target Config YAML example with:

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
  - id: local-config
    if_not_exists: true
```

- [ ] **Step 4: Replace target path override prose and example**

Replace the current `target_path` paragraph and example with:

````md
When a Target Config file entry has no `path_overrides`, driftline writes each source path to the same relative path in the Target Repository. Add `path_overrides` only for source paths that need a different target-side path:

```yaml
files:
  - id: github-workflow
    path_overrides:
      - from: .github/workflows/ci.yaml
        to: .github/workflows/project-ci.yaml
```
````

- [ ] **Step 5: Keep lock file README example file-oriented**

Update the lock example to show repeated `id` entries for a file set:

```yaml
version: 1
repository: y-writings/source-repo
ref: main
commit: 0123456789abcdef0123456789abcdef01234567
files:
  - id: github-workflow
    target_path: .github/workflows/project-ci.yaml
  - id: github-workflow
    target_path: .github/workflows/release.yaml
```

- [ ] **Step 6: Search for stale current config fields**

Run: `git grep -n -E 'source_path|target_path|path_overrides|paths:' -- README.md schema.json src/internal/driftline docs/superpowers/specs/2026-06-07-file-set-path-overrides-design.md docs/superpowers/plans/2026-06-07-file-set-path-overrides.md`

Expected: `source_path` appears only in docs/spec/plan text that says it is replaced or rejected. `target_path` appears in lock-file code/tests/docs and in docs/plan text that says Target Config rejects it. Current Source Manifest examples use `paths`. Current Target Config examples use `path_overrides` only for overrides.

- [ ] **Step 7: Run full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 8: Review final diff**

Run: `git diff -- schema.json README.md src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/plan.go src/internal/driftline/schema_test.go src/internal/driftline/config_test.go src/internal/driftline/plan_test.go src/internal/driftline/commands/commands_test.go src/internal/driftline/commands/changes.go docs/superpowers/specs/2026-06-07-file-set-path-overrides-design.md docs/superpowers/plans/2026-06-07-file-set-path-overrides.md`

Expected: Diff implements one coherent contract: Source Manifest `paths`, Target Config `path_overrides`, lock `target_path`, tests, README, spec, and plan.

- [ ] **Step 9: Check working tree status**

Run: `git status --short`

Expected: Shows only intended modified files and the two new docs files.
