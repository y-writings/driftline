package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/y-writings/driftline/src/internal/driftline"
)

type commandFakeSourceClient struct {
	defaultRef    string
	defaultCommit string
	refs          map[string]string
	files         map[string][]byte
	readErr       error
}

type sourceAccessFailingClient struct{}

func (sourceAccessFailingClient) ResolveDefaultRef(repository string) (string, string, error) {
	return "", "", errors.New("source should not be accessed")
}

func (sourceAccessFailingClient) ResolveRef(repository string, ref string) (string, error) {
	return "", errors.New("source should not be accessed")
}

func (sourceAccessFailingClient) ReadFile(repository string, commit string, path string) ([]byte, error) {
	return nil, errors.New("source should not be accessed")
}

func (f commandFakeSourceClient) ResolveDefaultRef(repository string) (string, string, error) {
	return f.defaultRef, f.defaultCommit, nil
}

func (f commandFakeSourceClient) ResolveRef(repository string, ref string) (string, error) {
	commit, ok := f.refs[repository+"@"+ref]
	if !ok {
		return "", os.ErrNotExist
	}
	return commit, nil
}

func (f commandFakeSourceClient) ReadFile(repository string, commit string, path string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestInitReadsContractAndCreatesSyncManifest(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, ".mise/config.toml", "target-owned\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`, map[string]string{
		".github/workflows/ci.yaml":      "ci\n",
		".github/workflows/release.yaml": "release\n",
		".mise/config.toml":              "source-template\n",
	})

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
	}

	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	for _, want := range []string{"version = 2", `[source]`, `repository = "y-writings/source-repo"`, `[files.github-workflow]`, `ci = ".github/workflows/ci.yaml"`} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("generated Sync manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, removed := range []string{"release", "mise", "template", "path_overrides", "if_not_exists"} {
		if strings.Contains(manifest, removed) {
			t.Fatalf("Sync manifest contains non-managed or old field %q:\n%s", removed, manifest)
		}
	}
	if got := readFile(t, targetDir, ".github/workflows/release.yaml"); got != "release\n" {
		t.Fatalf("expected missing template to be placed, got %q", got)
	}
	if got := readFile(t, targetDir, ".mise/config.toml"); got != "target-owned\n" {
		t.Fatalf("existing template target must be skipped, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".github/workflows/ci.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init should not copy managed files, stat err=%v", err)
	}
	if got, want := stdout.String(), "created Sync manifest .driftline/sync.toml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567\n"; got != want {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestInitDoesNotReadOldRootContract(t *testing.T) {
	targetDir := t.TempDir()
	commit := "0123456789abcdef0123456789abcdef01234567"
	client := commandFakeSourceClient{
		defaultRef:    "main",
		defaultCommit: commit,
		refs:          map[string]string{"y-writings/source-repo@main": commit},
		files: map[string][]byte{
			"y-writings/source-repo@" + commit + ":.driftline-source.toml": []byte("version = 2\n"),
		},
	}

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil || !strings.HasPrefix(err.Error(), "Contract not found: .driftline/contract.toml: ") {
		t.Fatalf("expected canonical Contract error, got %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("old Contract fallback must not emit migration or warning output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	assertFileMissing(t, targetDir, driftline.SyncManifestPath)
	assertFileMissing(t, targetDir, ".driftline-target.toml")
}

func TestInitReportsContractReadFailure(t *testing.T) {
	targetDir := t.TempDir()
	providerErr := errors.New("provider unavailable")
	client := commandFakeSourceClient{
		defaultRef:    "main",
		defaultCommit: "0123456789abcdef0123456789abcdef01234567",
		readErr:       providerErr,
	}

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil || err.Error() != "read Contract .driftline/contract.toml: provider unavailable" {
		t.Fatalf("expected canonical Contract read error, got %v", err)
	}
	if !errors.Is(err, providerErr) {
		t.Fatalf("Contract read error should preserve provider cause: %v", err)
	}
}

func TestInitForceAdoptsExistingManagedRegularFile(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "target-owned\n")
	writeFile(t, targetDir, ".mise/config.toml", "target-template\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`, map[string]string{
		".github/workflows/release.yaml": "release\n",
		".mise/config.toml":              "source-template\n",
	})

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir, "--force"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("forced init failed: %v\nstderr: %s", err, stderr.String())
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("forced init must not overwrite existing managed target, got %q", got)
	}
	if got := readFile(t, targetDir, ".github/workflows/release.yaml"); got != "release\n" {
		t.Fatalf("expected missing template to be placed, got %q", got)
	}
	if got := readFile(t, targetDir, ".mise/config.toml"); got != "target-template\n" {
		t.Fatalf("existing template should be skipped, got %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if !strings.Contains(manifest, `[files.github-workflow]`) || !strings.Contains(manifest, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("Sync manifest should record adopted managed target:\n%s", manifest)
	}
	if strings.Contains(manifest, "force") || strings.Contains(manifest, "release") || strings.Contains(manifest, "mise") {
		t.Fatalf("Sync manifest should not persist force or template entries:\n%s", manifest)
	}
	if got, want := stdout.String(), "created Sync manifest .driftline/sync.toml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567\n"; got != want {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestInitRefPreservesInputRef(t *testing.T) {
	targetDir := t.TempDir()
	client := newCommandSourceClient("feature/foo", "version = 2\n", nil)
	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--ref", "feature/foo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); !strings.Contains(got, `ref = "feature/foo"`) {
		t.Fatalf("expected input ref to be preserved:\n%s", got)
	}
}

func TestInitFailsOnExistingSyncManifestBeforeSourceAccess(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, "existing\n")

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: sourceAccessFailingClient{}}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil || err.Error() != "Sync manifest already exists: .driftline/sync.toml" {
		t.Fatalf("expected Sync manifest error before source access, got %v", err)
	}
}

func TestInitRejectsUnsafeMetadataDirectoryBeforeSourceAccess(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, targetDir string) string
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, targetDir string) string {
				writeFile(t, targetDir, driftline.MetadataDirectoryPath, "not a directory\n")
				return ""
			},
		},
		{
			name: "live directory symlink",
			setup: func(t *testing.T, targetDir string) string {
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(targetDir, driftline.MetadataDirectoryPath)); err != nil {
					t.Fatal(err)
				}
				return outside
			},
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, targetDir string) string {
				outside := filepath.Join(t.TempDir(), "missing-metadata")
				if err := os.Symlink(outside, filepath.Join(targetDir, driftline.MetadataDirectoryPath)); err != nil {
					t.Fatal(err)
				}
				return outside
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			outside := tt.setup(t, targetDir)

			var stdout, stderr bytes.Buffer
			err := (Runner{Source: sourceAccessFailingClient{}}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
			if err == nil || err.Error() != "driftline metadata path is not a real directory: .driftline" {
				t.Fatalf("expected canonical metadata directory error before source access, got %v", err)
			}
			if strings.Contains(err.Error(), "source should not be accessed") {
				t.Fatalf("source was accessed before rejecting metadata directory: %v", err)
			}
			for _, oldArtifact := range []string{".driftline-target.toml", ".driftline-source.toml", "driftline-lock.yaml"} {
				if strings.Contains(err.Error(), oldArtifact) {
					t.Fatalf("metadata error names old artifact %q: %v", oldArtifact, err)
				}
			}
			if outside != "" {
				assertFileMissing(t, outside, "sync.toml")
			}
		})
	}
}

func TestInitRejectsUnsafeMetadataSyncManifestBeforeSourceAccess(t *testing.T) {
	for _, tt := range []struct {
		name        string
		setup       func(t *testing.T, targetDir string) string
		wantOutside string
	}{
		{
			name: "directory",
			setup: func(t *testing.T, targetDir string) string {
				if err := os.Mkdir(filepath.Join(targetDir, driftline.SyncManifestPath), 0o755); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
		{
			name: "live symlink",
			setup: func(t *testing.T, targetDir string) string {
				outsideTarget := filepath.Join(t.TempDir(), "outside-sync.toml")
				if err := os.WriteFile(outsideTarget, []byte("outside Sync sentinel\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outsideTarget, filepath.Join(targetDir, driftline.SyncManifestPath)); err != nil {
					t.Fatal(err)
				}
				return outsideTarget
			},
			wantOutside: "outside Sync sentinel\n",
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, targetDir string) string {
				outside := filepath.Join(t.TempDir(), "missing-sync.toml")
				if err := os.Symlink(outside, filepath.Join(targetDir, driftline.SyncManifestPath)); err != nil {
					t.Fatal(err)
				}
				return outside
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			if err := os.Mkdir(filepath.Join(targetDir, driftline.MetadataDirectoryPath), 0o755); err != nil {
				t.Fatal(err)
			}
			outsideTarget := tt.setup(t, targetDir)

			var stdout, stderr bytes.Buffer
			err := (Runner{Source: sourceAccessFailingClient{}}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
			if err == nil || err.Error() != "Sync manifest path is not a regular file: .driftline/sync.toml" {
				t.Fatalf("expected canonical Sync manifest error before source access, got %v", err)
			}
			if strings.Contains(err.Error(), "source should not be accessed") {
				t.Fatalf("source was accessed before rejecting Sync manifest path: %v", err)
			}
			for _, oldArtifact := range []string{".driftline-target.toml", ".driftline-source.toml", "driftline-lock.yaml"} {
				if strings.Contains(err.Error(), oldArtifact) {
					t.Fatalf("Sync manifest error names old artifact %q: %v", oldArtifact, err)
				}
			}
			if outsideTarget != "" {
				outsideBytes, err := os.ReadFile(outsideTarget)
				if tt.wantOutside != "" {
					if err != nil || string(outsideBytes) != tt.wantOutside {
						t.Fatalf("outside target changed: got %q, err=%v", outsideBytes, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("outside target must remain absent, read err=%v", err)
				}
			}
		})
	}
}

func TestInitFailsBeforeWritingWhenSyncManifestOrManagedTargetExists(t *testing.T) {
	for name, tt := range map[string]struct {
		setup        func(string)
		wantGuidance bool
	}{
		"Sync manifest exists": {
			setup: func(targetDir string) {
				writeFile(t, targetDir, driftline.SyncManifestPath, "existing\n")
			},
		},
		"managed target exists": {
			setup: func(targetDir string) {
				writeFile(t, targetDir, ".github/workflows/ci.yaml", "existing\n")
			},
			wantGuidance: true,
		},
		"managed target broken symlink exists": {
			setup: func(targetDir string) {
				linkPath := filepath.Join(targetDir, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), linkPath); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			tt.setup(targetDir)
			client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }
`, map[string]string{".github/workflows/release.yaml": "release\n"})
			var stdout, stderr bytes.Buffer
			err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected init to fail")
			}
			if tt.wantGuidance && !strings.Contains(err.Error(), "rerun with --force") {
				t.Fatalf("expected force guidance, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(targetDir, ".github/workflows/release.yaml")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("init must fail before placing templates, stat err=%v", err)
			}
		})
	}
}

func TestInitRejectsSourceFilesTargetingReservedPaths(t *testing.T) {
	for name, contract := range map[string]string{
		"managed metadata path": `version = 2

[files.driftline]
target = { path = ".driftline/example.toml", mode = "managed" }
`,
		"template metadata path": `version = 2

[files.driftline]
target = { path = ".driftline/example.toml", mode = "template" }
`,
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			client := newCommandSourceClient("main", contract, map[string]string{
				".driftline/example.toml": "template bytes\n",
			})
			var stdout, stderr bytes.Buffer

			err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)

			if err == nil || !strings.HasPrefix(err.Error(), "reserved driftline metadata path: .driftline/example.toml") {
				t.Fatalf("expected reserved target path error, got %v", err)
			}
			assertFileMissing(t, targetDir, driftline.SyncManifestPath)
		})
	}
}

func TestInitPlacesTemplateGitIgnoreWithoutGeneratedSection(t *testing.T) {
	targetDir := t.TempDir()
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env", "/dist/"]

[files.repository]
ignore = { path = ".gitignore", mode = "template" }
`, map[string]string{".gitignore": "# source template\n.env.local\n"})

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
	}
	if got, want := readFile(t, targetDir, ".gitignore"), "# source template\n.env.local\n"; got != want {
		t.Fatalf("init should place only Template bytes: got %q, want %q", got, want)
	}
	if strings.Contains(readFile(t, targetDir, ".gitignore"), "# start driftline") {
		t.Fatal("init must not add the generated Gitignore section")
	}
}

func TestInitDoesNotInspectExistingGitIgnoreWithoutTemplateOperation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, targetDir string)
		check func(t *testing.T, targetDir string)
	}{
		{
			name: "malformed marker",
			setup: func(t *testing.T, targetDir string) {
				writeFile(t, targetDir, ".gitignore", "# start driftline from old/repo/.driftline/contract.toml\n.env\n")
			},
			check: func(t *testing.T, targetDir string) {
				if got, want := readFile(t, targetDir, ".gitignore"), "# start driftline from old/repo/.driftline/contract.toml\n.env\n"; got != want {
					t.Fatalf("init changed malformed target: got %q, want %q", got, want)
				}
			},
		},
		{
			name: "nonregular target",
			setup: func(t *testing.T, targetDir string) {
				if err := os.Mkdir(filepath.Join(targetDir, ".gitignore"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, targetDir string) {
				info, err := os.Lstat(filepath.Join(targetDir, ".gitignore"))
				if err != nil || !info.IsDir() {
					t.Fatalf("init changed nonregular target: info=%#v err=%v", info, err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			tt.setup(t, targetDir)
			client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]
`, nil)

			var stdout, stderr bytes.Buffer
			if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
				t.Fatalf("init should validate Contract without inspecting .gitignore: %v", err)
			}
			tt.check(t, targetDir)
		})
	}
}

func TestHelpReportsCurrentCommandsAndArtifacts(t *testing.T) {
	runner := Runner{Source: commandFakeSourceClient{}}
	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	wantHelp := `usage: driftline <command> [options]

commands:
  init owner/repo  create .driftline/sync.toml from a GitHub Source Repository
  check            check Target Repository state against the Contract
  diff             show planned content changes
  update           reconcile Managed files, Gitignore section, and Sync manifest

examples:
  driftline init owner/repo
  driftline init owner/repo --force
  driftline init owner/repo --ref main --target-dir .
  driftline check --target-dir .
  driftline update --force github-workflow.ci

options:
  --target-dir string  target repository directory (default ".")
  --ref string         init-only ref to preserve in .driftline/sync.toml
  --force              init-only adopt existing regular Managed target files
  --force group.file   update-only one-time conflict overwrite

authentication:
  set GITHUB_TOKEN for private repositories or higher rate limits
`
	if stdout.String() != wantHelp {
		t.Fatalf("unexpected help output:\n%s", stdout.String())
	}
	for _, stale := range []string{".driftline-source.toml", ".driftline-target.toml", "driftline-lock", "path_overrides", "if_not_exists", "prune", "\n  sync "} {
		if strings.Contains(stdout.String(), stale) {
			t.Fatalf("help still mentions removed surface %q:\n%s", stale, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.Run([]string{"prune"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected prune to be removed, got err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestCheckReportsMissingManagedEntryAndUpdateWritesCanonicalSyncManifest(t *testing.T) {
	targetDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := (Runner{Source: sourceAccessFailingClient{}}).Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil || err.Error() != "Sync manifest not found: .driftline/sync.toml" {
		t.Fatalf("expected canonical missing Sync manifest error, got %v", err)
	}

	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "ci\n"})
	runner := Runner{Source: client}

	stdout.Reset()
	stderr.Reset()
	err = runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected check drift")
	}
	wantChanges := "add github-workflow.ci: target file is missing\nsync-manifest-add github-workflow.ci: add Sync manifest entry\n"
	if stdout.String() != wantChanges {
		t.Fatalf("unexpected check output: %q", stdout.String())
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if stdout.String() != wantChanges {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "ci\n" {
		t.Fatalf("unexpected copied workflow: %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if !strings.Contains(manifest, `[files.github-workflow]`) || !strings.Contains(manifest, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("Sync manifest was not updated:\n%s", manifest)
	}
	assertFileMissing(t, targetDir, ".gitignore")
}

func TestGitIgnoreSectionCommandLifecyclePreservesTargetOwnedDuplicate(t *testing.T) {
	requireGitIgnoreUpdatePlatform(t)
	targetDir := t.TempDir()
	syncManifest := syncManifestTOML("")
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	outside := "# target-owned\n.env\n"
	writeFile(t, targetDir, ".gitignore", outside)
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env", "/dist/"]
`, nil)
	runner := Runner{Source: client}

	var stdout, stderr bytes.Buffer
	err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr)
	if err != errDrift {
		t.Fatalf("check should return drift, got %v", err)
	}
	if got, want := stdout.String(), "add gitignore: generated section is missing\n"; got != want {
		t.Fatalf("unexpected check output: got %q, want %q", got, want)
	}

	stdout.Reset()
	err = runner.Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if err != errDrift {
		t.Fatalf("diff should return drift, got %v", err)
	}
	for _, want := range []string{
		" .env\n",
		"+# start driftline from y-writings/source-repo/.driftline/contract.toml\n",
		"+# DO NOT EDIT: this section is managed automatically by driftline.\n",
		"+.env\n",
		"+/dist/\n",
		"+# end driftline\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("Gitignore diff missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got, want := stdout.String(), "add gitignore: generated section is missing\n"; got != want {
		t.Fatalf("unexpected update output: got %q, want %q", got, want)
	}
	wantGitIgnore := outside + "\n" + commandGitIgnoreBlock("y-writings/source-repo", ".env", "/dist/")
	if got := readFile(t, targetDir, ".gitignore"); got != wantGitIgnore {
		t.Fatalf("update should preserve outside duplicate and append generated section:\ngot:  %q\nwant: %q", got, wantGitIgnore)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("Gitignore-only update rewrote Sync manifest:\n%s", got)
	}

	stdout.Reset()
	if err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("post-update check failed: %v", err)
	}
	if got, want := stdout.String(), "synced\n"; got != want {
		t.Fatalf("unexpected synced output: got %q, want %q", got, want)
	}
}

func TestUpdateRemovesGitIgnoreSectionAndPreservesOutsideBytes(t *testing.T) {
	requireGitIgnoreUpdatePlatform(t)
	targetDir := t.TempDir()
	syncManifest := syncManifestTOML("")
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	before := []byte("local\r\n\r\n")
	after := []byte("# separator stays\ntail\n")
	current := append(append(append([]byte(nil), before...), []byte(commandGitIgnoreBlock("old/repo", ".env"))...), after...)
	writeFileBytes(t, targetDir, ".gitignore", current)
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got, want := stdout.String(), "remove gitignore: generated section is no longer declared\n"; got != want {
		t.Fatalf("unexpected removal output: got %q, want %q", got, want)
	}
	want := append(append([]byte(nil), before...), after...)
	if got, err := os.ReadFile(filepath.Join(targetDir, ".gitignore")); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("outside bytes changed: got %q, want %q, err=%v", got, want, err)
	}
	info, err := os.Lstat(filepath.Join(targetDir, ".gitignore"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("section removal should leave a regular file: info=%#v err=%v", info, err)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("Gitignore removal rewrote Sync manifest:\n%s", got)
	}
}

func TestUpdateReplacesEditedGitIgnoreSectionAndProvenanceInPlace(t *testing.T) {
	requireGitIgnoreUpdatePlatform(t)
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	before := "# target before\n\n"
	after := "# target after\n"
	writeFile(t, targetDir, ".gitignore", before+commandGitIgnoreBlock("old/repo", "edited-entry")+after)
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env", "/dist/"]
`, nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got, want := stdout.String(), "update gitignore: generated section differs\n"; got != want {
		t.Fatalf("unexpected update output: got %q, want %q", got, want)
	}
	want := before + commandGitIgnoreBlock("y-writings/source-repo", ".env", "/dist/") + after
	if got := readFile(t, targetDir, ".gitignore"); got != want {
		t.Fatalf("generated section was not replaced in place: got %q, want %q", got, want)
	}
}

func TestDiffReportsGitBinaryMessageForBinaryGitIgnoreOutsideBytes(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	original := []byte{'l', 'o', 'c', 'a', 'l', 0, '\n'}
	writeFileBytes(t, targetDir, ".gitignore", original)
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]
`, nil)

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if err != errDrift {
		t.Fatalf("binary diff should return drift, got %v", err)
	}
	if !strings.Contains(stdout.String(), "Binary files ") || !strings.Contains(stdout.String(), " differ\n") {
		t.Fatalf("expected Git binary diff output, got:\n%s", stdout.String())
	}
	if got, readErr := os.ReadFile(filepath.Join(targetDir, ".gitignore")); readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("diff changed .gitignore: got %q err=%v", got, readErr)
	}
}

func TestUpdateRejectsMalformedGitIgnoreBeforeManagedAndSyncWrites(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
	}{
		{
			name:    "unmatched marker",
			content: "# start driftline from old/repo/.driftline/contract.toml\n.env\n",
		},
		{
			name: "multiple sections",
			content: commandGitIgnoreBlock("old/repo", ".env") +
				commandGitIgnoreBlock("another/repo", "/dist/"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			syncManifest := syncManifestTOML("")
			writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
			writeFile(t, targetDir, ".gitignore", tt.content)
			writeFile(t, targetDir, "tool.toml", "target-owned\n")
			client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]

[files.tool]
config = { path = "tool.toml", mode = "managed" }
`, map[string]string{"tool.toml": "source\n"})

			var stdout, stderr bytes.Buffer
			err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir, "--force", "tool.config"}, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), "invalid driftline section in .gitignore") {
				t.Fatalf("expected marker planning error, got %v", err)
			}
			if strings.Contains(err.Error(), "--force") || strings.Contains(stdout.String(), "--force") {
				t.Fatalf("marker error must not include force guidance: err=%v stdout=%q", err, stdout.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("planning error should not print a partial plan: %q", stdout.String())
			}
			if got := readFile(t, targetDir, "tool.toml"); got != "target-owned\n" {
				t.Fatalf("marker error allowed Managed write: %q", got)
			}
			if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
				t.Fatalf("marker error allowed Sync rewrite:\n%s", got)
			}
			if got := readFile(t, targetDir, ".gitignore"); got != tt.content {
				t.Fatalf("marker error changed .gitignore: %q", got)
			}
		})
	}
}

func TestUpdateTransitionsBetweenManagedAndGitIgnoreSectionOwnership(t *testing.T) {
	requireGitIgnoreUpdatePlatform(t)
	t.Run("Managed to Gitignore section", func(t *testing.T) {
		targetDir := t.TempDir()
		writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.repository]
ignore = ".gitignore"
`))
		writeFile(t, targetDir, ".gitignore", "previous Managed bytes\n")
		client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]
`, nil)

		var stdout, stderr bytes.Buffer
		if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
			t.Fatalf("update failed: %v", err)
		}
		wantOutput := "remove repository.ignore: managed file removed from Contract\n" +
			"sync-manifest-remove repository.ignore: remove Sync manifest entry\n" +
			"add gitignore: generated section is missing\n"
		if got := stdout.String(); got != wantOutput {
			t.Fatalf("unexpected transition output: got %q, want %q", got, wantOutput)
		}
		if got, want := readFile(t, targetDir, ".gitignore"), commandGitIgnoreBlock("y-writings/source-repo", ".env"); got != want {
			t.Fatalf("Managed bytes should be replaced by generated-only file: got %q, want %q", got, want)
		}
	})

	t.Run("Gitignore section to Managed", func(t *testing.T) {
		targetDir := t.TempDir()
		writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
		writeFile(t, targetDir, ".gitignore", "# target-owned\n\n"+commandGitIgnoreBlock("y-writings/source-repo", ".env"))
		client := newCommandSourceClient("main", `version = 2

[files.repository]
ignore = { path = ".gitignore", mode = "managed" }
`, map[string]string{".gitignore": "whole source file\n"})
		runner := Runner{Source: client}

		var stdout, stderr bytes.Buffer
		err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr)
		if err != errDrift {
			t.Fatalf("new Managed .gitignore should conflict, got %v", err)
		}
		wantConflict := `conflict repository.ignore: target already exists
  target: .gitignore
  source mode: managed

Choose one:
  1. set another target path in .driftline/sync.toml
  2. move the existing target file
  3. rerun with --force repository.ignore to overwrite
`
		if got := stdout.String(); got != wantConflict {
			t.Fatalf("unexpected transition conflict:\n%s", got)
		}

		stdout.Reset()
		if err := runner.Run([]string{"update", "--target-dir", targetDir, "--force", "repository.ignore"}, &stdout, &stderr); err != nil {
			t.Fatalf("forced transition failed: %v", err)
		}
		wantOutput := "sync-manifest-add repository.ignore: add Sync manifest entry\n" +
			"update repository.ignore: target differs from source\n"
		if got := stdout.String(); got != wantOutput {
			t.Fatalf("unexpected forced transition output: got %q, want %q", got, wantOutput)
		}
		if got, want := readFile(t, targetDir, ".gitignore"), "whole source file\n"; got != want {
			t.Fatalf("forced transition should replace whole file: got %q, want %q", got, want)
		}

		stdout.Reset()
		if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
			t.Fatalf("force should be one-time, subsequent update failed: %v", err)
		}
		if got, want := stdout.String(), "synced\n"; got != want {
			t.Fatalf("unexpected subsequent output: got %q, want %q", got, want)
		}
	})
}

func TestUpdateGitIgnoreSectionPreservesTemplateBytesOutsideSection(t *testing.T) {
	requireGitIgnoreUpdatePlatform(t)
	targetDir := t.TempDir()
	syncManifest := syncManifestTOML("")
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	templateBytes := "# Template-owned defaults\n*.local\n"
	writeFile(t, targetDir, ".gitignore", templateBytes)
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]

[files.repository]
ignore = { path = ".gitignore", mode = "template" }
`, nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got, want := stdout.String(), "add gitignore: generated section is missing\n"; got != want {
		t.Fatalf("unexpected coexistence output: got %q, want %q", got, want)
	}
	want := templateBytes + "\n" + commandGitIgnoreBlock("y-writings/source-repo", ".env")
	if got := readFile(t, targetDir, ".gitignore"); got != want {
		t.Fatalf("Template bytes outside section changed: got %q, want %q", got, want)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("Gitignore-only Template coexistence rewrote Sync manifest:\n%s", got)
	}
}

func TestUpdateConflictPrintsCompletePlanIncludingGitIgnoreChange(t *testing.T) {
	targetDir := t.TempDir()
	syncManifest := syncManifestTOML("")
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	writeFile(t, targetDir, "tool.toml", "target-owned\n")
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]

[files.tool]
config = { path = "tool.toml", mode = "managed" }
`, nil)

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr)
	if err != errDrift {
		t.Fatalf("update should return conflict drift, got %v", err)
	}
	want := `conflict tool.config: target already exists
  target: tool.toml
  source mode: managed

Choose one:
  1. set another target path in .driftline/sync.toml
  2. move the existing target file
  3. rerun with --force tool.config to overwrite
add gitignore: generated section is missing
`
	if got := stdout.String(); got != want {
		t.Fatalf("conflict should print complete plan:\ngot:\n%swant:\n%s", got, want)
	}
	assertFileMissing(t, targetDir, ".gitignore")
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("conflict rewrote Sync manifest:\n%s", got)
	}
}

func TestDiffPrintsManagedChangesBeforeGitIgnoreDiff(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.tool]
config = "tool.toml"
`))
	writeFile(t, targetDir, "tool.toml", "old\n")
	writeFile(t, targetDir, ".gitignore", "# target-owned\n")
	client := newCommandSourceClient("main", `version = 2

[gitignore]
entries = [".env"]

[files.tool]
config = { path = "tool.toml", mode = "managed" }
`, map[string]string{"tool.toml": "new\n"})

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if err != errDrift {
		t.Fatalf("diff should return drift, got %v", err)
	}
	managedAt := strings.Index(stdout.String(), filepath.Join(targetDir, "tool.toml"))
	gitIgnoreAt := strings.Index(stdout.String(), filepath.Join(targetDir, ".gitignore"))
	if managedAt < 0 || gitIgnoreAt < 0 || managedAt >= gitIgnoreAt {
		t.Fatalf("Managed diff should precede Gitignore diff:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "+.env\n") {
		t.Fatalf("Gitignore diff missing generated entry:\n%s", stdout.String())
	}
}

func TestDiffPrintsCompleteGitIgnoreUpdateAndRemoval(t *testing.T) {
	for _, tt := range []struct {
		name          string
		contract      string
		current       string
		wantFragments []string
	}{
		{
			name: "update",
			contract: `version = 2

[gitignore]
entries = [".env"]
`,
			current: "# before\n\n" + commandGitIgnoreBlock("old/repo", "old-entry") + "# after\n",
			wantFragments: []string{
				" # before\n",
				"-# start driftline from old/repo/.driftline/contract.toml\n",
				"+# start driftline from y-writings/source-repo/.driftline/contract.toml\n",
				"-old-entry\n",
				"+.env\n",
				" # after\n",
			},
		},
		{
			name:     "remove",
			contract: "version = 2\n",
			current:  "# before\n\n" + commandGitIgnoreBlock("old/repo", "old-entry") + "# after\n",
			wantFragments: []string{
				" # before\n",
				"-# start driftline from old/repo/.driftline/contract.toml\n",
				"-old-entry\n",
				"-# end driftline\n",
				" # after\n",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
			writeFile(t, targetDir, ".gitignore", tt.current)
			client := newCommandSourceClient("main", tt.contract, nil)

			var stdout, stderr bytes.Buffer
			err := (Runner{Source: client}).Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
			if err != errDrift {
				t.Fatalf("diff should return drift, got %v", err)
			}
			for _, want := range tt.wantFragments {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("complete-file diff missing %q:\n%s", want, stdout.String())
				}
			}
			if got := readFile(t, targetDir, ".gitignore"); got != tt.current {
				t.Fatalf("diff changed .gitignore: got %q, want %q", got, tt.current)
			}
		})
	}
}

func TestUpdateManagesOldLockArtifactAsOrdinaryTarget(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	client := newCommandSourceClient("main", `version = 2

[files.driftline]
lock = { path = "driftline-lock.yaml", mode = "managed" }
`, map[string]string{"driftline-lock.yaml": "source\n"})

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("update failed: %v\nstdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if got := readFile(t, targetDir, "driftline-lock.yaml"); got != "source\n" {
		t.Fatalf("old lock artifact should be managed as an ordinary target, got %q", got)
	}
	if manifest := readFile(t, targetDir, driftline.SyncManifestPath); !strings.Contains(manifest, `lock = "driftline-lock.yaml"`) {
		t.Fatalf("Sync manifest should record ordinary old lock target:\n%s", manifest)
	}
}

func TestUpdateRemovesManagedFileDeletedFromSource(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`))
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	wantChanges := "remove github-workflow.ci: managed file removed from Contract\nsync-manifest-remove github-workflow.ci: remove Sync manifest entry\n"
	if stdout.String() != wantChanges {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}
	assertFileMissing(t, targetDir, ".github/workflows/ci.yaml")
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "github-workflow") || strings.Contains(manifest, "ci") {
		t.Fatalf("Sync manifest entry should be removed:\n%s", manifest)
	}
}

func TestUpdateRemovesStaleSyncManifestEntryWhenTargetPathBlockedByFile(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.old]
config = "config/old"
`))
	writeFile(t, targetDir, "config", "target-owned\n")
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, "config"); got != "target-owned\n" {
		t.Fatalf("parent file should be left untouched, got %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "old") {
		t.Fatalf("stale Sync manifest entry should be removed:\n%s", manifest)
	}
}

func TestUpdateLeavesDirectoryAtStaleManagedPath(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.old]
config = "dir"
`))
	if err := os.MkdirAll(filepath.Join(targetDir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	info, err := os.Stat(filepath.Join(targetDir, "dir"))
	if err != nil || !info.IsDir() {
		t.Fatalf("stale managed path directory should be left untouched, info=%#v err=%v", info, err)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "old") {
		t.Fatalf("stale Sync manifest entry should be removed:\n%s", manifest)
	}
}

func TestUpdatePreservesSyncManifestWhenOnlyManagedFileChanges(t *testing.T) {
	targetDir := t.TempDir()
	syncManifest := `version = 2

# keep target-side comments and order
[source]
ref = "main"
repository = "y-writings/source-repo"

[files.github-workflow]
# local placement rationale
ci = ".github/workflows/ci.yaml"
`
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "new\n"})

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "new\n" {
		t.Fatalf("managed file should be updated, got %q", got)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("Sync manifest should not be rewritten for file-only update:\n%s", got)
	}
}

func TestUpdatePreservesSyncManifestWhenAlreadySynced(t *testing.T) {
	targetDir := t.TempDir()
	syncManifest := `version = 2

# keep target-side comments and order
[source]
ref = "main"
repository = "y-writings/source-repo"

[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifest)
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "ci\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "ci\n"})

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, driftline.SyncManifestPath); got != syncManifest {
		t.Fatalf("Sync manifest should not be rewritten when already synced:\n%s", got)
	}
}

func TestUpdateManagedToTemplateLeavesTargetFileAndRemovesSyncManifestEntry(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.github-workflow]
release = ".github/workflows/release.yaml"
`))
	writeFile(t, targetDir, ".github/workflows/release.yaml", "target-owned\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
release = { path = ".github/workflows/release.yaml", mode = "template" }
`, map[string]string{".github/workflows/release.yaml": "source\n"})

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, ".github/workflows/release.yaml"); got != "target-owned\n" {
		t.Fatalf("managed-to-template should leave target untouched, got %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "release") || strings.Contains(manifest, "github-workflow") {
		t.Fatalf("Sync manifest entry should be removed:\n%s", manifest)
	}
}

func TestUpdateConflictDoesNotWriteAndForceOverwritesOnce(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "target-owned\n")
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "source\n"})
	runner := Runner{Source: client}

	var stdout, stderr bytes.Buffer
	err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected conflict")
	}
	wantConflict := `conflict github-workflow.ci: target already exists
  target: .github/workflows/ci.yaml
  source mode: managed

Choose one:
  1. set another target path in .driftline/sync.toml
  2. move the existing target file
  3. rerun with --force github-workflow.ci to overwrite
`
	if stdout.String() != wantConflict {
		t.Fatalf("unexpected conflict output:\n%s", stdout.String())
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("conflict must not overwrite target, got %q", got)
	}
	if manifest := readFile(t, targetDir, driftline.SyncManifestPath); strings.Contains(manifest, "github-workflow") {
		t.Fatalf("conflict must not update Sync manifest:\n%s", manifest)
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir, "--force", "github-workflow.ci"}, &stdout, &stderr); err != nil {
		t.Fatalf("forced update failed: %v", err)
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "source\n" {
		t.Fatalf("forced update should overwrite target once, got %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if !strings.Contains(manifest, `[files.github-workflow]`) || strings.Contains(manifest, "force") {
		t.Fatalf("Sync manifest should contain only path entry, no force state:\n%s", manifest)
	}
}

func TestUpdateConflictsWhenNewManagedTargetIsBrokenSymlink(t *testing.T) {
	targetDir := t.TempDir()
	outsideDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	linkPath := filepath.Join(targetDir, ".github/workflows/ci.yaml")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideTarget := filepath.Join(outsideDir, "outside.txt")
	if err := os.Symlink(outsideTarget, linkPath); err != nil {
		t.Fatal(err)
	}
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "source\n"})

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected broken symlink target conflict")
	}
	wantConflict := `conflict github-workflow.ci: target already exists
  target: .github/workflows/ci.yaml
  source mode: managed

Choose one:
  1. set another target path in .driftline/sync.toml
  2. remove or change the conflicting filesystem path or managed entry
`
	if stdout.String() != wantConflict {
		t.Fatalf("unexpected broken symlink conflict output:\n%s", stdout.String())
	}
	info, err := os.Lstat(linkPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("broken symlink should be left untouched, info=%#v err=%v", info, err)
	}
	if _, err := os.Stat(outsideTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("update must not write through broken symlink, stat err=%v", err)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "github-workflow") {
		t.Fatalf("conflict must not update Sync manifest:\n%s", manifest)
	}
}

func TestUpdateReplacesStaleManagedFileWithDirectoryChild(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.old]
config = "dir"
`))
	writeFile(t, targetDir, "dir", "old\n")
	client := newCommandSourceClient("main", `version = 2

[files.new]
config = { path = "dir/file", mode = "managed" }
`, map[string]string{"dir/file": "new\n"})

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, "dir/file"); got != "new\n" {
		t.Fatalf("unexpected child file content: %q", got)
	}
	manifest := readFile(t, targetDir, driftline.SyncManifestPath)
	if strings.Contains(manifest, "old") || !strings.Contains(manifest, `[files.new]`) || !strings.Contains(manifest, `config = "dir/file"`) {
		t.Fatalf("Sync manifest should move to new child entry:\n%s", manifest)
	}
}

func TestDiffReportsNonContentChanges(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(`[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`))
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected diff drift")
	}
	wantChanges := "remove github-workflow.ci: managed file removed from Contract\nsync-manifest-remove github-workflow.ci: remove Sync manifest entry\n"
	if stdout.String() != wantChanges {
		t.Fatalf("unexpected diff output: %q", stdout.String())
	}
}

func TestParseOptionsAcceptsStandardFlagFormsAndUpdateForce(t *testing.T) {
	for name, args := range map[string][]string{
		"double dash equals": {"--target-dir=/tmp/target"},
		"single dash space":  {"-target-dir", "/tmp/target"},
		"single dash equals": {"-target-dir=/tmp/target"},
	} {
		t.Run(name, func(t *testing.T) {
			opts, err := parseTargetOptions(args)
			if err != nil {
				t.Fatalf("parse target options failed: %v", err)
			}
			if opts.TargetDir != "/tmp/target" {
				t.Fatalf("unexpected target dir: %q", opts.TargetDir)
			}
		})
	}

	initOpts, err := parseInitOptions([]string{"y-writings/source-repo", "--ref=feature/foo", "-target-dir=/tmp/target"})
	if err != nil {
		t.Fatalf("parse init options failed: %v", err)
	}
	if initOpts.Repository != "y-writings/source-repo" || initOpts.Ref != "feature/foo" || initOpts.TargetDir != "/tmp/target" {
		t.Fatalf("unexpected init options: %#v", initOpts)
	}

	initForceBefore, err := parseInitOptions([]string{"--force", "y-writings/source-repo"})
	if err != nil {
		t.Fatalf("parse init --force before repository failed: %v", err)
	}
	if initForceBefore.Repository != "y-writings/source-repo" || !initForceBefore.Force {
		t.Fatalf("unexpected init --force before repository options: %#v", initForceBefore)
	}

	initForceAfter, err := parseInitOptions([]string{"y-writings/source-repo", "--force"})
	if err != nil {
		t.Fatalf("parse init --force after repository failed: %v", err)
	}
	if initForceAfter.Repository != "y-writings/source-repo" || !initForceAfter.Force {
		t.Fatalf("unexpected init --force after repository options: %#v", initForceAfter)
	}

	for name, args := range map[string][]string{
		"force equals": {"y-writings/source-repo", "--force=true"},
		"force value":  {"y-writings/source-repo", "--force", "github-workflow.ci"},
	} {
		t.Run("init rejects "+name, func(t *testing.T) {
			if _, err := parseInitOptions(args); err == nil {
				t.Fatalf("expected parse init options to reject %#v", args)
			}
		})
	}

	updateOpts, err := parseUpdateOptions([]string{"--target-dir", "/tmp/target", "--force", "github-workflow.ci"})
	if err != nil {
		t.Fatalf("parse update options failed: %v", err)
	}
	if updateOpts.TargetDir != "/tmp/target" || updateOpts.ForceKey != "github-workflow.ci" {
		t.Fatalf("unexpected update options: %#v", updateOpts)
	}
	for name, args := range map[string][]string{
		"force equals empty": {"--force="},
		"force space empty":  {"--force", ""},
	} {
		t.Run("update rejects "+name, func(t *testing.T) {
			if _, err := parseUpdateOptions(args); err == nil {
				t.Fatalf("expected parse update options to reject %#v", args)
			}
		})
	}
	if _, err := parseTargetOptions([]string{"--force", "github-workflow.ci"}); err == nil {
		t.Fatal("check/diff target options must not accept --force")
	}
}

func syncManifestTOML(files string) string {
	return `version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"
` + files
}

func newCommandSourceClient(ref string, contract string, files map[string]string) commandFakeSourceClient {
	commit := "0123456789abcdef0123456789abcdef01234567"
	client := commandFakeSourceClient{
		defaultRef:    ref,
		defaultCommit: commit,
		refs:          map[string]string{"y-writings/source-repo@" + ref: commit},
		files: map[string][]byte{
			"y-writings/source-repo@" + commit + ":" + driftline.ContractPath: []byte(contract),
		},
	}
	for path, content := range files {
		client.files["y-writings/source-repo@"+commit+":"+path] = []byte(content)
	}
	return client
}

func commandGitIgnoreBlock(repository string, entries ...string) string {
	return "# start driftline from " + repository + "/" + driftline.ContractPath + "\n" +
		"# DO NOT EDIT: this section is managed automatically by driftline.\n" +
		strings.Join(entries, "\n") + "\n" +
		"# end driftline\n"
}

func requireGitIgnoreUpdatePlatform(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
		return
	default:
		t.Skip("atomic .gitignore replacement is unsupported on this platform")
	}
}

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFileBytes(t *testing.T, root, path string, content []byte) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertFileMissing(t *testing.T, root, path string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be missing, stat err=%v", path, err)
	}
}
