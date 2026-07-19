//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import "os"

func validateAtomicGitIgnoreReplacement() error {
	return nil
}

func commitAtomicGitIgnoreReplacement(tempPath, targetPath string) error {
	return os.Rename(tempPath, targetPath)
}
