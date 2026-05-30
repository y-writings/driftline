package commands

import (
	"io"
	"path/filepath"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runUpdate(source driftline.SourceClient, opts TargetOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source})
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(plan.Changes) {
		if (change.Status == driftline.StatusAdd || change.Status == driftline.StatusUpdate) && change.WritesTarget {
			if err := driftline.WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if err := driftline.EnsureGitIgnore(filepath.Join(opts.TargetDir, ".gitignore"), plan.GitIgnore); err != nil {
		return err
	}
	if err := driftline.WriteLock(filepath.Join(opts.TargetDir, driftline.LockFilePath), plan.NextLock); err != nil {
		return err
	}
	printChanges(stdout, plan.Changes)
	return nil
}
