package commands

import (
	"io"

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
	if err := (driftline.TargetRepository{Root: opts.TargetDir}).Apply(plan); err != nil {
		return err
	}
	printChanges(stdout, plan.Changes)
	return nil
}
