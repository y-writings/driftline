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
		fmt.Fprintf(w, "%s %s: %s\n", change.Status, change.ID, change.Reason)
	}
	if !driftline.HasDrift(changes) {
		fmt.Fprintln(w, "synced")
	}
}
