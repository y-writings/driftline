package commands

import (
	"fmt"
	"io"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runCheck(source driftline.SourceClient, opts TargetOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source})
	if err != nil {
		return err
	}
	printChanges(stdout, plan.Changes)
	if driftline.HasDrift(plan.Changes) {
		return errDrift
	}
	return nil
}

func printChanges(w io.Writer, changes []driftline.Change) {
	for _, change := range sortedChanges(changes) {
		if change.Status == driftline.StatusSynced {
			continue
		}
		printChange(w, change)
	}
	if !driftline.HasDrift(changes) {
		fmt.Fprintln(w, "synced")
	}
}

func printChange(w io.Writer, change driftline.Change) {
	if change.Status != driftline.StatusConflict {
		fmt.Fprintf(w, "%s %s: %s\n", change.Status, change.ID, change.Reason)
		return
	}
	fmt.Fprintf(w, "conflict %s: %s\n", change.ID, change.Reason)
	fmt.Fprintf(w, "  target: %s\n", change.Target)
	fmt.Fprintln(w, "  source mode: managed")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Choose one:")
	fmt.Fprintln(w, "  1. set another target path in .driftline/sync.toml")
	if change.ForceAllowed {
		fmt.Fprintln(w, "  2. move the existing target file")
		fmt.Fprintf(w, "  3. rerun with --force %s to overwrite\n", change.ID)
		return
	}
	fmt.Fprintln(w, "  2. remove or change the conflicting filesystem path or managed entry")
}
