# Source Target Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `target` from Source Repository manifests while keeping Target Repository `target` optional and defaulting the effective target path to the source path.

**Architecture:** Source manifests declare only file identity and source-side paths. Target configs select source file IDs and optionally override the target-side path. Plan resolution computes `effectiveTarget = targetConfigFile.Target`, falling back to `sourceManifestFile.Source` when the target config omits `target`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, existing command tests and plan tests.

---

### Task 1: Tests Describe New Schema Boundary

**Files:**
- Modify: `src/internal/driftline/config_test.go`
- Modify: `src/internal/driftline/plan_test.go`
- Modify: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Update config tests first**

Change `TestLoadSourceManifestStrictValidation` so source manifest file entries contain `id`, `source`, and optional `if_not_exists`, but no `target`. Add a case in `TestLoadSourceManifestRejectsUnknownAndDuplicateKeys` proving `target` is rejected as an unknown source manifest file key.

- [ ] **Step 2: Update plan tests first**

Change plan fixtures so source manifests omit `target`. Add or adjust assertions to prove a target config entry with no `target` resolves to the source path, while a target config entry with `target` still overrides the source path.

- [ ] **Step 3: Update command tests first**

Change init/update command fixtures so source manifests omit `target`. Assert `driftline init` writes target config entries with only `id` and copied target-side policy, not copied `target` paths.

- [ ] **Step 4: Run tests to verify RED**

Run: `go test ./src/internal/driftline/...`

Expected: FAIL because the parser still accepts/requires source manifest `target`, plan resolution still falls back to manifest `target`, and init still copies manifest targets into target config.

### Task 2: Implement Source Manifest Without Target

**Files:**
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/config.go`
- Modify: `src/internal/driftline/plan.go`

- [ ] **Step 1: Remove target from the source manifest type and allowed keys**

Remove `Target string yaml:"target"` from `SourceManifestFile`. Remove `target` from `allowedSourceManifestKeys()`.

- [ ] **Step 2: Stop validating source manifest target**

In `validateSourceManifest`, keep validating `item.Source` and remove validation of `item.Target`.

- [ ] **Step 3: Stop copying manifest target during init config generation**

In `TargetConfigFromSourceManifest`, create each `TargetConfigFile` with `ID: item.ID` only, while preserving `if_not_exists` when present.

- [ ] **Step 4: Resolve omitted target config paths from source paths**

In `resolveTargetConfigFile`, use `manifestItem.Source` when `configured.Target == ""`. Normalize the resulting target path before duplicate and reserved path checks.

- [ ] **Step 5: Run tests to verify GREEN**

Run: `go test ./src/internal/driftline/...`

Expected: PASS.

### Task 3: Update User-Facing Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update examples**

Remove `target` from Source Manifest examples. Show Target Config examples where `target` is omitted for same-path placement and present only for explicit override.

- [ ] **Step 2: Document defaulting rule**

Add a short sentence: when a Target Config file entry omits `target`, driftline writes to the same relative path as the source manifest `source` path.

- [ ] **Step 3: Run full verification**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 4: Review diff**

Run: `git diff -- README.md src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/plan.go src/internal/driftline/config_test.go src/internal/driftline/plan_test.go src/internal/driftline/commands/commands_test.go docs/superpowers/plans/2026-06-02-source-target-boundary.md`

Expected: Diff shows only the schema boundary change, tests, docs, and this plan. Do not commit unless explicitly requested.
