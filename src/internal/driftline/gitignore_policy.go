package driftline

import "fmt"

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
