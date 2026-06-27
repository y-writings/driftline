package driftline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSourceClient struct {
	defaultRef    string
	defaultCommit string
	refs          map[string]string
	files         map[string][]byte
}

func (f fakeSourceClient) ResolveDefaultRef(repository string) (string, string, error) {
	return f.defaultRef, f.defaultCommit, nil
}

func (f fakeSourceClient) ResolveRef(repository string, ref string) (string, error) {
	commit, ok := f.refs[repository+"@"+ref]
	if !ok {
		return "", os.ErrNotExist
	}
	return commit, nil
}

func (f fakeSourceClient) ReadFile(repository string, commit string, path string) ([]byte, error) {
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestBuildPlanAddsMissingManagedEntryAndTargetFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(""))
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "ci\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	assertPlanHasChange(t, plan, StatusTargetConfigAdd, "github-workflow.ci", "add target config entry")
	change := planChange(t, plan, StatusAdd, "github-workflow.ci")
	if !change.WritesTarget || change.Target != ".github/workflows/ci.yaml" || string(change.SourceBytes) != "ci\n" {
		t.Fatalf("unexpected add change: %#v", change)
	}
	if got := plan.NextConfig.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("missing next target config entry: %#v", plan.NextConfig.Files)
	}
}

func TestBuildPlanUpdatesDeclaredManagedTarget(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
`))
	writePlanFile(t, targetDir, ".github/workflows/project-ci.yaml", "old\n")
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	change := planChange(t, plan, StatusUpdate, "github-workflow.ci")
	if !change.WritesTarget || change.Target != ".github/workflows/project-ci.yaml" || string(change.SourceBytes) != "new\n" {
		t.Fatalf("unexpected update change: %#v", change)
	}
	assertPlanDoesNotHaveChange(t, plan, StatusTargetConfigAdd, "github-workflow.ci")
}

func TestBuildPlanRemovesManagedFileAbsentFromSource(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`))
	writePlanFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newPlanSourceClient(`version = 2
`, nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	remove := planChange(t, plan, StatusRemove, "github-workflow.ci")
	if !remove.DeletesTarget || remove.Target != ".github/workflows/ci.yaml" {
		t.Fatalf("unexpected remove change: %#v", remove)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "github-workflow.ci", "remove target config entry")
	if _, ok := plan.NextConfig.Files["github-workflow"]; ok {
		t.Fatalf("empty group should be removed from next config: %#v", plan.NextConfig.Files)
	}
}

func TestBuildPlanManagedToTemplateRemovesConfigAndLeavesTargetFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.github-workflow]
release = ".github/workflows/release.yaml"
`))
	writePlanFile(t, targetDir, ".github/workflows/release.yaml", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
release = { path = ".github/workflows/release.yaml", mode = "template" }
`, map[string]string{".github/workflows/release.yaml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	transition := planChange(t, plan, StatusModeTransition, "github-workflow.release")
	if transition.DeletesTarget || transition.WritesTarget {
		t.Fatalf("managed-to-template must leave target file untouched: %#v", transition)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "github-workflow.release", "remove target config entry")
	assertPlanDoesNotHaveChange(t, plan, StatusRemove, "github-workflow.release")
}

func TestBuildPlanTemplateToManagedConflictsWhenTargetExists(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(""))
	writePlanFile(t, targetDir, ".github/workflows/ci.yaml", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	conflict := planChange(t, plan, StatusConflict, "github-workflow.ci")
	if conflict.Target != ".github/workflows/ci.yaml" || !strings.Contains(conflict.Reason, "target already exists") {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
	if !plan.HasConflicts() {
		t.Fatalf("plan should report conflicts: %#v", plan.Changes)
	}
}

func TestBuildPlanForceAllowsOneManagedOverwrite(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(""))
	writePlanFile(t, targetDir, ".github/workflows/ci.yaml", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client, ForceKey: "github-workflow.ci"})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.HasConflicts() {
		t.Fatalf("forced key should not conflict: %#v", plan.Changes)
	}
	change := planChange(t, plan, StatusUpdate, "github-workflow.ci")
	if !change.WritesTarget || string(change.SourceBytes) != "source\n" {
		t.Fatalf("unexpected forced update change: %#v", change)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigAdd, "github-workflow.ci", "add target config entry")
}

func TestBuildPlanConflictsWhenNewManagedKeyReusesExistingStaleTarget(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.old]
config = "same.txt"
`))
	writePlanFile(t, targetDir, "same.txt", "old\n")
	client := newPlanSourceClient(`version = 2

[files.new]
config = { path = "same.txt", mode = "managed" }
`, map[string]string{"same.txt": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	conflict := planChange(t, plan, StatusConflict, "new.config")
	if conflict.Target != "same.txt" || !strings.Contains(conflict.Reason, "target already exists") || !conflict.ForceAllowed {
		t.Fatalf("unexpected conflict for reused stale target: %#v", conflict)
	}
	assertPlanDoesNotHaveChange(t, plan, StatusUpdate, "new.config")
}

func TestBuildPlanForceMovesStaleTargetConfigPathToNewManagedKey(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.old]
config = "same.txt"
`))
	writePlanFile(t, targetDir, "same.txt", "old\n")
	client := newPlanSourceClient(`version = 2

[files.new]
config = { path = "same.txt", mode = "managed" }
`, map[string]string{"same.txt": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client, ForceKey: "new.config"})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.HasConflicts() {
		t.Fatalf("forced stale target reuse should not conflict: %#v", plan.Changes)
	}
	change := planChange(t, plan, StatusUpdate, "new.config")
	if !change.WritesTarget || change.Target != "same.txt" {
		t.Fatalf("new managed key should update reused target path: %#v", change)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigAdd, "new.config", "add target config entry")
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "old.config", "remove target config entry")
	assertPlanDoesNotHaveChange(t, plan, StatusRemove, "old.config")
}

func TestBuildPlanAllowsReplacingStaleManagedFileWithDirectoryChild(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.old]
config = "dir"
`))
	writePlanFile(t, targetDir, "dir", "old\n")
	client := newPlanSourceClient(`version = 2

[files.new]
config = { path = "dir/file", mode = "managed" }
`, map[string]string{"dir/file": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	remove := planChange(t, plan, StatusRemove, "old.config")
	if !remove.DeletesTarget || remove.Target != "dir" {
		t.Fatalf("unexpected stale removal: %#v", remove)
	}
	add := planChange(t, plan, StatusAdd, "new.config")
	if !add.WritesTarget || add.Target != "dir/file" || string(add.SourceBytes) != "new\n" {
		t.Fatalf("unexpected child add: %#v", add)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigAdd, "new.config", "add target config entry")
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "old.config", "remove target config entry")
}

func TestBuildPlanConflictsWhenNewManagedParentReusesStaleChildDirectory(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.old]
config = "dir/file"
`))
	writePlanFile(t, targetDir, "dir/file", "old\n")
	client := newPlanSourceClient(`version = 2

[files.new]
config = { path = "dir", mode = "managed" }
`, map[string]string{"dir": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	conflict := planChange(t, plan, StatusConflict, "new.config")
	if conflict.Target != "dir" || !strings.Contains(conflict.Reason, "target already exists") || conflict.ForceAllowed {
		t.Fatalf("unexpected parent directory conflict: %#v", conflict)
	}
	remove := planChange(t, plan, StatusRemove, "old.config")
	if !remove.DeletesTarget || remove.Target != "dir/file" {
		t.Fatalf("unexpected stale child removal: %#v", remove)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "old.config", "remove target config entry")
}

func TestBuildPlanConflictsWhenNewManagedChildIsBlockedByTargetOwnedFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(""))
	writePlanFile(t, targetDir, "config", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.tool]
config = { path = "config/tool.toml", mode = "managed" }
`, map[string]string{"config/tool.toml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	conflict := planChange(t, plan, StatusConflict, "tool.config")
	if conflict.Target != "config/tool.toml" || !strings.Contains(conflict.Reason, "target already exists") || conflict.ForceAllowed {
		t.Fatalf("unexpected blocked child conflict: %#v", conflict)
	}
	assertPlanDoesNotHaveChange(t, plan, StatusAdd, "tool.config")
}

func TestBuildPlanConflictsWhenDeclaredManagedChildIsBlockedByTargetOwnedFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.tool]
config = "config/tool.toml"
`))
	writePlanFile(t, targetDir, "config", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.tool]
config = { path = "config/tool.toml", mode = "managed" }
`, map[string]string{"config/tool.toml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	conflict := planChange(t, plan, StatusConflict, "tool.config")
	if conflict.Target != "config/tool.toml" || !strings.Contains(conflict.Reason, "target already exists") || conflict.ForceAllowed {
		t.Fatalf("unexpected declared blocked child conflict: %#v", conflict)
	}
	assertPlanDoesNotHaveChange(t, plan, StatusAdd, "tool.config")
	assertPlanDoesNotHaveChange(t, plan, StatusUpdate, "tool.config")
}

func TestBuildPlanConflictsWhenStaleChildProbeIsBlockedByTargetOwnedFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.old]
config = "config/old"
`))
	writePlanFile(t, targetDir, "config", "target-owned\n")
	client := newPlanSourceClient(`version = 2

[files.new]
config = { path = "config/old/tool.toml", mode = "managed" }
`, map[string]string{"config/old/tool.toml": "source\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	conflict := planChange(t, plan, StatusConflict, "new.config")
	if conflict.Target != "config/old/tool.toml" || !strings.Contains(conflict.Reason, "target already exists") || conflict.ForceAllowed {
		t.Fatalf("unexpected stale probe conflict: %#v", conflict)
	}
	remove := planChange(t, plan, StatusRemove, "old.config")
	if !remove.DeletesTarget || remove.Target != "config/old" {
		t.Fatalf("unexpected stale removal: %#v", remove)
	}
	assertPlanHasChange(t, plan, StatusTargetConfigRemove, "old.config", "remove target config entry")
}

func TestBuildPlanIgnoresTemplateAndOldLockFilesDuringUpdate(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(""))
	writePlanFile(t, targetDir, "driftline-lock.yaml", "not: valid: yaml\n")
	client := newPlanSourceClient(`version = 2

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`, map[string]string{".mise/config.toml": "template\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan should ignore template and lock files: %v", err)
	}
	if HasDrift(plan.Changes) {
		t.Fatalf("new templates are not applied during update: %#v", plan.Changes)
	}
}

func TestBuildPlanConflictsWhenNewManagedDefaultTargetIsAlreadyDeclared(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, TargetConfigPath, targetConfigTOML(`[files.existing]
config = "same.txt"
`))
	client := newPlanSourceClient(`version = 2

[files.existing]
config = { path = "existing.txt", mode = "managed" }

[files.new]
config = { path = "same.txt", mode = "managed" }
`, map[string]string{"existing.txt": "existing\n", "same.txt": "new\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	conflict := planChange(t, plan, StatusConflict, "new.config")
	if !strings.Contains(conflict.Reason, "already declared") {
		t.Fatalf("unexpected duplicate target conflict: %#v", conflict)
	}
	if conflict.ForceAllowed {
		t.Fatalf("force must not be advertised for duplicate desired targets: %#v", conflict)
	}
}

func targetConfigTOML(files string) string {
	return `version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"
` + files
}

func newPlanSourceClient(sourceManifest string, files map[string]string) fakeSourceClient {
	commit := "0123456789abcdef0123456789abcdef01234567"
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": commit},
		files: map[string][]byte{
			"y-writings/source-repo@" + commit + ":" + SourceManifestPath: []byte(sourceManifest),
		},
	}
	for path, content := range files {
		client.files["y-writings/source-repo@"+commit+":"+path] = []byte(content)
	}
	return client
}

func writePlanFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertPlanHasChange(t *testing.T, plan Plan, status Status, id string, reason string) {
	t.Helper()
	for _, change := range plan.Changes {
		if change.Status == status && change.ID == id && strings.Contains(change.Reason, reason) {
			return
		}
	}
	t.Fatalf("missing change %s %s containing %q in %#v", status, id, reason, plan.Changes)
}

func planChange(t *testing.T, plan Plan, status Status, id string) Change {
	t.Helper()
	for _, change := range plan.Changes {
		if change.Status == status && change.ID == id {
			return change
		}
	}
	t.Fatalf("missing change %s %s in %#v", status, id, plan.Changes)
	return Change{}
}

func assertPlanDoesNotHaveChange(t *testing.T, plan Plan, status Status, id string) {
	t.Helper()
	for _, change := range plan.Changes {
		if change.Status == status && change.ID == id {
			t.Fatalf("unexpected change %s %s in %#v", status, id, plan.Changes)
		}
	}
}
