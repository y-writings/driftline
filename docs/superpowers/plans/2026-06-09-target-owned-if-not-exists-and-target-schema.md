# Target-Owned If-Not-Exists And Target Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `if_not_exists` fully to `.driftline-target.yaml` and add a dedicated Target Config JSON Schema.

**Architecture:** Keep `schema.json` as the Source Manifest schema and add `target-schema.json` for Target Config. Remove source-side `if_not_exists` from Go types, strict parser allow-lists, schema, tests, and README; keep target-side `if_not_exists` as a boolean policy on adoption units.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, JSON Schema draft 2020-12, existing Go test suite.

---

This plan intentionally omits commit steps because repository instructions allow commits only when explicitly requested.

## File Structure

- Modify `src/internal/driftline/types.go`: remove source-side `IfNotExists`; make target-side `IfNotExists` a plain boolean.
- Modify `src/internal/driftline/config.go`: remove `if_not_exists` from Source Manifest allowed keys and stop copying it during `init` target config generation.
- Modify `src/internal/driftline/plan.go`: resolve `if_not_exists` only from `TargetConfigFile`.
- Modify `schema.json`: keep Source Manifest schema current and remove `files[].if_not_exists`.
- Create `target-schema.json`: canonical Target Config schema for `.driftline-target.yaml`.
- Modify `src/internal/driftline/config_test.go`: parser tests for source rejection and target acceptance.
- Modify `src/internal/driftline/schema_test.go`: schema/parser drift tests for both source and target schemas.
- Modify `src/internal/driftline/plan_test.go`: runtime `if_not_exists` fixtures move to Target Config.
- Modify `src/internal/driftline/commands/commands_test.go`: command fixtures move `if_not_exists` to Target Config and init output no longer contains it.
- Modify `README.md`: source/target examples, schema directives, and allowed property lists.

### Task 1: Parser And Schema Tests Describe The New Contract

**Files:**
- Modify: `src/internal/driftline/config_test.go`
- Modify: `src/internal/driftline/schema_test.go`

- [ ] **Step 1: Update Source Manifest parser tests**

In `src/internal/driftline/config_test.go`, replace `TestLoadSourceManifestStrictValidation` with:

```go
func TestLoadSourceManifestStrictValidation(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 1\ngitignore:\n  - ' .cache/tool '\n  - ''\nfiles:\n  - id: example\n    paths:\n      - templates/example.txt\n      - templates/example-extra.txt\n  - id: local-config\n    paths:\n      - templates/config.local\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Files) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if len(manifest.Files[0].Paths) != 2 || manifest.Files[0].Paths[0] != "templates/example.txt" || manifest.Files[0].Paths[1] != "templates/example-extra.txt" {
		t.Fatalf("unexpected source paths: %#v", manifest.Files[0])
	}
	if len(manifest.GitIgnore) != 2 {
		t.Fatalf("gitignore entries should be preserved before write-time trimming: %#v", manifest.GitIgnore)
	}
}
```

In the map inside `TestLoadSourceManifestRejectsUnknownAndDuplicateKeys`, add this case:

```go
"source if_not_exists": "version: 1\nfiles:\n  - id: sample\n    paths:\n      - sample.txt\n    if_not_exists: true\n",
```

- [ ] **Step 2: Update Target Config parser test expectations**

In `src/internal/driftline/config_test.go`, replace `TestLoadTargetConfigDecodesPathOverridesAndExplicitFalse` with:

```go
func TestLoadTargetConfigDecodesPathOverridesAndIfNotExists(t *testing.T) {
	config, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: inherited\n  - id: explicit\n    path_overrides:\n      - from: source.txt\n        to: custom.txt\n    if_not_exists: true\n  - id: explicit-false\n    if_not_exists: false\n"))
	if err != nil {
		t.Fatalf("load target config failed: %v", err)
	}
	if config.Files[0].IfNotExists {
		t.Fatalf("expected omitted if_not_exists to default false")
	}
	if len(config.Files[1].PathOverrides) != 1 || config.Files[1].PathOverrides[0].From != "source.txt" || config.Files[1].PathOverrides[0].To != "custom.txt" {
		t.Fatalf("expected path_overrides to decode, got %#v", config.Files[1])
	}
	if !config.Files[1].IfNotExists {
		t.Fatalf("expected explicit true, got %#v", config.Files[1])
	}
	if config.Files[2].IfNotExists {
		t.Fatalf("expected explicit false, got %#v", config.Files[2])
	}
}
```

- [ ] **Step 3: Update source schema test to reject `if_not_exists`**

In `src/internal/driftline/schema_test.go`, inside `TestSourceManifestSchemaMatchesParserAllowedKeys`, after the existing `target` assertion, add:

```go
if _, ok := fileProperties["if_not_exists"]; ok {
	t.Fatal("source manifest schema must not allow file if_not_exists")
}
```

- [ ] **Step 4: Add target schema test helpers**

In `src/internal/driftline/schema_test.go`, replace `schemaRef` with:

```go
func schemaDef(t *testing.T, schema map[string]any, ref string, name string) map[string]any {
	t.Helper()
	want := "#/$defs/" + name
	if ref != want {
		t.Fatalf("unexpected schema ref: got %q, want %q", ref, want)
	}
	return objectValue(objectValue(schema, "$defs"), name)
}
```

Then replace the `schemaRef` call in `TestSourceManifestSchemaMatchesParserAllowedKeys` with:

```go
fileItemSchema := schemaDef(t, schema, stringValue(objectValue(filesSchema, "items"), "$ref"), "file")
```

Replace `readSourceManifestSchema` with:

```go
func readSourceManifestSchema(t *testing.T) map[string]any {
	t.Helper()
	return readSchema(t, "../../../schema.json")
}

func readTargetConfigSchema(t *testing.T) map[string]any {
	t.Helper()
	return readSchema(t, "../../../target-schema.json")
}

func readSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return schema
}
```

- [ ] **Step 5: Add Target Config schema drift test**

In `src/internal/driftline/schema_test.go`, add this test after `TestSourceManifestSchemaMatchesParserAllowedKeys`:

```go
func TestTargetConfigSchemaMatchesParserAllowedKeys(t *testing.T) {
	schema := readTargetConfigSchema(t)
	allowed := allowedTargetConfigKeys()

	if got := stringValue(schema, "$schema"); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected schema draft: %q", got)
	}
	assertFalseValue(t, "root additionalProperties", schema, "additionalProperties")
	rootProperties := objectValue(schema, "properties")
	assertSameStringSet(t, "root properties", propertyNames(rootProperties), allowed[""])
	assertSameStringSet(t, "root required", stringArrayValue(schema, "required"), map[string]struct{}{"version": {}, "source": {}, "files": {}})

	sourceSchema := schemaDef(t, schema, stringValue(objectValue(rootProperties, "source"), "$ref"), "source")
	assertFalseValue(t, "source additionalProperties", sourceSchema, "additionalProperties")
	assertSameStringSet(t, "source properties", propertyNames(objectValue(sourceSchema, "properties")), allowed["source"])
	assertSameStringSet(t, "source required", stringArrayValue(sourceSchema, "required"), map[string]struct{}{"repository": {}, "ref": {}})

	filesSchema := objectValue(rootProperties, "files")
	fileItemSchema := schemaDef(t, schema, stringValue(objectValue(filesSchema, "items"), "$ref"), "file")
	assertFalseValue(t, "file item additionalProperties", fileItemSchema, "additionalProperties")
	fileProperties := propertyNames(objectValue(fileItemSchema, "properties"))
	assertSameStringSet(t, "file properties", fileProperties, allowed["files"])
	assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}})
	if _, ok := fileProperties["target_path"]; ok {
		t.Fatal("target config schema must not allow old file target_path key")
	}

	pathOverridesSchema := objectValue(objectValue(fileItemSchema, "properties"), "path_overrides")
	if got := numberValue(pathOverridesSchema, "minItems"); got != 1 {
		t.Fatalf("path_overrides must require at least one item, got %v", got)
	}
	overrideItemSchema := schemaDef(t, schema, stringValue(objectValue(pathOverridesSchema, "items"), "$ref"), "pathOverride")
	assertFalseValue(t, "path override additionalProperties", overrideItemSchema, "additionalProperties")
	assertSameStringSet(t, "path override properties", propertyNames(objectValue(overrideItemSchema, "properties")), allowed["path_overrides"])
	assertSameStringSet(t, "path override required", stringArrayValue(overrideItemSchema, "required"), map[string]struct{}{"from": {}, "to": {}})
}
```

- [ ] **Step 6: Run parser and schema tests to verify they fail**

Run:

```sh
go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestSourceManifestSchema|TestTargetConfigSchema'
```

Expected: FAIL with either a compile error for using `TargetConfigFile.IfNotExists` as a `bool` while it is still `*bool`, or a test failure for missing `target-schema.json` / source schema still allowing `if_not_exists` after the type is updated.

### Task 2: Implement Parser, Types, And Schemas

**Files:**
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/config.go`
- Modify: `schema.json`
- Create: `target-schema.json`
- Modify: `src/internal/driftline/schema_test.go`

- [ ] **Step 1: Update Go config types**

In `src/internal/driftline/types.go`, replace `SourceManifestFile` and `TargetConfigFile` with:

```go
type SourceManifestFile struct {
	ID    string   `yaml:"id"`
	Paths []string `yaml:"paths"`
}

type TargetConfigFile struct {
	ID            string         `yaml:"id"`
	PathOverrides []PathOverride `yaml:"path_overrides,omitempty"`
	IfNotExists   bool           `yaml:"if_not_exists,omitempty"`
}
```

- [ ] **Step 2: Update strict parser allow-lists and init generation**

In `src/internal/driftline/config.go`, replace `allowedSourceManifestKeys` with:

```go
func allowedSourceManifestKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":      set("version", "gitignore", "files"),
		"files": set("id", "paths"),
	}
}
```

In `TargetConfigFromSourceManifest`, replace the body inside the `for _, item := range manifest.Files` loop with:

```go
for _, sourcePath := range item.Paths {
	defaultTarget := normalizedConfigPath(sourcePath)
	if _, ok := seenDefaultTargets[defaultTarget]; ok {
		return TargetConfig{}, fmt.Errorf("duplicate target %q", defaultTarget)
	}
	seenDefaultTargets[defaultTarget] = struct{}{}
}
config.Files = append(config.Files, TargetConfigFile{ID: item.ID})
```

Keep `allowedTargetConfigKeys` unchanged:

```go
func allowedTargetConfigKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":               set("version", "source", "files"),
		"source":         set("repository", "ref"),
		"files":          set("id", "path_overrides", "if_not_exists"),
		"path_overrides": set("from", "to"),
	}
}
```

- [ ] **Step 3: Remove source schema `if_not_exists`**

In `schema.json`, replace the `$defs.file.properties` object with:

```json
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
  }
}
```

The file definition still has:

```json
"additionalProperties": false,
"required": ["id", "paths"]
```

- [ ] **Step 4: Add Target Config schema**

Create `target-schema.json` with this exact content:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/y-writings/driftline/main/target-schema.json",
  "title": "driftline Target Config",
  "description": "Schema for .driftline-target.yaml in a Target Repository.",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "source", "files"],
  "properties": {
    "version": {
      "description": "Target config schema version.",
      "const": 1
    },
    "source": {
      "description": "Source Repository to adopt files from.",
      "$ref": "#/$defs/source"
    },
    "files": {
      "description": "Source Manifest adoption units selected by this Target Repository.",
      "type": "array",
      "items": {
        "$ref": "#/$defs/file"
      }
    }
  },
  "$defs": {
    "source": {
      "type": "object",
      "additionalProperties": false,
      "required": ["repository", "ref"],
      "properties": {
        "repository": {
          "description": "GitHub repository in owner/repo form.",
          "type": "string",
          "pattern": "^[^\\s/]+/[^\\s/]+$"
        },
        "ref": {
          "description": "Branch, tag, or commit-ish to read from the Source Repository.",
          "type": "string",
          "minLength": 1
        }
      }
    },
    "file": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id"],
      "properties": {
        "id": {
          "description": "Source Manifest adoption unit identifier.",
          "type": "string",
          "minLength": 1,
          "pattern": "\\S"
        },
        "path_overrides": {
          "description": "Per-source-path target path overrides for this adoption unit.",
          "type": "array",
          "minItems": 1,
          "items": {
            "$ref": "#/$defs/pathOverride"
          }
        },
        "if_not_exists": {
          "description": "When true, driftline does not overwrite existing target files in this adoption unit.",
          "type": "boolean"
        }
      }
    },
    "pathOverride": {
      "type": "object",
      "additionalProperties": false,
      "required": ["from", "to"],
      "properties": {
        "from": {
          "description": "Source path from the adopted Source Manifest paths list.",
          "$ref": "#/$defs/relativePath"
        },
        "to": {
          "description": "Target repository path to write that source path to.",
          "$ref": "#/$defs/relativePath"
        }
      }
    },
    "relativePath": {
      "type": "string",
      "minLength": 1,
      "pattern": "^(?!\\s)(?!.*\\s$)(?!/)(?!\\.$)(?!.*//)(?!.*(?:^|/)\\.\\.(?:/|$))(?!.*\\\\)(?!.*/$).+$"
    }
  }
}
```

- [ ] **Step 5: Run parser and schema tests to verify green**

Run:

```sh
go test ./src/internal/driftline -run 'TestLoadSourceManifest|TestLoadTargetConfig|TestSourceManifestSchema|TestTargetConfigSchema'
```

Expected: PASS.

- [ ] **Step 6: Review focused diff**

Run:

```sh
git diff -- src/internal/driftline/types.go src/internal/driftline/config.go schema.json target-schema.json src/internal/driftline/config_test.go src/internal/driftline/schema_test.go
```

Expected: diff shows source-side `if_not_exists` removed, target-side `if_not_exists` retained as a boolean, and a new `target-schema.json`.

### Task 3: Move Runtime If-Not-Exists Fixtures To Target Config

**Files:**
- Modify: `src/internal/driftline/plan.go`
- Modify: `src/internal/driftline/plan_test.go`

- [ ] **Step 1: Move plan fixture policy to Target Config**

In `src/internal/driftline/plan_test.go`, replace the target config string in `TestBuildPlanPreservesIfNotExistsTargetWithoutHashMetadata` with:

```go
writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: local-config\n    path_overrides:\n      - from: templates/config.local\n        to: config.local\n    if_not_exists: true\n")
```

In the same test, replace the source manifest fixture with:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\nfiles:\n  - id: local-config\n    paths:\n      - templates/config.local\n"),
```

- [ ] **Step 2: Run the focused plan test and verify it fails before runtime update**

Run:

```sh
go test ./src/internal/driftline -run TestBuildPlanPreservesIfNotExistsTargetWithoutHashMetadata
```

Expected: FAIL if runtime still depends on source-side `IfNotExists`, or compile failure before `plan.go` is updated.

- [ ] **Step 3: Update runtime resolution to target-only policy**

In `src/internal/driftline/plan.go`, replace the first lines of `resolveTargetConfigFile` with:

```go
func resolveTargetConfigFile(configured TargetConfigFile, manifestItem SourceManifestFile) ([]resolvedFile, error) {
	ifNotExists := configured.IfNotExists

	sourcePaths := map[string]struct{}{}
```

Remove these old lines from the same function:

```go
ifNotExists := manifestItem.IfNotExists
if configured.IfNotExists != nil {
	ifNotExists = *configured.IfNotExists
}
```

- [ ] **Step 4: Run focused plan test to verify green**

Run:

```sh
go test ./src/internal/driftline -run TestBuildPlanPreservesIfNotExistsTargetWithoutHashMetadata
```

Expected: PASS.

- [ ] **Step 5: Run all driftline package tests**

Run:

```sh
go test ./src/internal/driftline
```

Expected: PASS.

- [ ] **Step 6: Review focused diff**

Run:

```sh
git diff -- src/internal/driftline/plan.go src/internal/driftline/plan_test.go
```

Expected: diff shows runtime policy sourced only from Target Config and test fixtures no longer use Source Manifest `if_not_exists`.

### Task 4: Update Command Tests For Init And Update Behavior

**Files:**
- Modify: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Update init fixture and expectations**

In `TestInitCreatesTargetConfigFromSourceManifest`, replace the fake source manifest fixture with:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: example\n    paths:\n      - templates/example.txt\n      - templates/example-extra.txt\n  - id: local-config\n    paths:\n      - templates/config.local\n"),
```

Replace the `want` list loop with:

```go
for _, want := range []string{"version: 1", "repository: y-writings/source-repo", "ref: main", "id: example", "id: local-config"} {
	if !strings.Contains(got, want) {
		t.Fatalf("generated config missing %q:\n%s", want, got)
	}
}
```

After the existing `path_overrides` / `target_path` assertion, add:

```go
if strings.Contains(got, "if_not_exists") {
	t.Fatalf("target config generated by init must not copy target-side policy from source manifest:\n%s", got)
}
```

- [ ] **Step 2: Move update command policy to Target Config**

In `TestUpdatePreservesIfNotExistsLocalEdits`, replace the target config fixture with:

```go
writeFile(t, targetDir, ".driftline-target.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: local-config\n    if_not_exists: true\n")
```

In the same test, replace the source manifest fixture with:

```go
"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 1\nfiles:\n  - id: local-config\n    paths:\n      - config.local\n"),
```

- [ ] **Step 3: Run focused command tests**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestInitCreatesTargetConfigFromSourceManifest|TestUpdatePreservesIfNotExistsLocalEdits'
```

Expected: PASS.

- [ ] **Step 4: Run all command tests**

Run:

```sh
go test ./src/internal/driftline/commands
```

Expected: PASS.

- [ ] **Step 5: Review focused diff**

Run:

```sh
git diff -- src/internal/driftline/commands/commands_test.go
```

Expected: diff shows source fixtures no longer contain `if_not_exists`; target fixtures contain it only where behavior requires it.

### Task 5: Update README With Property Lists And Target Schema Directive

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update Source Manifest example**

In `README.md`, replace the Source Manifest YAML example with:

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
```

- [ ] **Step 2: Add Source Manifest allowed property list**

After the sentence that begins `Source Manifest file entries define adoption units`, add:

```md
Allowed Source Manifest properties:

| Section | Properties |
| --- | --- |
| root | `version`, `gitignore`, `files` |
| `files[]` | `id`, `paths` |
```

- [ ] **Step 3: Update Target Config example with target schema directive**

In `README.md`, replace the Target Config YAML example with:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/y-writings/driftline/main/target-schema.json
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
  - id: local-config
    if_not_exists: true
```

- [ ] **Step 4: Add Target Config allowed property list**

After the Target Config example, add:

```md
Allowed Target Config properties:

| Section | Properties |
| --- | --- |
| root | `version`, `source`, `files` |
| `source` | `repository`, `ref` |
| `files[]` | `id`, `path_overrides`, `if_not_exists` |
| `path_overrides[]` | `from`, `to` |
```

- [ ] **Step 5: Run README drift search**

Run:

```sh
git diff -- README.md
```

Expected: Source Manifest example has no `if_not_exists`; Target Config example has the target schema directive and keeps target-side `if_not_exists: true`.

### Task 6: Final Verification And Drift Audit

**Files:**
- Verify: implementation, tests, schemas, README

- [ ] **Step 1: Search current implementation and docs for stale source-side `if_not_exists`**

Run:

```sh
rg -n "if_not_exists" README.md schema.json target-schema.json src/internal/driftline
```

Expected current matches:

```text
README.md: target config example/property list only
target-schema.json: target file property only
src/internal/driftline/types.go: TargetConfigFile only
src/internal/driftline/config.go: allowedTargetConfigKeys only
src/internal/driftline/plan.go: existing runtime comment only
src/internal/driftline/*_test.go: source rejection tests and target-side behavior fixtures
```

- [ ] **Step 2: Run internal package tests**

Run:

```sh
go test ./src/internal/driftline/...
```

Expected: PASS.

- [ ] **Step 3: Run full repository tests**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run whitespace diff check**

Run:

```sh
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 5: Review final diff**

Run:

```sh
git diff -- README.md schema.json target-schema.json src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/plan.go src/internal/driftline/config_test.go src/internal/driftline/schema_test.go src/internal/driftline/plan_test.go src/internal/driftline/commands/commands_test.go
```

Expected: diff implements one coherent contract: Source Manifest allows `id` and `paths`; Target Config allows `id`, `path_overrides`, and `if_not_exists`; runtime uses only target-side policy; docs and schemas match parser allow-lists.
