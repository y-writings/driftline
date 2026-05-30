package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runDiff(source driftline.SourceClient, opts TargetOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source})
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(plan.Changes) {
		switch {
		case (change.Status == driftline.StatusAdd || change.Status == driftline.StatusUpdate) && change.WritesTarget:
			if err := printBytesDiff(stdout, change.SourceBytes, change.TargetPath, change.Status == driftline.StatusAdd); err != nil {
				return err
			}
		case change.Status != driftline.StatusSynced:
			fmt.Fprintf(stdout, "%s %s: %s\n", change.Status, change.ID, change.Reason)
		}
	}
	if driftline.HasDrift(plan.Changes) {
		return errDrift
	}
	return nil
}

func printBytesDiff(w io.Writer, sourceBytes []byte, targetPath string, targetMissing bool) error {
	temp, err := os.CreateTemp("", "driftline-source-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(sourceBytes); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return printGitDiff(w, temp.Name(), targetPath, targetMissing)
}

func printGitDiff(w io.Writer, sourcePath, targetPath string, targetMissing bool) error {
	left := targetPath
	if targetMissing {
		left = os.DevNull
	}
	cmd := exec.Command("git", "diff", "--no-index", "--", left, sourcePath)
	data, err := cmd.CombinedOutput()
	if len(data) > 0 {
		fmt.Fprint(w, string(data))
	}
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("run git diff: %w", err)
}
