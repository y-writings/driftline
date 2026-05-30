package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runPrune(source driftline.SourceClient, opts TargetOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source})
	if err != nil {
		return err
	}
	if !plan.HadLock {
		fmt.Fprintln(stdout, "nothing to prune")
		return nil
	}
	pruned := false
	conflicted := false
	next := plan.Lock
	next.Files = append([]driftline.LockItem(nil), plan.Lock.Files...)
	for _, change := range sortedChanges(plan.Changes) {
		switch change.Status {
		case driftline.StatusPrune:
			if err := os.Remove(change.TargetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("prune %s: %w", change.ID, err)
			}
			next.Files = removeLockItem(next.Files, change.ID, change.Target)
			fmt.Fprintf(stdout, "prune %s: %s\n", change.ID, change.Reason)
			pruned = true
		case driftline.StatusConflict:
			fmt.Fprintf(stdout, "conflict %s: %s\n", change.ID, change.Reason)
			conflicted = true
		}
	}
	if pruned {
		if err := driftline.WriteLock(filepath.Join(opts.TargetDir, driftline.LockFilePath), next); err != nil {
			return err
		}
	}
	if !pruned && !conflicted {
		fmt.Fprintln(stdout, "nothing to prune")
	}
	if conflicted {
		return errDrift
	}
	return nil
}

func removeLockItem(items []driftline.LockItem, id string, target string) []driftline.LockItem {
	out := items[:0]
	for _, item := range items {
		if item.ID == id && item.Target == target {
			continue
		}
		out = append(out, item)
	}
	return out
}
