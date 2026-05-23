package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/y-writings/templates/src/internal/templatesync"
)

func runPrune(opts templatesync.Options, stdout io.Writer) error {
	_, lock, changes, err := templatesync.BuildPlan(opts)
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(changes) {
		switch change.Status {
		case templatesync.StatusPrune:
			if err := os.Remove(change.TargetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("prune %s: %w", change.ID, err)
			}
			delete(lock.Files, change.ID)
			fmt.Fprintf(stdout, "prune %s\n", change.ID)
		case templatesync.StatusConflict:
			fmt.Fprintf(stdout, "conflict %s: %s\n", change.ID, change.Reason)
		}
	}
	if err := templatesync.WriteLock(filepath.Join(opts.TargetDir, opts.LockPath), lock); err != nil {
		return err
	}
	return nil
}
