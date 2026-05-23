package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runDiff(opts driftline.Options, stdout io.Writer) error {
	_, _, changes, err := driftline.BuildPlan(opts)
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(changes) {
		switch change.Status {
		case driftline.StatusAdd, driftline.StatusUpdate:
			if err := printGitDiff(stdout, change.SourcePath, change.TargetPath, change.Status == driftline.StatusAdd); err != nil {
				return err
			}
		case driftline.StatusPrune, driftline.StatusConflict:
			fmt.Fprintf(stdout, "%s %s: %s\n", change.Status, change.ID, change.Reason)
		}
	}
	if driftline.HasDrift(changes) {
		return errDrift
	}
	return nil
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
