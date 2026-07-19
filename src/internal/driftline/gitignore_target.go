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

type gitIgnoreTempOperations struct {
	write  func(*os.File, []byte) error
	close  func(*os.File) error
	remove func(string) error
}

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
	return prepareGitIgnoreRewriteWithOperations(change, gitIgnoreTempOperations{})
}

func prepareGitIgnoreRewriteWithOperations(change GitIgnoreSectionChange, ops gitIgnoreTempOperations) (commit, cleanup func() error, err error) {
	if err := validateAtomicGitIgnoreReplacement(); err != nil {
		return nil, nil, err
	}
	if ops.write == nil {
		ops.write = func(file *os.File, data []byte) error {
			_, err := file.Write(data)
			return err
		}
	}
	if ops.close == nil {
		ops.close = (*os.File).Close
	}
	if ops.remove == nil {
		ops.remove = os.Remove
	}

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
		err := ops.remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return gitIgnoreTempOperationError("remove", err)
	}
	fail := func(primary error) (func() error, func() error, error) {
		closeErr := gitIgnoreTempOperationError("close", ops.close(temp))
		return nil, nil, errors.Join(primary, closeErr, cleanup())
	}

	if !change.TargetMissing {
		if err := temp.Chmod(mode); err != nil {
			return fail(fmt.Errorf("chmod %s temp file: %w", GitIgnorePath, err))
		}
	}
	if err := ops.write(temp, change.DesiredBytes); err != nil {
		return fail(fmt.Errorf("write %s temp file: %w", GitIgnorePath, err))
	}
	if err := ops.close(temp); err != nil {
		return nil, nil, errors.Join(gitIgnoreTempOperationError("close", err), cleanup())
	}

	commit = func() error {
		if err := commitAtomicGitIgnoreReplacement(tempName, change.TargetPath); err != nil {
			return fmt.Errorf("commit %s rewrite: %w", GitIgnorePath, err)
		}
		return nil
	}
	return commit, cleanup, nil
}

func gitIgnoreTempOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s temp file: %w", operation, GitIgnorePath, err)
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
