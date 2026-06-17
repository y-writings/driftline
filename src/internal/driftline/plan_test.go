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

func TestBuildPlanDetectsMissingLockAndAdd(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("hello\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if item := plan.NextLockItem("sample", "sample.txt"); item.TargetPath != "sample.txt" {
		t.Fatalf("expected omitted target config to default to source path, got %#v", item)
	}
	assertPlanHasChange(t, plan, StatusUpdate, "lock", "lock file is missing")
	assertPlanHasChange(t, plan, StatusAdd, "sample", "target file is missing")
}

func TestBuildPlanPreservesIfNotExistsTargetWithoutHashMetadata(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: local-config\n    path_overrides:\n      config: config.local\n    if_not_exists: true\n")
	writePlanFile(t, targetDir, "config.local", "edited-local\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: local-config\n    target_path: config.local\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: local-config\n    paths:\n      config:\n        path: templates/config.local\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:templates/config.local": []byte("from-source\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if item := plan.NextLockItem("local-config", "config.local"); item.TargetPath != "config.local" {
		t.Fatalf("expected existing target to remain locked by identity, got %#v", item)
	}
	assertPlanHasChange(t, plan, StatusUpdate, "lock", "source commit changed")
	assertPlanDoesNotHaveChange(t, plan, StatusUpdate, "local-config")
}

func TestBuildPlanReportsManagedTargetEditAsDriftWithoutHashMetadata(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	writePlanFile(t, targetDir, "sample.txt", "edited-local\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target_path: sample.txt\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("from-source\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	change := planChange(t, plan, StatusUpdate, "sample")
	if !change.WritesTarget || !strings.Contains(change.Reason, "target differs from source") {
		t.Fatalf("expected managed target edit to be overwritten from source, got %#v", change)
	}
}

func TestBuildPlanLeavesOldTargetAsPruneCandidateWhenDefaultSourcePathChanges(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	writePlanFile(t, targetDir, "old.txt", "old\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: sample\n    target_path: old.txt\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: new.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:new.txt":                []byte("new\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	assertPlanHasChange(t, plan, StatusAdd, "sample", "target file is missing")
	assertPlanHasChange(t, plan, StatusPrune, "sample", "target is no longer adopted")
}

func TestBuildPlanExpandsFileSetToMultipleDefaultTargets(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: github-workflow\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml":         []byte("version: 2\nfiles:\n  - id: github-workflow\n    paths:\n      ci:\n        name: CI workflow\n        path: .github/workflows/ci.yaml\n      release:\n        name: Release workflow\n        path: .github/workflows/release.yaml\n"),
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

func TestBuildPlanAppliesPathOverridesInsideFileSet(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: github-workflow\n    path_overrides:\n      ci: .github/workflows/project-ci.yaml\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml":         []byte("version: 2\nfiles:\n  - id: github-workflow\n    paths:\n      ci:\n        path: .github/workflows/ci.yaml\n      release:\n        path: .github/workflows/release.yaml\n"),
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

func TestBuildPlanRejectsUnknownPathOverrideSource(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      missing: custom.txt\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "path override") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected unknown path override error, got %v", err)
	}
}

func TestBuildPlanReadsNormalizedSourcePaths(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: ./sample.txt\n"),
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

func TestBuildPlanRejectsUnknownSourceID(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: missing\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles: []\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "unknown source file id") {
		t.Fatalf("expected unknown id error, got %v", err)
	}
}

func TestBuildPlanRejectsResolvedDuplicateTargets(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: first\n    path_overrides:\n      first: same.txt\n  - id: second\n    path_overrides:\n      second: same.txt\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: first\n    paths:\n      first:\n        path: first.txt\n  - id: second\n    paths:\n      second:\n        path: second.txt\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("expected duplicate target error, got %v", err)
	}
}

func TestBuildPlanRejectsNormalizedDuplicateTargets(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: first\n    path_overrides:\n      first: foo.txt\n  - id: second\n    path_overrides:\n      second: ./foo.txt\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: first\n    paths:\n      first:\n        path: first.txt\n  - id: second\n    paths:\n      second:\n        path: second.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:first.txt":              []byte("first\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:second.txt":             []byte("second\n"),
		},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("expected normalized duplicate target error, got %v", err)
	}
}

func TestBuildPlanTreatsNormalizedLockTargetAsActive(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      sample: ./foo.txt\n")
	writePlanFile(t, targetDir, "foo.txt", "hello\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target_path: foo.txt\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("hello\n"),
		},
	}
	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	for _, change := range plan.Changes {
		if change.Status == StatusPrune {
			t.Fatalf("normalized active target should not be stale: %#v", plan.Changes)
		}
	}
	if item := plan.NextLockItem("sample", "foo.txt"); item.TargetPath != "foo.txt" {
		t.Fatalf("expected normalized active lock target, got %#v", item)
	}
}

func TestBuildPlanRejectsReservedTargetPaths(t *testing.T) {
	for name, tc := range map[string]struct {
		targetConfig string
		manifest     string
	}{
		"source path default target config": {
			targetConfig: "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n",
			manifest:     "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: .driftline-target.yaml\n",
		},
		"target config override lock": {
			targetConfig: "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      sample: driftline-lock.yaml\n",
			manifest:     "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n",
		},
		"normalized source path default target config": {
			targetConfig: "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n",
			manifest:     "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: ./.driftline-target.yaml\n",
		},
		"normalized target config override lock": {
			targetConfig: "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      sample: ./driftline-lock.yaml\n",
			manifest:     "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, ".driftline-target.yaml", tc.targetConfig)
			client := fakeSourceClient{
				refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
				files: map[string][]byte{
					"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte(tc.manifest),
					"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("hello\n"),
				},
			}
			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.Contains(err.Error(), "reserved target path") {
				t.Fatalf("expected reserved target path error, got %v", err)
			}
		})
	}
}

func TestBuildPlanDropsStaleLockEntryWhenTargetIsActiveAgain(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, ".driftline-target.yaml", "version: 2\nsource:\n  repository: y-writings/new-source\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      sample: same.txt\n")
	writePlanFile(t, targetDir, "same.txt", "old\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/old-source\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: old-id\n    target_path: same.txt\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/new-source@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/new-source@0123456789abcdef0123456789abcdef01234567:.driftline-source.yaml": []byte("version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n"),
			"y-writings/new-source@0123456789abcdef0123456789abcdef01234567:sample.txt":             []byte("new\n"),
		},
	}
	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	for _, item := range plan.NextLock.Files {
		if item.ID == "old-id" {
			t.Fatalf("old lock entry for active target should be dropped: %#v", plan.NextLock.Files)
		}
	}
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

func countChanges(plan Plan, status Status, id string) int {
	count := 0
	for _, change := range plan.Changes {
		if change.Status == status && change.ID == id {
			count++
		}
	}
	return count
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
