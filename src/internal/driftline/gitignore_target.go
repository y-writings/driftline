package driftline

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errOpenedTargetNotRegular = errors.New("opened target is not a regular file")

func planGitIgnoreSectionChange(targetDir string, repository string, config *ContractGitIgnore, replaceAfterManagedDelete bool) (*GitIgnoreSectionChange, error) {
	targetPath, err := PathWithin(targetDir, GitIgnorePath, GitIgnorePath+" target")
	if err != nil {
		return nil, err
	}

	active := config != nil && len(config.Entries) > 0
	targetMissing := false
	info, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		targetMissing = true
	} else if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", GitIgnorePath, err)
	} else if !info.Mode().IsRegular() {
		if active {
			return nil, fmt.Errorf("%s must be a regular file when gitignore entries are configured", GitIgnorePath)
		}
		return nil, nil
	}

	if targetMissing && !active {
		return nil, nil
	}

	var original []byte
	if !targetMissing {
		original, _, err = readRegularFileNoFollow(targetPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", GitIgnorePath, err)
		}
	}

	logicalCurrent := original
	logicalTargetMissing := targetMissing
	if replaceAfterManagedDelete {
		logicalCurrent = nil
		logicalTargetMissing = true
	}
	transformed, err := transformGitIgnoreSection(logicalCurrent, logicalTargetMissing, repository, config)
	if err != nil {
		return nil, err
	}
	if !transformed.Changed {
		return nil, nil
	}
	return &GitIgnoreSectionChange{
		Status:        transformed.Status,
		Reason:        transformed.Reason,
		TargetPath:    targetPath,
		TargetMissing: targetMissing,
		OriginalBytes: original,
		DesiredBytes:  transformed.DesiredBytes,
	}, nil
}

func PrepareGitIgnoreRewrite(change GitIgnoreSectionChange) (commit, cleanup func() error, err error) {
	mode := os.FileMode(0o644)
	if change.TargetMissing {
		_, err := os.Lstat(change.TargetPath)
		if err == nil {
			return nil, nil, staleGitIgnorePlanError("target appeared", nil)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, staleGitIgnorePlanError("inspect target", err)
		}
	} else {
		current, currentMode, err := readRegularFileNoFollow(change.TargetPath)
		if err != nil {
			return nil, nil, staleGitIgnorePlanError("read target", err)
		}
		if !bytes.Equal(current, change.OriginalBytes) {
			return nil, nil, staleGitIgnorePlanError("target content changed", nil)
		}
		mode = currentMode
	}

	temp, err := createGitIgnoreTemp(filepath.Dir(change.TargetPath))
	if err != nil {
		return nil, nil, fmt.Errorf("create %s temp file: %w", GitIgnorePath, err)
	}
	tempName := temp.Name()
	cleanup = func() error {
		err := os.Remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	fail := func(err error) (func() error, func() error, error) {
		temp.Close()
		_ = cleanup()
		return nil, nil, err
	}

	if !change.TargetMissing {
		if err := temp.Chmod(mode); err != nil {
			return fail(fmt.Errorf("chmod %s temp file: %w", GitIgnorePath, err))
		}
	}
	if _, err := temp.Write(change.DesiredBytes); err != nil {
		return fail(fmt.Errorf("write %s temp file: %w", GitIgnorePath, err))
	}
	if err := temp.Close(); err != nil {
		_ = cleanup()
		return nil, nil, fmt.Errorf("close %s temp file: %w", GitIgnorePath, err)
	}

	commit = func() error {
		if err := os.Rename(tempName, change.TargetPath); err != nil {
			return fmt.Errorf("commit %s rewrite: %w", GitIgnorePath, err)
		}
		return nil
	}
	return commit, cleanup, nil
}

func createGitIgnoreTemp(dir string) (*os.File, error) {
	for range 100 {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, fmt.Sprintf(".gitignore-%x.tmp", suffix))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, fmt.Errorf("could not allocate a unique temp file")
}

func staleGitIgnorePlanError(reason string, cause error) error {
	if cause != nil {
		return fmt.Errorf("stale %s plan: %s: %w", GitIgnorePath, reason, cause)
	}
	return fmt.Errorf("stale %s plan: %s", GitIgnorePath, reason)
}

func validateManagedGitIgnoreTarget(config *ContractGitIgnore, file resolvedManagedFile) error {
	if config == nil {
		return nil
	}
	if file.target == GitIgnorePath {
		return fmt.Errorf("Contract file %q cannot manage %s while gitignore is configured", file.Key, GitIgnorePath)
	}
	if len(config.Entries) > 0 && isPathAncestor(GitIgnorePath, file.target) {
		return fmt.Errorf("Contract file %q target %q cannot be below %s while gitignore entries are configured", file.Key, file.target, GitIgnorePath)
	}
	return nil
}
