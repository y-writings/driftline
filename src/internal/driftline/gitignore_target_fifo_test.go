//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package driftline

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadRegularFileNoFollowRejectsFIFO(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, GitIgnorePath)
	setPlanGitIgnoreFIFO(t, root)

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
