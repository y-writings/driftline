package commands

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/y-writings/templates/src/internal/templatesync"
)

func runUpdate(opts templatesync.Options, stdout io.Writer) error {
	manifest, lock, changes, err := templatesync.BuildPlan(opts)
	if err != nil {
		return err
	}
	if lock.Files == nil {
		lock.Files = map[string]templatesync.LockItem{}
	}
	if opts.Repository != "" {
		lock.Repository = opts.Repository
	}
	if opts.Ref != "" {
		lock.Ref = opts.Ref
	}
	sourceByID := map[string]templatesync.ManifestTemplate{}
	for _, item := range manifest.Template {
		sourceByID[item.ID] = item
	}
	for _, change := range sortedChanges(changes) {
		switch change.Status {
		case templatesync.StatusAdd, templatesync.StatusUpdate:
			if err := templatesync.CopyFile(change.SourcePath, change.TargetPath); err != nil {
				return fmt.Errorf("update %s: %w", change.ID, err)
			}
			item := sourceByID[change.ID]
			lock.Files[change.ID] = templatesync.LockItem{Target: item.Target, SourceSHA256: change.SourceHash}
			fmt.Fprintf(stdout, "%s %s -> %s\n", change.Status, item.Source, item.Target)
		case templatesync.StatusSynced:
			item := sourceByID[change.ID]
			lockHash := change.SourceHash
			if item.IfNotExists && change.CurrentHash != "" {
				if locked, ok := lock.Files[change.ID]; ok && locked.SourceSHA256 != "" && locked.Target == item.Target {
					lockHash = locked.SourceSHA256
				} else {
					lockHash = change.CurrentHash
				}
			}
			lock.Files[change.ID] = templatesync.LockItem{Target: item.Target, SourceSHA256: lockHash}
		case templatesync.StatusPrune, templatesync.StatusConflict:
			fmt.Fprintf(stdout, "skip %s %s: %s\n", change.Status, change.ID, change.Reason)
		}
	}
	if err := templatesync.EnsureGitIgnore(filepath.Join(opts.TargetDir, ".gitignore"), manifest.GitIgnore); err != nil {
		return fmt.Errorf("update .gitignore: %w", err)
	}
	if err := templatesync.WriteLock(filepath.Join(opts.TargetDir, opts.LockPath), lock); err != nil {
		return err
	}
	return nil
}
