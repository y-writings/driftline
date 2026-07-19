//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadRegularFileNoFollowRejectsFinalSymlink(t *testing.T) {
	root := t.TempDir()
	externalPath := filepath.Join(root, "external")
	if err := os.WriteFile(externalPath, []byte("external bytes must not be read\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, GitIgnorePath)
	if err := os.Symlink(externalPath, targetPath); err != nil {
		t.Fatal(err)
	}

	got, err := readRegularFileNoFollow(targetPath)
	if err == nil {
		t.Fatalf("expected final symlink rejection, read %q", got)
	}
	if !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("expected no-follow symlink error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("final symlink bytes must not be read: %q", got)
	}
}

func TestReadRegularFileNoFollowRejectsFIFO(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), GitIgnorePath)
	if err := syscall.Mkfifo(targetPath, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readRegularFileNoFollow(targetPath)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected FIFO regular-file error, got bytes %q and error %v", got, err)
	}
	if len(got) != 0 {
		t.Fatalf("FIFO bytes must not be read: %q", got)
	}
}

func TestBuildPlanRejectsFIFOGitIgnoreWithActiveConfig(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	setPlanGitIgnoreFIFO(t, targetDir)
	client := newPlanSourceClient(`version = 2

[gitignore]
entries = [".env"]
`, nil)

	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), GitIgnorePath) || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected .gitignore regular-file error, got %v", err)
	}
}

func TestBuildPlanIgnoresFIFOGitIgnoreWithInactiveConfig(t *testing.T) {
	configs := []struct {
		name     string
		contract string
	}{
		{name: "absent", contract: "version = 2\n"},
		{name: "empty", contract: "version = 2\n\n[gitignore]\nentries = []\n"},
	}
	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			setPlanGitIgnoreFIFO(t, targetDir)

			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient(config.contract, nil)})
			if err != nil {
				t.Fatalf("inactive config should ignore FIFO .gitignore: %v", err)
			}
			if plan.GitIgnore != nil || plan.HasDrift() {
				t.Fatalf("inactive config should not plan FIFO .gitignore: %#v", plan)
			}
		})
	}
}

func setPlanGitIgnoreFIFO(t *testing.T, root string) {
	t.Helper()
	if err := syscall.Mkfifo(filepath.Join(root, GitIgnorePath), 0o600); err != nil {
		t.Fatal(err)
	}
}
