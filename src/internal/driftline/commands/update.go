package commands

import (
	"errors"
	"io"
	"os"
	"path/filepath"

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
	commitConfig, cleanupConfig, err := driftline.PrepareTargetConfigWrite(filepath.Join(opts.TargetDir, driftline.TargetConfigPath), plan.NextConfig)
	if err != nil {
		return err
	}
	defer cleanupConfig()
	for _, change := range sortedChanges(plan.Changes) {
		if change.Status == driftline.StatusRemove && change.DeletesTarget {
			if err := os.Remove(change.TargetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if (change.Status == driftline.StatusAdd || change.Status == driftline.StatusUpdate) && change.WritesTarget {
			if err := driftline.WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if err := commitConfig(); err != nil {
		return err
	}
	printChanges(stdout, plan.Changes)
	return nil
}
