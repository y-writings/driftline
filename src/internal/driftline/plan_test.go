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
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: config/sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":     []byte("hello\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	assertPlanHasChange(t, plan, StatusUpdate, "lock", "lock file is missing")
	assertPlanHasChange(t, plan, StatusAdd, "sample", "target file is missing")
}

func TestBuildPlanPreservesIfNotExistsTargetHashAcrossUpdates(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: local-config\n")
	writePlanFile(t, targetDir, "config.local", "edited-local\n")
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: local-config\n    target: config.local\n    source_sha256: old-source\n    target_sha256: locked-target\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml":         []byte("version: 1\nfiles:\n  - id: local-config\n    source: templates/config.local\n    target: config.local\n    if_not_exists: true\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:templates/config.local": []byte("from-source\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	item := plan.NextLockItem("local-config", "config.local")
	if item.TargetSHA256 != "locked-target" {
		t.Fatalf("target hash should remain locked, got %#v", item)
	}
	assertPlanHasChange(t, plan, StatusUpdate, "local-config", "target preserved because if_not_exists is enabled")
}

func TestBuildPlanLeavesOldTargetAsPruneCandidateWhenDefaultTargetChanges(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	writePlanFile(t, targetDir, "old.txt", "old\n")
	oldHash := HashBytes([]byte("old\n"))
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: sample\n    target: old.txt\n    source_sha256: "+oldHash+"\n    target_sha256: "+oldHash+"\n")

	client := fakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: new.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":     []byte("new\n"),
		},
	}

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	assertPlanHasChange(t, plan, StatusAdd, "sample", "target file is missing")
	assertPlanHasChange(t, plan, StatusPrune, "sample", "target is no longer adopted")
}

func TestBuildPlanRejectsUnknownSourceID(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: missing\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles: []\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "unknown source file id") {
		t.Fatalf("expected unknown id error, got %v", err)
	}
}

func TestBuildPlanRejectsResolvedDuplicateTargets(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: first\n  - id: second\n")
	client := fakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: first\n    source: first.txt\n    target: same.txt\n  - id: second\n    source: second.txt\n    target: same.txt\n")},
	}
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("expected duplicate target error, got %v", err)
	}
}

func TestBuildPlanRejectsReservedTargetPaths(t *testing.T) {
	for name, tc := range map[string]struct {
		targetConfig string
		manifest     string
	}{
		"source manifest target config": {
			targetConfig: "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n",
			manifest:     "version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: driftline.yaml\n",
		},
		"target config override lock": {
			targetConfig: "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    target: driftline-lock.yaml\n",
			manifest:     "version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, "driftline.yaml", tc.targetConfig)
			client := fakeSourceClient{
				refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
				files: map[string][]byte{
					"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte(tc.manifest),
					"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":     []byte("hello\n"),
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
	writePlanFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/new-source\n  ref: main\nfiles:\n  - id: sample\n    target: same.txt\n")
	writePlanFile(t, targetDir, "same.txt", "old\n")
	oldHash := HashBytes([]byte("old\n"))
	writePlanFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/old-source\nref: main\ncommit: oldoldoldoldoldoldoldoldoldoldoldoldoldoldoldold\nfiles:\n  - id: old-id\n    target: same.txt\n    source_sha256: "+oldHash+"\n    target_sha256: "+oldHash+"\n")
	client := fakeSourceClient{
		refs: map[string]string{"y-writings/new-source@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/new-source@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: same.txt\n"),
			"y-writings/new-source@0123456789abcdef0123456789abcdef01234567:sample.txt":     []byte("new\n"),
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
