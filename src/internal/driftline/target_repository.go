package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type TargetRepository struct {
	Root string
}

func (r TargetRepository) Apply(plan Plan) error {
	if plan.HasConflicts() {
		return fmt.Errorf("cannot apply conflicted sync plan")
	}
	root := r.Root
	if root == "" {
		root = "."
	}

	var commitConfig func() error
	if planHasTargetConfigChanges(plan.Changes) {
		commit, cleanup, err := PrepareTargetConfigWrite(filepath.Join(root, TargetConfigPath), plan.NextConfig)
		if err != nil {
			return err
		}
		defer cleanup()
		commitConfig = commit
	}

	changes := SortedChanges(plan.Changes)
	for _, change := range changes {
		if change.Status == StatusRemove && change.DeletesTarget {
			if err := removeManagedTargetFile(change.TargetPath); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		if (change.Status == StatusAdd || change.Status == StatusUpdate) && change.WritesTarget {
			if err := WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if commitConfig != nil {
		if err := commitConfig(); err != nil {
			return err
		}
	}
	return nil
}

func removeManagedTargetFile(targetPath string) error {
	info, err := os.Lstat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.Remove(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	return nil
}

func planHasTargetConfigChanges(changes []Change) bool {
	for _, change := range changes {
		if change.Status == StatusTargetConfigAdd || change.Status == StatusTargetConfigRemove {
			return true
		}
	}
	return false
}
