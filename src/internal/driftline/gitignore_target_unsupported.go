//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"fmt"
	"os"
	"runtime"
)

func readRegularFileNoFollow(string) ([]byte, os.FileMode, error) {
	return nil, 0, fmt.Errorf("safe .gitignore target reads are unsupported on %s", runtime.GOOS)
}
