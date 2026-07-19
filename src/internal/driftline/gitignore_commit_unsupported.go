//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"fmt"
	"runtime"
)

func validateAtomicGitIgnoreReplacement() error {
	return fmt.Errorf("atomic %s replacement is unsupported on %s", GitIgnorePath, runtime.GOOS)
}

func commitAtomicGitIgnoreReplacement(string, string) error {
	return validateAtomicGitIgnoreReplacement()
}
