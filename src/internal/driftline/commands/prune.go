package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runPrune(opts driftline.Options, stdout io.Writer) error {
	_, lock, changes, err := driftline.BuildPlan(opts)
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(changes) {
		switch change.Status {
		case driftline.StatusPrune:
			if err := os.Remove(change.TargetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("prune %s: %w", change.ID, err)
			}
			delete(lock.Files, change.ID)
			fmt.Fprintf(stdout, "prune %s\n", change.ID)
		case driftline.StatusConflict:
			fmt.Fprintf(stdout, "conflict %s: %s\n", change.ID, change.Reason)
		}
	}
	if err := driftline.WriteLock(filepath.Join(opts.TargetDir, opts.LockPath), lock); err != nil {
		return err
	}
	return nil
}
