package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDetectsMissingTarget(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n")
	writeFile(t, templateDir, "sample.txt", "hello\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"check", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected drift error")
	}
	if !strings.Contains(stdout.String(), "add sample") {
		t.Fatalf("expected add status, got %q", stdout.String())
	}
}

func TestRunAcceptsLeadingArgumentSeparator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run([]string{"--", "help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("help with argument separator failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "usage: template-sync") {
		t.Fatalf("expected usage output, got %q", stdout.String())
	}
}

func TestUpdateCopiesFilesAndWritesLock(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: config/sample.txt\n")
	writeFile(t, templateDir, "sample.txt", "hello\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"update",
		"--template-dir", templateDir,
		"--target-dir", targetDir,
		"--repository", "y-writings/templates",
		"--ref", "v2026.05.01",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("update failed: %v\nstderr: %s", err, stderr.String())
	}

	got := readFile(t, targetDir, "config/sample.txt")
	if got != "hello\n" {
		t.Fatalf("unexpected target content: %q", got)
	}
	lock := readFile(t, targetDir, ".template-sync.lock")
	for _, want := range []string{"repository: y-writings/templates", "ref: v2026.05.01", "sample:", "target: config/sample.txt", "source_sha256:"} {
		if !strings.Contains(lock, want) {
			t.Fatalf("lock file missing %q:\n%s", want, lock)
		}
	}
}

func TestUpdateCreatesNestedLockParents(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n")
	writeFile(t, templateDir, "sample.txt", "hello\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"update",
		"--template-dir", templateDir,
		"--target-dir", targetDir,
		"--lock", ".config/template-sync.lock",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("update failed: %v\nstderr: %s", err, stderr.String())
	}
	if lock := readFile(t, targetDir, ".config/template-sync.lock"); !strings.Contains(lock, "sample:") {
		t.Fatalf("nested lock was not written correctly:\n%s", lock)
	}
}

func TestRejectsManifestPathsThatEscapeRoots(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: ../outside.txt\n")
	writeFile(t, templateDir, "sample.txt", "hello\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected escaping target path to fail")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestRejectsLockTargetsThatEscapeRoots(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	writeFile(t, targetDir, ".template-sync.lock", "files:\n  old:\n    target: ../outside.txt\n    source_sha256: 0000\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected escaping lock target path to fail")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestPruneRemovesOnlyUnchangedStaleFiles(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	writeFile(t, targetDir, "old.txt", "old\n")
	hash := hashFile(t, filepath.Join(targetDir, "old.txt"))
	writeFile(t, targetDir, ".template-sync.lock", "files:\n  old:\n    target: old.txt\n    source_sha256: "+hash+"\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prune failed: %v\nstderr: %s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(targetDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected old.txt to be removed, stat err=%v", err)
	}
}

func TestPruneKeepsLocallyChangedStaleFiles(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	writeFile(t, targetDir, "old.txt", "changed\n")
	writeFile(t, targetDir, ".template-sync.lock", "files:\n  old:\n    target: old.txt\n    source_sha256: 0000\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prune failed: %v\nstderr: %s", err, stderr.String())
	}
	if got := readFile(t, targetDir, "old.txt"); got != "changed\n" {
		t.Fatalf("expected local file to remain, got %q", got)
	}
	if !strings.Contains(stdout.String(), "conflict old") {
		t.Fatalf("expected conflict output, got %q", stdout.String())
	}
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

func hashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestUpdateAddsConfiguredGitIgnoreEntries(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ngitignore:\n  - .gh\n  - .cache/tool\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n")
	writeFile(t, templateDir, "sample.txt", "hello\n")
	writeFile(t, targetDir, ".gitignore", "node_modules\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got := readFile(t, targetDir, ".gitignore")
	for _, want := range []string{"node_modules", ".gh", ".cache/tool"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected .gitignore to contain %q, got %q", want, got)
		}
	}
}

func TestIfNotExistsSkipsUpdateAndPrunesWhenUnchanged(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n    if_not_exists: true\n")
	writeFile(t, templateDir, "sample.txt", "from-template\n")
	writeFile(t, targetDir, "sample.txt", "local\n")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, "sample.txt"); got != "local\n" {
		t.Fatalf("expected existing file to remain unchanged, got %q", got)
	}
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	err = Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "sample.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected sample.txt to be pruned, stat err=%v", err)
	}
}

func TestIfNotExistsKeepsInitialLockHashAcrossUpdates(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n    if_not_exists: true\n")
	writeFile(t, templateDir, "sample.txt", "from-template\n")
	writeFile(t, targetDir, "sample.txt", "initial-local\n")

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	writeFile(t, targetDir, "sample.txt", "edited-local\n")
	if err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	if err := Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if got := readFile(t, targetDir, "sample.txt"); got != "edited-local\n" {
		t.Fatalf("expected edited file to remain, got %q", got)
	}
	if !strings.Contains(stdout.String(), "conflict sample") {
		t.Fatalf("expected conflict output, got %q", stdout.String())
	}
}

func TestIfNotExistsResetsLockHashWhenTargetPathChanges(t *testing.T) {
	templateDir := t.TempDir()
	targetDir := t.TempDir()
	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: old.txt\n    if_not_exists: true\n")
	writeFile(t, templateDir, "sample.txt", "from-template\n")
	writeFile(t, targetDir, "old.txt", "old-local\n")

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates:\n  - id: sample\n    source: sample.txt\n    target: new.txt\n    if_not_exists: true\n")
	writeFile(t, targetDir, "new.txt", "new-local\n")
	if err := Run([]string{"update", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	writeFile(t, templateDir, "templates.yaml", "version: 1\ntemplates: []\n")
	if err := Run([]string{"prune", "--template-dir", templateDir, "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected new.txt to be pruned, stat err=%v", err)
	}
}
