package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/y-writings/driftline/src/internal/driftline"
)

type commandFakeSourceClient struct {
	defaultRef    string
	defaultCommit string
	refs          map[string]string
	files         map[string][]byte
}

func (f commandFakeSourceClient) ResolveDefaultRef(repository string) (string, string, error) {
	return f.defaultRef, f.defaultCommit, nil
}

func (f commandFakeSourceClient) ResolveRef(repository string, ref string) (string, error) {
	commit, ok := f.refs[repository+"@"+ref]
	if !ok {
		return "", os.ErrNotExist
	}
	return commit, nil
}

func (f commandFakeSourceClient) ReadFile(repository string, commit string, path string) ([]byte, error) {
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestInitCreatesTargetConfigFromSourceManifest(t *testing.T) {
	targetDir := t.TempDir()
	client := commandFakeSourceClient{
		defaultRef:    "main",
		defaultCommit: "0123456789abcdef0123456789abcdef01234567",
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: example\n    source: templates/example.txt\n    target: example.txt\n  - id: local-config\n    source: templates/config.local\n    target: config.local\n    if_not_exists: true\n"),
		},
	}

	var stdout, stderr bytes.Buffer
	runner := Runner{Source: client}
	err := runner.Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
	}

	got := readFile(t, targetDir, "driftline.yaml")
	for _, want := range []string{"version: 1", "repository: y-writings/source-repo", "ref: main", "id: example", "target: example.txt", "id: local-config", "if_not_exists: true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "gitignore") {
		t.Fatalf("target config must not copy gitignore:\n%s", got)
	}
	if !strings.Contains(stdout.String(), "created driftline.yaml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestInitRefPreservesInputRef(t *testing.T) {
	targetDir := t.TempDir()
	client := commandFakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@feature/foo": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		files: map[string][]byte{"y-writings/source-repo@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:driftline.yaml": []byte("version: 1\nfiles: []\n")},
	}
	var stdout, stderr bytes.Buffer
	runner := Runner{Source: client}
	if err := runner.Run([]string{"init", "y-writings/source-repo", "--ref", "feature/foo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if got := readFile(t, targetDir, "driftline.yaml"); !strings.Contains(got, "ref: feature/foo") {
		t.Fatalf("expected input ref to be preserved:\n%s", got)
	}
}

func TestInitRefusesExistingConfigOrLock(t *testing.T) {
	for name, file := range map[string]string{"config": "driftline.yaml", "lock": "driftline-lock.yaml"} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			writeFile(t, targetDir, file, "existing\n")
			client := commandFakeSourceClient{defaultRef: "main", defaultCommit: "0123456789abcdef0123456789abcdef01234567"}
			var stdout, stderr bytes.Buffer
			runner := Runner{Source: client}
			err := runner.Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected init to fail")
			}
		})
	}
}

func TestParseOptionsAcceptsStandardFlagForms(t *testing.T) {
	for name, args := range map[string][]string{
		"double dash equals": {"--target-dir=/tmp/target"},
		"single dash space":  {"-target-dir", "/tmp/target"},
		"single dash equals": {"-target-dir=/tmp/target"},
	} {
		t.Run(name, func(t *testing.T) {
			opts, err := parseTargetOptions(args)
			if err != nil {
				t.Fatalf("parse target options failed: %v", err)
			}
			if opts.TargetDir != "/tmp/target" {
				t.Fatalf("unexpected target dir: %q", opts.TargetDir)
			}
		})
	}

	initOpts, err := parseInitOptions([]string{"y-writings/source-repo", "--ref=feature/foo", "-target-dir=/tmp/target"})
	if err != nil {
		t.Fatalf("parse init options failed: %v", err)
	}
	if initOpts.Repository != "y-writings/source-repo" || initOpts.Ref != "feature/foo" || initOpts.TargetDir != "/tmp/target" {
		t.Fatalf("unexpected init options: %#v", initOpts)
	}
}

func TestHelpShowsNewCommandsAndGitHubToken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := Runner{Source: commandFakeSourceClient{}}
	if err := runner.Run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	for _, want := range []string{"driftline init owner/repo", "check", "diff", "update", "prune", "GITHUB_TOKEN"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
	for _, removed := range []string{removedSourceDirFlag(), removedManifestFlag(), removedLockFlag(), " pull "} {
		if strings.Contains(stdout.String(), removed) {
			t.Fatalf("help still mentions removed surface %q:\n%s", removed, stdout.String())
		}
	}
}

func TestCheckReportsMissingLockAndUpdateCreatesIt(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	client := commandFakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\ngitignore:\n  - .cache/tool\nfiles:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:sample.txt":     []byte("hello\n"),
		},
	}
	runner := Runner{Source: client}

	var stdout, stderr bytes.Buffer
	err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected check drift")
	}
	if !strings.Contains(stdout.String(), "update lock: lock file is missing") || !strings.Contains(stdout.String(), "add sample") {
		t.Fatalf("unexpected check output: %q", stdout.String())
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, "sample.txt"); got != "hello\n" {
		t.Fatalf("unexpected copied file: %q", got)
	}
	lock := readFile(t, targetDir, "driftline-lock.yaml")
	for _, want := range []string{"version: 1", "repository: y-writings/source-repo", "ref: main", "commit: 0123456789abcdef0123456789abcdef01234567", "target_sha256:"} {
		if !strings.Contains(lock, want) {
			t.Fatalf("lock missing %q:\n%s", want, lock)
		}
	}
	gitignore := readFile(t, targetDir, ".gitignore")
	if !strings.Contains(gitignore, ".cache/tool") {
		t.Fatalf("expected gitignore entry, got %q", gitignore)
	}
}

func TestUpdatePreservesIfNotExistsLocalEdits(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: local-config\n")
	writeFile(t, targetDir, "config.local", "initial-local\n")
	client := commandFakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles:\n  - id: local-config\n    source: config.local\n    target: config.local\n    if_not_exists: true\n"),
			"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:config.local":   []byte("from-source\n"),
		},
	}
	runner := Runner{Source: client}
	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	writeFile(t, targetDir, "config.local", "edited-local\n")
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	if got := readFile(t, targetDir, "config.local"); got != "edited-local\n" {
		t.Fatalf("expected local edit to remain, got %q", got)
	}
}

func TestPruneFailsWhenStaleFileHasLocalChanges(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles: []\n")
	writeFile(t, targetDir, "old.txt", "changed\n")
	writeFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: old\n    target: old.txt\n    source_sha256: 0000\n    target_sha256: 0000\n")
	client := commandFakeSourceClient{
		refs:  map[string]string{"y-writings/source-repo@main": "0123456789abcdef0123456789abcdef01234567"},
		files: map[string][]byte{"y-writings/source-repo@0123456789abcdef0123456789abcdef01234567:driftline.yaml": []byte("version: 1\nfiles: []\n")},
	}
	var stdout, stderr bytes.Buffer
	err := Runner{Source: client}.Run([]string{"prune", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected prune conflict failure")
	}
	if !strings.Contains(stdout.String(), "conflict old") {
		t.Fatalf("expected conflict output, got %q", stdout.String())
	}
	if got := readFile(t, targetDir, "old.txt"); got != "changed\n" {
		t.Fatalf("expected stale file to remain, got %q", got)
	}
}

func TestPruneDoesNotAdvanceActiveLockEntries(t *testing.T) {
	targetDir := t.TempDir()
	oldCommit := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	newCommit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	activeHash := driftline.HashBytes([]byte("old\n"))
	staleHash := driftline.HashBytes([]byte("stale\n"))
	writeFile(t, targetDir, "driftline.yaml", "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n")
	writeFile(t, targetDir, "sample.txt", "old\n")
	writeFile(t, targetDir, "old.txt", "stale\n")
	writeFile(t, targetDir, "driftline-lock.yaml", "version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: "+oldCommit+"\nfiles:\n  - id: sample\n    target: sample.txt\n    source_sha256: "+activeHash+"\n    target_sha256: "+activeHash+"\n  - id: old\n    target: old.txt\n    source_sha256: "+staleHash+"\n    target_sha256: "+staleHash+"\n")
	client := commandFakeSourceClient{
		refs: map[string]string{"y-writings/source-repo@main": newCommit},
		files: map[string][]byte{
			"y-writings/source-repo@" + newCommit + ":driftline.yaml": []byte("version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n"),
			"y-writings/source-repo@" + newCommit + ":sample.txt":     []byte("new\n"),
		},
	}

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"prune", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old.txt to be pruned, stat err=%v", err)
	}
	lock := readFile(t, targetDir, "driftline-lock.yaml")
	if strings.Contains(lock, newCommit) {
		t.Fatalf("prune must not advance active lock commit:\n%s", lock)
	}
	if !strings.Contains(lock, "commit: "+oldCommit) || !strings.Contains(lock, "source_sha256: "+activeHash) {
		t.Fatalf("prune must preserve active lock metadata:\n%s", lock)
	}
	if strings.Contains(lock, "target: old.txt") {
		t.Fatalf("prune must remove stale lock entry:\n%s", lock)
	}
}

func TestRemovedLocalSourceOptionsAndPullFail(t *testing.T) {
	runner := Runner{Source: commandFakeSourceClient{}}
	for _, args := range [][]string{
		{"pull"},
		{"check", removedSourceDirFlag(), "../source"},
		{"check", removedManifestFlag(), "custom.yaml"},
		{"check", removedLockFlag(), "custom-lock.yaml"},
		{"check", removedRepositoryFlag(), "y-writings/source-repo"},
		{"check", "--ref", "main"},
	} {
		var stdout, stderr bytes.Buffer
		if err := runner.Run(args, &stdout, &stderr); err == nil {
			t.Fatalf("expected args to fail: %v", args)
		}
	}
}

func removedSourceDirFlag() string {
	return "--source" + "-dir"
}

func removedManifestFlag() string {
	return "--mani" + "fest"
}

func removedLockFlag() string {
	return "--lo" + "ck"
}

func removedRepositoryFlag() string {
	return "--repo" + "sitory"
}

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
