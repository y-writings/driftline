# Target Repository Apply Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Target Repository write sequencing out of `runUpdate` into a Deep apply Module with a narrow, testable Interface.

**Architecture:** Add a `TargetRepository` Module in the `driftline` package. `BuildPlan` continues to own planning and conflict detection; `commands/runUpdate` continues to own CLI conflict reporting and stdout; the new Module owns conflict-safe application order, file deletes, file writes, and final Target manifest commit.

**Tech Stack:** Go, standard library filesystem APIs, existing TOML config writer, existing `go test ./...` test suite.

---

## File Structure

- Create `src/internal/driftline/target_repository.go`: `TargetRepository` type and `Apply(plan Plan) error` method. This Module owns Target Repository writes and the apply ordering.
- Create `src/internal/driftline/target_repository_test.go`: unit tests for conflict rejection, delete-before-write ordering, Target manifest commit ordering, and file-only update behavior.
- Modify `src/internal/driftline/commands/update.go`: replace inline delete/write/config commit code with `TargetRepository.Apply(plan)` while preserving current conflict output.
- Modify `src/internal/driftline/commands/commands_test.go`: keep existing integration coverage unchanged unless a test directly asserts now-removed command-private helpers.
- Keep `src/internal/driftline/plan.go` unchanged in this work.

Do not commit during execution unless the user explicitly requests it.

## Task 1: Add Target Repository Apply Module

**Files:**
- Create: `src/internal/driftline/target_repository.go`
- Test: `src/internal/driftline/target_repository_test.go`

- [ ] **Step 1: Write the failing conflict-rejection test**

Add this test file with a focused first test:

```go
package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetRepositoryApplyRejectsConflictPlanBeforeWriting(t *testing.T) {
	targetDir := t.TempDir()
	writeTargetRepositoryTestFile(t, targetDir, TargetConfigPath, targetConfigTOMLForApplyTest(""))
	writeTargetRepositoryTestFile(t, targetDir, "existing.txt", "target-owned\n")

	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "existing.txt", Status: StatusConflict, Reason: "target already exists", ForceAllowed: true},
			{ID: "tool.config", Target: "existing.txt", TargetPath: filepath.Join(targetDir, "existing.txt"), SourceBytes: []byte("source\n"), Status: StatusUpdate, WritesTarget: true},
		},
		NextConfig: TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "existing.txt"}}},
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
	config := readTargetRepositoryTestFile(t, targetDir, TargetConfigPath)
	if strings.Contains(config, "tool") {
		t.Fatalf("conflict plan must not commit target config:\n%s", config)
	}
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./src/internal/driftline -run TestTargetRepositoryApplyRejectsConflictPlanBeforeWriting -count=1`

Expected: FAIL because `TargetRepository` is undefined.

- [ ] **Step 3: Implement minimal `TargetRepository.Apply` with conflict rejection**

Create `src/internal/driftline/target_repository.go`:

```go
package driftline

import "fmt"

type TargetRepository struct {
	Root string
}

func (r TargetRepository) Apply(plan Plan) error {
	if plan.HasConflicts() {
		return fmt.Errorf("cannot apply conflicted sync plan")
	}
	return nil
}
```

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `go test ./src/internal/driftline -run TestTargetRepositoryApplyRejectsConflictPlanBeforeWriting -count=1`

Expected: PASS.

## Task 2: Move Apply Ordering Into TargetRepository

**Files:**
- Modify: `src/internal/driftline/target_repository.go`
- Modify: `src/internal/driftline/target_repository_test.go`

- [ ] **Step 1: Add tests for delete-before-write and Target manifest commit ordering**

Append these helper functions and tests to `src/internal/driftline/target_repository_test.go`:

```go
func TestTargetRepositoryApplyDeletesBeforeWritingChildPath(t *testing.T) {
	targetDir := t.TempDir()
	writeTargetRepositoryTestFile(t, targetDir, TargetConfigPath, targetConfigTOMLForApplyTest(`[files.old]
config = "dir"
`))
	writeTargetRepositoryTestFile(t, targetDir, "dir", "old\n")

	plan := Plan{
		Changes: []Change{
			{ID: "old.config", Target: "dir", TargetPath: filepath.Join(targetDir, "dir"), Status: StatusRemove, DeletesTarget: true},
			{ID: "old.config", Target: "dir", Status: StatusTargetConfigRemove},
			{ID: "new.config", Target: "dir/file", Status: StatusTargetConfigAdd},
			{ID: "new.config", Target: "dir/file", TargetPath: filepath.Join(targetDir, "dir/file"), SourceBytes: []byte("new\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextConfig: TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"new": {"config": "dir/file"}}},
	}

	if err := TargetRepository{Root: targetDir}.Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, "dir/file"); got != "new\n" {
		t.Fatalf("unexpected child file content: %q", got)
	}
	config := readTargetRepositoryTestFile(t, targetDir, TargetConfigPath)
	if strings.Contains(config, "old") || !strings.Contains(config, `[files.new]`) || !strings.Contains(config, `config = "dir/file"`) {
		t.Fatalf("target config should move to new child entry:\n%s", config)
	}
}

func TestTargetRepositoryApplyDoesNotCommitConfigWhenWriteFails(t *testing.T) {
	targetDir := t.TempDir()
	originalConfig := targetConfigTOMLForApplyTest("")
	writeTargetRepositoryTestFile(t, targetDir, TargetConfigPath, originalConfig)
	writeTargetRepositoryTestFile(t, targetDir, "blocked", "target-owned\n")

	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Target: "blocked/file.txt", Status: StatusTargetConfigAdd},
			{ID: "tool.config", Target: "blocked/file.txt", TargetPath: filepath.Join(targetDir, "blocked/file.txt"), SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true},
		},
		NextConfig: TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "blocked/file.txt"}}},
	}

	if err := TargetRepository{Root: targetDir}.Apply(plan); err == nil {
		t.Fatal("expected write failure")
	}
	if got := readTargetRepositoryTestFile(t, targetDir, TargetConfigPath); got != originalConfig {
		t.Fatalf("target config must not be committed after write failure:\n%s", got)
	}
}

func TestTargetRepositoryApplyDoesNotRewriteConfigForFileOnlyUpdate(t *testing.T) {
	targetDir := t.TempDir()
	originalConfig := `version = 2

# keep target-side comments and order
[source]
ref = "main"
repository = "y-writings/source-repo"

[files.github-workflow]
# local placement rationale
ci = ".github/workflows/ci.yaml"
`
	writeTargetRepositoryTestFile(t, targetDir, TargetConfigPath, originalConfig)
	writeTargetRepositoryTestFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")

	plan := Plan{
		Changes: []Change{
			{ID: "github-workflow.ci", Target: ".github/workflows/ci.yaml", TargetPath: filepath.Join(targetDir, ".github/workflows/ci.yaml"), SourceBytes: []byte("new\n"), Status: StatusUpdate, WritesTarget: true},
		},
		NextConfig: TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"github-workflow": {"ci": ".github/workflows/ci.yaml"}}},
	}

	if err := TargetRepository{Root: targetDir}.Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, ".github/workflows/ci.yaml"); got != "new\n" {
		t.Fatalf("managed file should be updated, got %q", got)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, TargetConfigPath); got != originalConfig {
		t.Fatalf("target config should not be rewritten for file-only update:\n%s", got)
	}
}

func targetConfigTOMLForApplyTest(files string) string {
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
```

- [ ] **Step 2: Run the TargetRepository tests to verify they fail**

Run: `go test ./src/internal/driftline -run TestTargetRepositoryApply -count=1`

Expected: FAIL because `Apply` does not yet delete, write, or commit the Target manifest.

- [ ] **Step 3: Implement apply ordering in `target_repository.go`**

Replace `src/internal/driftline/target_repository.go` with:

```go
package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

type TargetRepository struct {
	Root string
}

func (r TargetRepository) Apply(plan Plan) error {
	if plan.HasConflicts() {
		return fmt.Errorf("cannot apply conflicted sync plan")
	}
	root := r.Root
	if root == "" {
		root = "."
	}

	var commitConfig func() error
	if planHasTargetConfigChanges(plan.Changes) {
		commit, cleanup, err := PrepareTargetConfigWrite(filepath.Join(root, TargetConfigPath), plan.NextConfig)
		if err != nil {
			return err
		}
		defer cleanup()
		commitConfig = commit
	}

	changes := sortedPlanChanges(plan.Changes)
	for _, change := range changes {
		if change.Status == StatusRemove && change.DeletesTarget {
			if err := removeManagedTargetFile(change.TargetPath); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		if (change.Status == StatusAdd || change.Status == StatusUpdate) && change.WritesTarget {
			if err := WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if commitConfig != nil {
		if err := commitConfig(); err != nil {
			return err
		}
	}
	return nil
}

func removeManagedTargetFile(targetPath string) error {
	info, err := os.Lstat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.Remove(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	return nil
}

func planHasTargetConfigChanges(changes []Change) bool {
	for _, change := range changes {
		if change.Status == StatusTargetConfigAdd || change.Status == StatusTargetConfigRemove {
			return true
		}
	}
	return false
}

func sortedPlanChanges(changes []Change) []Change {
	out := append([]Change(nil), changes...)
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

- [ ] **Step 4: Run the TargetRepository tests to verify they pass**

Run: `go test ./src/internal/driftline -run TestTargetRepositoryApply -count=1`

Expected: PASS.

## Task 3: Reuse Change Sorting Between Domain and Commands

**Files:**
- Modify: `src/internal/driftline/types.go`
- Modify: `src/internal/driftline/commands/changes.go`
- Modify: `src/internal/driftline/target_repository.go`

- [ ] **Step 1: Move sorting helper into the `driftline` package**

Add this function to `src/internal/driftline/types.go` after the `Change` type:

```go
func SortedChanges(changes []Change) []Change {
	out := append([]Change(nil), changes...)
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

Add `import "sort"` to `types.go`.

- [ ] **Step 2: Update command sorting wrapper**

Replace `src/internal/driftline/commands/changes.go` with:

```go
package commands

import "github.com/y-writings/driftline/src/internal/driftline"

func sortedChanges(changes []driftline.Change) []driftline.Change {
	return driftline.SortedChanges(changes)
}
```

- [ ] **Step 3: Update TargetRepository to call `SortedChanges`**

In `src/internal/driftline/target_repository.go`, use:

```go
changes := SortedChanges(plan.Changes)
```

Remove any private `sortedPlanChanges` or `sortChanges` helper added during Task 2.

- [ ] **Step 4: Run package tests**

Run: `go test ./src/internal/driftline ./src/internal/driftline/commands -count=1`

Expected: PASS.

## Task 4: Wire runUpdate Through TargetRepository

**Files:**
- Modify: `src/internal/driftline/commands/update.go`
- Test: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Replace inline apply logic in `runUpdate`**

Update `src/internal/driftline/commands/update.go` so it contains only the command orchestration:

```go
package commands

import (
	"io"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runUpdate(source driftline.SourceClient, opts UpdateOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source, ForceKey: opts.ForceKey})
	if err != nil {
		return err
	}
	if plan.HasConflicts() {
		printChanges(stdout, plan.Changes)
		return errDrift
	}
	if err := (driftline.TargetRepository{Root: opts.TargetDir}).Apply(plan); err != nil {
		return err
	}
	printChanges(stdout, plan.Changes)
	return nil
}
```

Remove `removeManagedTargetFile` and `hasTargetConfigChanges` from `commands/update.go`; they now belong to `target_repository.go` as unexported domain helpers.

- [ ] **Step 2: Run existing command update tests**

Run: `go test ./src/internal/driftline/commands -run 'TestUpdate|TestDiffReportsNonContentChanges' -count=1`

Expected: PASS.

- [ ] **Step 3: Run the full Go test suite**

Run: `go test ./... -count=1`

Expected: PASS.

## Task 5: Final Review And Cleanup

**Files:**
- Review: `src/internal/driftline/target_repository.go`
- Review: `src/internal/driftline/commands/update.go`
- Review: `src/internal/driftline/target_repository_test.go`
- Review: `docs/superpowers/specs/2026-06-28-target-repository-apply-module-design.md`

- [ ] **Step 1: Verify spec coverage manually**

Check that implementation satisfies these requirements:

```text
conflict plan rejected before writes
delete stale Managed files before Managed file writes
Target manifest commit after file operations
no rollback implementation
no stdout/stderr in TargetRepository
check/diff behavior unchanged
runUpdate output remains plan-based
```

- [ ] **Step 2: Inspect git diff**

Run: `git diff -- CONTEXT.md docs/superpowers/specs/2026-06-28-target-repository-apply-module-design.md docs/superpowers/plans/2026-06-28-target-repository-apply-module.md src/internal/driftline/target_repository.go src/internal/driftline/target_repository_test.go src/internal/driftline/types.go src/internal/driftline/commands/changes.go src/internal/driftline/commands/update.go`

Expected: diff only contains the documented design, the plan, the new Target Repository apply Module, its tests, shared change sorting, and the simplified update command.

- [ ] **Step 3: Run final verification**

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 4: Report results**

Report changed files and exact verification command output. Do not commit unless the user explicitly asks for a commit.
