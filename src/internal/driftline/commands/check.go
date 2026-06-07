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
	sharedIDs := sharedChangeIDs(changes)
	for _, change := range sortedChanges(changes) {
		if change.Status == driftline.StatusSynced {
			continue
		}
		if _, ok := sharedIDs[change.ID]; ok && change.Target != "" {
			fmt.Fprintf(w, "%s %s %s: %s\n", change.Status, change.ID, change.Target, change.Reason)
			continue
		}
		fmt.Fprintf(w, "%s %s: %s\n", change.Status, change.ID, change.Reason)
	}
	if !driftline.HasDrift(changes) {
		fmt.Fprintln(w, "synced")
	}
}

func sharedChangeIDs(changes []driftline.Change) map[string]struct{} {
	counts := map[string]int{}
	for _, change := range changes {
		if change.Target == "" {
			continue
		}
		counts[change.ID]++
	}
	shared := map[string]struct{}{}
	for id, count := range counts {
		if count > 1 {
			shared[id] = struct{}{}
		}
	}
	return shared
}
