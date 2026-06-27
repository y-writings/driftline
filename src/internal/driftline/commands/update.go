package commands

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

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
	var commitConfig func() error
	if hasTargetConfigChanges(plan.Changes) {
		commit, cleanup, err := driftline.PrepareTargetConfigWrite(filepath.Join(opts.TargetDir, driftline.TargetConfigPath), plan.NextConfig)
		if err != nil {
			return err
		}
		defer cleanup()
		commitConfig = commit
	}
	changes := sortedChanges(plan.Changes)
	for _, change := range changes {
		if change.Status == driftline.StatusRemove && change.DeletesTarget {
			if err := removeManagedTargetFile(change.TargetPath); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		if (change.Status == driftline.StatusAdd || change.Status == driftline.StatusUpdate) && change.WritesTarget {
			if err := driftline.WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if commitConfig != nil {
		if err := commitConfig(); err != nil {
			return err
		}
	}
	printChanges(stdout, plan.Changes)
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

func hasTargetConfigChanges(changes []driftline.Change) bool {
	for _, change := range changes {
		if change.Status == driftline.StatusTargetConfigAdd || change.Status == driftline.StatusTargetConfigRemove {
			return true
		}
	}
	return false
}
