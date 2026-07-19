package driftline

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type TargetRepository struct {
	Root                               string
	validateAtomicGitIgnoreReplacement func() error
	prepareSyncManifestRewrite         func(string, SyncManifest) (func() error, func() error, error)
	prepareGitIgnoreRewrite            func(GitIgnoreSectionChange) (func() error, func() error, error)
}

func (r TargetRepository) Apply(plan Plan) (err error) {
	if plan.HasConflicts() {
		return fmt.Errorf("cannot apply conflicted sync plan")
	}
	if plan.GitIgnore != nil {
		validate := r.validateAtomicGitIgnoreReplacement
		if validate == nil {
			validate = validateAtomicGitIgnoreReplacement
		}
		if err := validate(); err != nil {
			return err
		}
	}
	root := r.Root
	if root == "" {
		root = "."
	}

	var commitSyncManifest func() error
	if planHasSyncManifestChanges(plan.Changes) {
		prepare := r.prepareSyncManifestRewrite
		if prepare == nil {
			prepare = PrepareSyncManifestRewrite
		}
		commit, cleanup, prepareErr := prepare(root, plan.NextSyncManifest)
		if prepareErr != nil {
			return prepareErr
		}
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("cleanup Sync manifest rewrite: %w", cleanupErr))
			}
		}()
		commitSyncManifest = commit
	}

	var commitGitIgnore func() error
	if plan.GitIgnore != nil {
		prepare := r.prepareGitIgnoreRewrite
		if prepare == nil {
			prepare = PrepareGitIgnoreRewrite
		}
		commit, cleanup, prepareErr := prepare(*plan.GitIgnore)
		if prepareErr != nil {
			return prepareErr
		}
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("cleanup %s rewrite: %w", GitIgnorePath, cleanupErr))
			}
		}()
		commitGitIgnore = commit
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
	if commitGitIgnore != nil {
		if err := commitGitIgnore(); err != nil {
			return err
		}
	}
	if commitSyncManifest != nil {
		if err := commitSyncManifest(); err != nil {
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

func planHasSyncManifestChanges(changes []Change) bool {
	for _, change := range changes {
		if change.Status == StatusSyncManifestAdd || change.Status == StatusSyncManifestRemove {
			return true
		}
	}
	return false
}
