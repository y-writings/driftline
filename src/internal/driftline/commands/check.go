package commands

import (
	"fmt"
	"io"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runCheck(opts driftline.Options, stdout io.Writer) error {
	_, _, changes, err := driftline.BuildPlan(opts)
	if err != nil {
		return err
	}
	printChanges(stdout, changes)
	if driftline.HasDrift(changes) {
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
