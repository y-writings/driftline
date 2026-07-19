package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func TestPrintGitIgnoreDiffUsesPlannedSnapshotsInsteadOfLiveTarget(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, targetPath string)
	}{
		{
			name: "changed",
			mutate: func(t *testing.T, targetPath string) {
				if err := os.WriteFile(targetPath, []byte("live changed bytes\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "disappeared",
			mutate: func(t *testing.T, targetPath string) {
				if err := os.Remove(targetPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "replaced by symlink",
			mutate: func(t *testing.T, targetPath string) {
				if err := os.Remove(targetPath); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("external bytes\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, targetPath); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			targetPath := filepath.Join(root, driftline.GitIgnorePath)
			if err := os.WriteFile(targetPath, []byte("planned old bytes\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			tt.mutate(t, targetPath)

			var output bytes.Buffer
			err := printGitIgnoreDiff(
				&output,
				[]byte("planned old bytes\n"),
				[]byte("planned new bytes\n"),
				false,
			)
			if err != nil {
				t.Fatalf("print planned Gitignore diff: %v", err)
			}
			if !strings.Contains(output.String(), "-planned old bytes\n") || !strings.Contains(output.String(), "+planned new bytes\n") {
				t.Fatalf("diff did not use planned snapshots:\n%s", output.String())
			}
			for _, forbidden := range []string{"live changed bytes", "external bytes", targetPath} {
				if strings.Contains(output.String(), forbidden) {
					t.Fatalf("diff used live target content or path %q:\n%s", forbidden, output.String())
				}
			}
			assertStableGitIgnoreDiffPaths(t, output.String(), root, false)
		})
	}
}

func TestPrintGitIgnoreDiffPreservesHeaderLikeContent(t *testing.T) {
	var output bytes.Buffer
	err := printGitIgnoreDiff(&output, []byte("-- a/original\n"), []byte("++ b/desired\n"), false)
	if err != nil {
		t.Fatalf("print planned Gitignore diff: %v", err)
	}
	for _, want := range []string{"--- a/original\n", "+++ b/desired\n"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("diff corrupted header-like content %q:\n%s", want, output.String())
		}
	}
}
