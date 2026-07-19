//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRegularFileNoFollowReportsUnsupportedPlatform(t *testing.T) {
	_, _, err := readRegularFileNoFollow(".gitignore")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported-platform error, got %v", err)
	}
}

func TestPrepareGitIgnoreRewriteReportsUnsupportedAtomicReplacement(t *testing.T) {
	if err := validateAtomicGitIgnoreReplacement(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected platform atomic replacement rejection, got %v", err)
	}

	_, _, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath:    filepath.Join(t.TempDir(), GitIgnorePath),
		TargetMissing: true,
		DesiredBytes:  []byte("desired\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "atomic .gitignore replacement is unsupported") {
		t.Fatalf("expected unsupported atomic replacement error, got %v", err)
	}
}
