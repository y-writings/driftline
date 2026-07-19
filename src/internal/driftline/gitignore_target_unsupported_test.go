//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"strings"
	"testing"
)

func TestReadRegularFileNoFollowReportsUnsupportedPlatform(t *testing.T) {
	_, _, err := readRegularFileNoFollow(".gitignore")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported-platform error, got %v", err)
	}
}
