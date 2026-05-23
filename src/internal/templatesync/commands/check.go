package commands

import (
	"fmt"
	"io"

	"github.com/y-writings/templates/src/internal/templatesync"
)

func runCheck(opts templatesync.Options, stdout io.Writer) error {
	_, _, changes, err := templatesync.BuildPlan(opts)
	if err != nil {
		return err
	}
	printChanges(stdout, changes)
	if templatesync.HasDrift(changes) {
		return errDrift
	}
	return nil
}

func printChanges(w io.Writer, changes []templatesync.Change) {
	for _, change := range sortedChanges(changes) {
		if change.Status == templatesync.StatusSynced {
			continue
		}
		fmt.Fprintf(w, "%s %s: %s\n", change.Status, change.ID, change.Reason)
	}
	if !templatesync.HasDrift(changes) {
		fmt.Fprintln(w, "synced")
	}
}
