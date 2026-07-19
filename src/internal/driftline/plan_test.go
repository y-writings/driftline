package driftline

import (
	"errors"
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
	readErr       error
}

type planSourceAccessFailingClient struct{}

func (planSourceAccessFailingClient) ResolveDefaultRef(repository string) (string, string, error) {
	return "", "", os.ErrPermission
}

func (planSourceAccessFailingClient) ResolveRef(repository string, ref string) (string, error) {
	return "", os.ErrPermission
}

func (planSourceAccessFailingClient) ReadFile(repository string, commit string, path string) ([]byte, error) {
	return nil, os.ErrPermission
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
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestBuildPlanAddsMissingManagedEntryAndTargetFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	client := newPlanSourceClient(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "ci\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	assertPlanHasChange(t, plan, StatusSyncManifestAdd, "github-workflow.ci", "add Sync manifest entry")
	change := planChange(t, plan, StatusAdd, "github-workflow.ci")
	if !change.WritesTarget || change.Target != ".github/workflows/ci.yaml" || string(change.SourceBytes) != "ci\n" {
		t.Fatalf("unexpected add change: %#v", change)
	}
	if got := plan.NextSyncManifest.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("missing next Sync manifest entry: %#v", plan.NextSyncManifest.Files)
	}
}

func TestBuildPlanDoesNotReadOldRootSyncManifest(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.toml", syncManifestTOML(""))

	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: planSourceAccessFailingClient{}})
	if err == nil || !strings.Contains(err.Error(), "Sync manifest not found: .driftline/sync.toml") {
		t.Fatalf("expected canonical Sync manifest error before source access, got %v", err)
	}
}

func TestBuildPlanClassifiesContractReadErrors(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	for _, tt := range []struct {
		name       string
		readErr    error
		wantPrefix string
	}{
		{
			name:       "not found",
			readErr:    os.ErrNotExist,
			wantPrefix: "Contract not found: .driftline/contract.toml: ",
		},
		{
			name:       "provider failure",
			readErr:    providerErr,
			wantPrefix: "read Contract .driftline/contract.toml: ",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			client := fakeSourceClient{
				refs:    map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
				readErr: tt.readErr,
			}

			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Fatalf("expected %q error, got %v", tt.wantPrefix, err)
			}
			if !errors.Is(err, tt.readErr) {
				t.Fatalf("Contract read error should preserve cause %v: %v", tt.readErr, err)
			}
		})
	}
}

func TestBuildPlanUpdatesDeclaredManagedTarget(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.github-workflow]
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
	assertPlanDoesNotHaveChange(t, plan, StatusSyncManifestAdd, "github-workflow.ci")
}

func TestBuildPlanRemovesManagedFileAbsentFromSource(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.github-workflow]
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
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "github-workflow.ci", "remove Sync manifest entry")
	assertPlanHasChange(t, plan, StatusRemove, "github-workflow.ci", "managed file removed from Contract")
	if _, ok := plan.NextSyncManifest.Files["github-workflow"]; ok {
		t.Fatalf("empty group should be removed from next Sync manifest: %#v", plan.NextSyncManifest.Files)
	}
}

func TestBuildPlanManagedToTemplateRemovesSyncManifestEntryAndLeavesTargetFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.github-workflow]
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
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "github-workflow.release", "remove Sync manifest entry")
	assertPlanDoesNotHaveChange(t, plan, StatusRemove, "github-workflow.release")
}

func TestBuildPlanTemplateToManagedConflictsWhenTargetExists(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
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
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
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
	assertPlanHasChange(t, plan, StatusSyncManifestAdd, "github-workflow.ci", "add Sync manifest entry")
}

func TestBuildPlanConflictsWhenNewManagedKeyReusesExistingStaleTarget(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.old]
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

func TestBuildPlanForceMovesStaleSyncManifestPathToNewManagedKey(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.old]
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
	assertPlanHasChange(t, plan, StatusSyncManifestAdd, "new.config", "add Sync manifest entry")
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "old.config", "remove Sync manifest entry")
	assertPlanDoesNotHaveChange(t, plan, StatusRemove, "old.config")
}

func TestBuildPlanAllowsReplacingStaleManagedFileWithDirectoryChild(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.old]
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
	assertPlanHasChange(t, plan, StatusSyncManifestAdd, "new.config", "add Sync manifest entry")
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "old.config", "remove Sync manifest entry")
}

func TestBuildPlanConflictsWhenNewManagedParentReusesStaleChildDirectory(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.old]
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
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "old.config", "remove Sync manifest entry")
}

func TestBuildPlanConflictsWhenNewManagedChildIsBlockedByTargetOwnedFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
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
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
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
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.old]
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
	assertPlanHasChange(t, plan, StatusSyncManifestRemove, "old.config", "remove Sync manifest entry")
}

func TestBuildPlanIgnoresTemplateFilesDuringUpdate(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	client := newPlanSourceClient(`version = 2

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`, map[string]string{".mise/config.toml": "template\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan should ignore template files: %v", err)
	}
	if HasDrift(plan.Changes) {
		t.Fatalf("new templates are not applied during update: %#v", plan.Changes)
	}
}

func TestBuildPlanConflictsWhenNewManagedDefaultTargetIsAlreadyDeclared(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.existing]
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

func TestBuildPlanConflictsWhenManagedTargetPathsOverlap(t *testing.T) {
	tests := []struct {
		name       string
		contract   string
		targets    string
		conflictID string
		target     string
		reason     string
	}{
		{
			name: "parent path before child path",
			contract: `version = 2

[files.a]
parent = { path = "source-parent.txt", mode = "managed" }

[files.b]
child = { path = "source-child.txt", mode = "managed" }
`,
			targets: `[files.a]
parent = "dir"

[files.b]
child = "dir/file"
`,
			conflictID: "b.child",
			target:     "dir/file",
			reason:     "overlaps with a.parent",
		},
		{
			name: "child path before parent path",
			contract: `version = 2

[files.a]
child = { path = "source-child.txt", mode = "managed" }

[files.b]
parent = { path = "source-parent.txt", mode = "managed" }
`,
			targets: `[files.a]
child = "dir/file"

[files.b]
parent = "dir"
`,
			conflictID: "b.parent",
			target:     "dir",
			reason:     "overlaps with a.child",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(tt.targets))
			client := newPlanSourceClient(tt.contract, map[string]string{
				"source-parent.txt": "parent\n",
				"source-child.txt":  "child\n",
			})

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			conflict := planChange(t, plan, StatusConflict, tt.conflictID)
			if conflict.Target != tt.target || !strings.Contains(conflict.Reason, tt.reason) || conflict.ForceAllowed {
				t.Fatalf("unexpected overlapping target conflict: %#v", conflict)
			}
			if !plan.HasConflicts() {
				t.Fatalf("plan should report conflicts: %#v", plan.Changes)
			}
			assertPlanDoesNotHaveChange(t, plan, StatusAdd, tt.conflictID)
		})
	}
}

func TestBuildPlanAddsGitIgnoreSectionToMissingTarget(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	change := plan.GitIgnore
	if change == nil {
		t.Fatal("expected .gitignore section change")
	}
	if change.Status != StatusAdd || change.Reason != "generated section is missing" {
		t.Fatalf("unexpected .gitignore change: %#v", change)
	}
	if change.TargetPath != filepath.Join(targetDir, GitIgnorePath) || !change.TargetMissing {
		t.Fatalf("unexpected .gitignore target: %#v", change)
	}
	if len(change.OriginalBytes) != 0 || string(change.DesiredBytes) != planGitIgnoreBlock("y-writings/source-repo", ".env") {
		t.Fatalf("unexpected .gitignore bytes: %#v", change)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("Gitignore-only drift must not add a Managed sentinel: %#v", plan.Changes)
	}
	if !plan.HasDrift() {
		t.Fatal("plan should report .gitignore section drift")
	}
}

func TestBuildPlanAddsGitIgnoreSectionToTargetOwnedRegularFile(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	current := "node_modules/\n"
	writePlanFile(t, targetDir, GitIgnorePath, current)
	client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	change := plan.GitIgnore
	if change == nil || change.Status != StatusAdd || change.TargetMissing {
		t.Fatalf("unexpected .gitignore add: %#v", change)
	}
	if string(change.OriginalBytes) != current {
		t.Fatalf("original .gitignore bytes = %q, want %q", change.OriginalBytes, current)
	}
	want := current + "\n" + planGitIgnoreBlock("y-writings/source-repo", ".env")
	if string(change.DesiredBytes) != want {
		t.Fatalf("desired .gitignore bytes = %q, want %q", change.DesiredBytes, want)
	}
}

func TestBuildPlanUpdatesDifferingGitIgnoreSectionWithCurrentProvenance(t *testing.T) {
	tests := []struct {
		name    string
		current string
	}{
		{name: "old source", current: planGitIgnoreBlock("old/repo", ".env")},
		{name: "old content", current: planGitIgnoreBlock("y-writings/source-repo", "old-entry")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			change := plan.GitIgnore
			if change == nil || change.Status != StatusUpdate || change.Reason != "generated section differs" {
				t.Fatalf("unexpected .gitignore update: %#v", change)
			}
			if string(change.OriginalBytes) != tt.current {
				t.Fatalf("original .gitignore bytes = %q, want %q", change.OriginalBytes, tt.current)
			}
			want := planGitIgnoreBlock("y-writings/source-repo", ".env")
			if string(change.DesiredBytes) != want {
				t.Fatalf("desired .gitignore bytes = %q, want current repository provenance %q", change.DesiredBytes, want)
			}
		})
	}
}

func TestBuildPlanRemovesUndeclaredGitIgnoreSection(t *testing.T) {
	tests := []struct {
		name     string
		contract string
	}{
		{name: "absent config", contract: "version = 2\n"},
		{name: "empty config", contract: "version = 2\n\n[gitignore]\nentries = []\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			current := "before\n\n" + planGitIgnoreBlock("old/repo", "old-entry") + "after\n"
			writePlanFile(t, targetDir, GitIgnorePath, current)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(tt.contract, nil)})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			change := plan.GitIgnore
			if change == nil || change.Status != StatusRemove || change.Reason != "generated section is no longer declared" {
				t.Fatalf("unexpected .gitignore removal: %#v", change)
			}
			if string(change.OriginalBytes) != current || string(change.DesiredBytes) != "before\n\nafter\n" {
				t.Fatalf("unexpected preserved .gitignore bytes: %#v", change)
			}
		})
	}
}

func TestBuildPlanLeavesUnownedGitIgnoreWithoutConfigUntouched(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	writePlanFile(t, targetDir, GitIgnorePath, "local-only\n")

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient("version = 2\n", nil)})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.GitIgnore != nil {
		t.Fatalf("unexpected .gitignore change: %#v", plan.GitIgnore)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Status != StatusSynced || plan.HasDrift() {
		t.Fatalf("expected synced plan, got %#v", plan)
	}
}

func TestBuildPlanLeavesDesiredGitIgnoreSectionSynced(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	writePlanFile(t, targetDir, GitIgnorePath, planGitIgnoreBlock("y-writings/source-repo", ".env"))
	client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.GitIgnore != nil {
		t.Fatalf("unexpected .gitignore change: %#v", plan.GitIgnore)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Status != StatusSynced || plan.HasDrift() {
		t.Fatalf("expected synced plan, got %#v", plan)
	}
}

func TestBuildPlanRejectsNonRegularGitIgnoreWithActiveConfig(t *testing.T) {
	for _, state := range []string{"directory", "live symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			setPlanGitIgnoreTargetState(t, targetDir, state)
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.Contains(err.Error(), GitIgnorePath) || !strings.Contains(err.Error(), "regular file") {
				t.Fatalf("expected .gitignore regular-file error, got %v", err)
			}
		})
	}
}

func TestBuildPlanIgnoresNonRegularGitIgnoreWithInactiveConfig(t *testing.T) {
	configs := []struct {
		name     string
		contract string
	}{
		{name: "absent", contract: "version = 2\n"},
		{name: "empty", contract: "version = 2\n\n[gitignore]\nentries = []\n"},
	}
	for _, config := range configs {
		for _, state := range []string{"directory", "live symlink", "broken symlink"} {
			t.Run(config.name+"/"+state, func(t *testing.T) {
				targetDir := t.TempDir()
				writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
				setPlanGitIgnoreTargetState(t, targetDir, state)

				plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(config.contract, nil)})
				if err != nil {
					t.Fatalf("inactive config should ignore %s .gitignore: %v", state, err)
				}
				if plan.GitIgnore != nil || plan.HasDrift() {
					t.Fatalf("inactive config should not plan %s .gitignore: %#v", state, plan)
				}
			})
		}
	}
}

func TestBuildPlanReportsUnreadableRegularGitIgnore(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	writePlanFile(t, targetDir, GitIgnorePath, "local\n")
	targetPath := filepath.Join(targetDir, GitIgnorePath)
	if err := os.Chmod(targetPath, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(targetPath, 0o600) })
	if _, err := os.ReadFile(targetPath); err == nil {
		t.Skip("filesystem permits reading a mode 000 file")
	}

	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient("version = 2\n", nil)})
	if err == nil || !strings.Contains(err.Error(), "read "+GitIgnorePath) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected .gitignore read error, got %v", err)
	}
}

func TestBuildPlanRejectsResolvedManagedGitIgnoreTargetWithTable(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "canonical", target: ".gitignore"},
		{name: "normalized", target: ".gitignore/."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = "`+tt.target+`"
`))
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = []

[files.tool]
ignore = { path = "source.ignore", mode = "managed" }
`, map[string]string{"source.ignore": "managed\n"})

			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.Contains(err.Error(), "tool.ignore") || !strings.Contains(err.Error(), "cannot manage .gitignore") {
				t.Fatalf("expected resolved .gitignore ownership error, got %v", err)
			}
		})
	}
}

func TestBuildPlanRejectsResolvedManagedTargetBelowActiveGitIgnore(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "canonical", target: ".gitignore/rules"},
		{name: "normalized", target: ".gitignore/./rules"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = "`+tt.target+`"
`))
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]

[files.tool]
ignore = { path = "source.ignore", mode = "managed" }
`, map[string]string{"source.ignore": "managed\n"})

			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.Contains(err.Error(), "tool.ignore") || !strings.Contains(err.Error(), "cannot be below .gitignore") {
				t.Fatalf("expected resolved .gitignore descendant error, got %v", err)
			}
		})
	}
}

func TestBuildPlanManagedGitIgnoreOwnershipTakesPrecedenceOverGeneratedSection(t *testing.T) {
	tests := []struct {
		name    string
		current string
	}{
		{name: "existing generated section", current: planGitIgnoreBlock("old/repo", "old-entry")},
		{name: "malformed marker is not scanned", current: "# start driftline from old/repo/" + ContractPath + "\nunterminated\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore"
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			desired := "whole-file-managed\n"
			client := newPlanSourceClient(`version = 2

[files.tool]
ignore = { path = ".gitignore", mode = "managed" }
`, map[string]string{GitIgnorePath: desired})

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}
			if plan.GitIgnore != nil {
				t.Fatalf("whole-file Managed ownership must skip section planning: %#v", plan.GitIgnore)
			}
			change := planChange(t, plan, StatusUpdate, "tool.ignore")
			if change.Target != GitIgnorePath || !change.WritesTarget || change.DeletesTarget || string(change.SourceBytes) != desired {
				t.Fatalf("unexpected whole-file Managed plan: %#v", plan)
			}
		})
	}
}

func TestBuildPlanReplacesRemovedManagedGitIgnoreWithGeneratedSection(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		current string
	}{
		{
			name:    "former Managed target-owned lines do not survive",
			target:  GitIgnorePath,
			current: "node_modules/\nlocal-only\n",
		},
		{
			name:    "normalized stale owner with malformed marker is not scanned",
			target:  ".gitignore/.",
			current: "# start driftline from old/repo/" + ContractPath + "\nunterminated\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = "`+tt.target+`"
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			remove := planChange(t, plan, StatusRemove, "tool.ignore")
			if remove.Target != GitIgnorePath || !remove.DeletesTarget || remove.WritesTarget {
				t.Fatalf("unexpected former Managed removal: %#v", remove)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			change := plan.GitIgnore
			if change == nil || change.Status != StatusAdd || change.Reason != "generated section is missing" {
				t.Fatalf("unexpected generated replacement: %#v", change)
			}
			if change.TargetMissing || string(change.OriginalBytes) != tt.current {
				t.Fatalf("replacement must retain current target state for stale validation: %#v", change)
			}
			want := planGitIgnoreBlock("y-writings/source-repo", ".env")
			if string(change.DesiredBytes) != want {
				t.Fatalf("replacement bytes = %q, want generated block only %q", change.DesiredBytes, want)
			}
		})
	}
}

func TestBuildPlanRemovesManagedGitIgnoreWithoutSectionPlanning(t *testing.T) {
	tests := []struct {
		name     string
		contract string
	}{
		{name: "absent config", contract: "version = 2\n"},
		{name: "explicit empty config", contract: "version = 2\n\n[gitignore]\nentries = []\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore"
`))
			writePlanFile(t, targetDir, GitIgnorePath, "# start driftline from old/repo/"+ContractPath+"\nunterminated\n")

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(tt.contract, nil)})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			remove := planChange(t, plan, StatusRemove, "tool.ignore")
			if remove.Target != GitIgnorePath || !remove.DeletesTarget || remove.WritesTarget {
				t.Fatalf("unexpected Managed removal: %#v", remove)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			if plan.GitIgnore != nil {
				t.Fatalf("inactive config must not add a dedicated .gitignore change: %#v", plan.GitIgnore)
			}
		})
	}
}

func TestBuildPlanManagedToTemplateGitIgnoreUsesCurrentBytesForSection(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		wantStatus Status
		want       string
	}{
		{
			name:       "append",
			current:    "local-only\n",
			wantStatus: StatusAdd,
			want:       "local-only\n\n" + planGitIgnoreBlock("y-writings/source-repo", ".env"),
		},
		{
			name:       "replace",
			current:    planGitIgnoreBlock("old/repo", "old-entry"),
			wantStatus: StatusUpdate,
			want:       planGitIgnoreBlock("y-writings/source-repo", ".env"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore/."
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]

[files.tool]
ignore = { path = "./.gitignore", mode = "template" }
`, nil)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			transition := planChange(t, plan, StatusModeTransition, "tool.ignore")
			if transition.Target != GitIgnorePath || transition.WritesTarget || transition.DeletesTarget {
				t.Fatalf("Managed-to-Template transition must leave the file for section planning: %#v", transition)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			change := plan.GitIgnore
			if change == nil || change.Status != tt.wantStatus || change.TargetMissing {
				t.Fatalf("unexpected dedicated section change: %#v", change)
			}
			if string(change.OriginalBytes) != tt.current || string(change.DesiredBytes) != tt.want {
				t.Fatalf("dedicated change did not transform current bytes: %#v", change)
			}
		})
	}
}

func TestBuildPlanManagedToRenamedTemplateGitIgnoreUsesCurrentBytesForSection(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		wantStatus Status
		want       string
	}{
		{
			name:       "append",
			current:    "local-only\n",
			wantStatus: StatusAdd,
			want:       "local-only\n\n" + planGitIgnoreBlock("y-writings/source-repo", ".env"),
		},
		{
			name:       "replace",
			current:    planGitIgnoreBlock("old/repo", "old-entry"),
			wantStatus: StatusUpdate,
			want:       planGitIgnoreBlock("y-writings/source-repo", ".env"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore"
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]

[files.tool]
ignore = { path = "templates/ignore", mode = "template" }
`, nil)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			transition := planChange(t, plan, StatusModeTransition, "tool.ignore")
			if transition.Target != GitIgnorePath || transition.WritesTarget || transition.DeletesTarget {
				t.Fatalf("Managed-to-Template transition must leave the former target for section planning: %#v", transition)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			change := plan.GitIgnore
			if change == nil || change.Status != tt.wantStatus || change.TargetMissing {
				t.Fatalf("unexpected dedicated section change: %#v", change)
			}
			if string(change.OriginalBytes) != tt.current || string(change.DesiredBytes) != tt.want {
				t.Fatalf("dedicated change did not transform former target bytes: %#v", change)
			}
		})
	}
}

func TestBuildPlanManagedToTemplateGitIgnoreWithInactiveConfigSkipsSectionPlanning(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		current  string
	}{
		{
			name: "absent config leaves valid section untouched",
			contract: `version = 2

[files.tool]
ignore = { path = ".gitignore", mode = "template" }
`,
			current: planGitIgnoreBlock("old/repo", "old-entry"),
		},
		{
			name: "explicit empty config does not scan malformed marker",
			contract: `version = 2

[gitignore]
entries = []

[files.tool]
ignore = { path = ".gitignore", mode = "template" }
`,
			current: "# start driftline from old/repo/" + ContractPath + "\nunterminated\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore"
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(tt.contract, nil)})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			transition := planChange(t, plan, StatusModeTransition, "tool.ignore")
			if transition.Target != GitIgnorePath || transition.WritesTarget || transition.DeletesTarget {
				t.Fatalf("unexpected Managed-to-Template transition: %#v", transition)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			if plan.GitIgnore != nil {
				t.Fatalf("inactive config must leave former Managed bytes untouched: %#v", plan.GitIgnore)
			}
		})
	}
}

func TestBuildPlanManagedToRenamedTemplateGitIgnoreWithInactiveConfigSkipsSectionPlanning(t *testing.T) {
	tests := []struct {
		name            string
		gitIgnoreConfig string
		current         string
	}{
		{
			name:            "absent config leaves valid section untouched",
			gitIgnoreConfig: "",
			current:         planGitIgnoreBlock("old/repo", "old-entry"),
		},
		{
			name:            "explicit empty config does not scan malformed marker",
			gitIgnoreConfig: "\n[gitignore]\nentries = []\n",
			current:         "# start driftline from old/repo/" + ContractPath + "\nunterminated\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(`[files.tool]
ignore = ".gitignore"
`))
			writePlanFile(t, targetDir, GitIgnorePath, tt.current)
			contract := "version = 2\n" + tt.gitIgnoreConfig + `
[files.tool]
ignore = { path = "templates/ignore", mode = "template" }
`

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(contract, nil)})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}

			transition := planChange(t, plan, StatusModeTransition, "tool.ignore")
			if transition.Target != GitIgnorePath || transition.WritesTarget || transition.DeletesTarget {
				t.Fatalf("unexpected Managed-to-Template transition: %#v", transition)
			}
			assertPlanHasChange(t, plan, StatusSyncManifestRemove, "tool.ignore", "remove Sync manifest entry")
			if plan.GitIgnore != nil {
				t.Fatalf("inactive config must leave former Managed bytes untouched: %#v", plan.GitIgnore)
			}
		})
	}
}

func TestBuildPlanPlansManagedAndGitIgnoreChangesTogether(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]

[files.tool]
config = { path = "tool.toml", mode = "managed" }
`, map[string]string{"tool.toml": "managed\n"})

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	managed := planChange(t, plan, StatusAdd, "tool.config")
	if !managed.WritesTarget || managed.Target != "tool.toml" {
		t.Fatalf("unexpected Managed add: %#v", managed)
	}
	if plan.GitIgnore == nil || plan.GitIgnore.Status != StatusAdd || !plan.HasDrift() {
		t.Fatalf("missing .gitignore add alongside Managed changes: %#v", plan)
	}
}

func TestPlanHasDriftDelegatesToManagedChanges(t *testing.T) {
	if (Plan{Changes: []Change{{Status: StatusSynced}}}).HasDrift() {
		t.Fatal("synced Managed changes should not report drift")
	}
	if !(Plan{Changes: []Change{{Status: StatusUpdate}}}).HasDrift() {
		t.Fatal("drifted Managed changes should report drift")
	}
}

func syncManifestTOML(files string) string {
	return `version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"
` + files
}

func planGitIgnoreBlock(repository string, entry string) string {
	return "# start driftline from " + repository + "/" + ContractPath + "\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\n" +
		entry + "\n" +
		"# end driftline\n"
}

func setPlanGitIgnoreTargetState(t *testing.T, root string, state string) {
	t.Helper()
	targetPath := filepath.Join(root, GitIgnorePath)
	switch state {
	case "directory":
		if err := os.Mkdir(targetPath, 0o755); err != nil {
			t.Fatal(err)
		}
	case "live symlink":
		writePlanFile(t, root, "real-gitignore", "outside\n")
		if err := os.Symlink("real-gitignore", targetPath); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
	case "broken symlink":
		if err := os.Symlink("missing-gitignore", targetPath); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
	default:
		t.Fatalf("unknown .gitignore target state %q", state)
	}
}

func newPlanSourceClient(contract string, files map[string]string) fakeSourceClient {
	commit := "0123456789abcdef0123456789abcdef01234567"
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": commit},
		files: map[string][]byte{
			"y-writings/source-repo@" + commit + ":" + ContractPath: []byte(contract),
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
