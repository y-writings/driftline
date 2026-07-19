//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import (
	"io"
	"os"
	"syscall"
)

func readRegularFileNoFollow(path string) ([]byte, os.FileMode, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errOpenedTargetNotRegular
	}
	data, err := io.ReadAll(file)
	return data, info.Mode().Perm(), err
}
