//go:build windows

package driftline

import "fmt"

func validateAtomicGitIgnoreReplacement() error {
	return fmt.Errorf("atomic %s replacement is unsupported on windows", GitIgnorePath)
}

func commitAtomicGitIgnoreReplacement(string, string) error {
	return validateAtomicGitIgnoreReplacement()
}
