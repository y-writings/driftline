//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"fmt"
	"runtime"
)

func readRegularFileNoFollow(string) ([]byte, error) {
	return nil, fmt.Errorf("safe .gitignore target reads are unsupported on %s", runtime.GOOS)
}
