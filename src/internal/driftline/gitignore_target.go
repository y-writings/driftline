package driftline

import (
	"errors"
	"fmt"
	"os"
)

var errOpenedTargetNotRegular = errors.New("opened target is not a regular file")

func planGitIgnoreSectionChange(targetDir string, repository string, config *ContractGitIgnore) (*GitIgnoreSectionChange, error) {
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
		original, err = readRegularFileNoFollow(targetPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", GitIgnorePath, err)
		}
	}

	transformed, err := transformGitIgnoreSection(original, targetMissing, repository, config)
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
