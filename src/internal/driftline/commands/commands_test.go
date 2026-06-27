package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/y-writings/driftline/src/internal/driftline"
)

type commandFakeSourceClient struct {
	defaultRef    string
	defaultCommit string
	refs          map[string]string
	files         map[string][]byte
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
	data, ok := f.files[repository+"@"+commit+":"+path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestInitCreatesTargetConfigAndPlacesTemplates(t *testing.T) {
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

	config := readFile(t, targetDir, driftline.TargetConfigPath)
	for _, want := range []string{"version = 2", `[source]`, `repository = "y-writings/source-repo"`, `[files.github-workflow]`, `ci = ".github/workflows/ci.yaml"`} {
		if !strings.Contains(config, want) {
			t.Fatalf("generated config missing %q:\n%s", want, config)
		}
	}
	for _, removed := range []string{"release", "mise", "template", "path_overrides", "if_not_exists"} {
		if strings.Contains(config, removed) {
			t.Fatalf("target config contains non-managed or old field %q:\n%s", removed, config)
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
	if !strings.Contains(stdout.String(), "created .driftline-target.toml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestInitRefPreservesInputRef(t *testing.T) {
	targetDir := t.TempDir()
	client := newCommandSourceClient("feature/foo", "version = 2\n", nil)
	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--ref", "feature/foo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if got := readFile(t, targetDir, driftline.TargetConfigPath); !strings.Contains(got, `ref = "feature/foo"`) {
		t.Fatalf("expected input ref to be preserved:\n%s", got)
	}
}

func TestInitFailsBeforeWritingWhenConfigOrManagedTargetExists(t *testing.T) {
	for name, setup := range map[string]func(string){
		"target config exists": func(targetDir string) {
			writeFile(t, targetDir, driftline.TargetConfigPath, "existing\n")
		},
		"managed target exists": func(targetDir string) {
			writeFile(t, targetDir, ".github/workflows/ci.yaml", "existing\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			setup(targetDir)
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
			if _, err := os.Stat(filepath.Join(targetDir, ".github/workflows/release.yaml")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("init must fail before placing templates, stat err=%v", err)
			}
		})
	}
}

func TestInitRejectsSourceFilesTargetingTargetConfig(t *testing.T) {
	for name, sourceManifest := range map[string]string{
		"managed": `version = 2

[files.driftline]
target = { path = ".driftline-target.toml", mode = "managed" }
`,
		"template": `version = 2

[files.driftline]
target = { path = ".driftline-target.toml", mode = "template" }
`,
	} {
		t.Run(name, func(t *testing.T) {
			targetDir := t.TempDir()
			client := newCommandSourceClient("main", sourceManifest, map[string]string{driftline.TargetConfigPath: "template bytes\n"})
			var stdout, stderr bytes.Buffer

			err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr)

			if err == nil || !strings.Contains(err.Error(), "reserved target path") {
				t.Fatalf("expected reserved target path error, got %v", err)
			}
			assertFileMissing(t, targetDir, driftline.TargetConfigPath)
		})
	}
}

func TestHelpOmitsPruneAndPruneCommandIsRemoved(t *testing.T) {
	runner := Runner{Source: commandFakeSourceClient{}}
	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	for _, want := range []string{"driftline init owner/repo", "check", "diff", "update", "GITHUB_TOKEN"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "prune") || strings.Contains(stdout.String(), "driftline-lock") || strings.Contains(stdout.String(), ".yaml") {
		t.Fatalf("help still mentions removed surface:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runner.Run([]string{"prune"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected prune to be removed, got err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestCheckReportsMissingManagedEntryAndUpdateCreatesNoLock(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.TargetConfigPath, targetConfigTOML(""))
	client := newCommandSourceClient("main", `version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
`, map[string]string{".github/workflows/ci.yaml": "ci\n"})
	runner := Runner{Source: client}

	var stdout, stderr bytes.Buffer
	err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected check drift")
	}
	for _, want := range []string{"target-config-add github-workflow.ci: add target config entry", "add github-workflow.ci: target file is missing"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("check output missing %q: %q", want, stdout.String())
		}
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "ci\n" {
		t.Fatalf("unexpected copied workflow: %q", got)
	}
	config := readFile(t, targetDir, driftline.TargetConfigPath)
	if !strings.Contains(config, `[files.github-workflow]`) || !strings.Contains(config, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("target config was not updated:\n%s", config)
	}
	assertFileMissing(t, targetDir, "driftline-lock.yaml")
	assertFileMissing(t, targetDir, ".gitignore")
}

func TestUpdateRemovesManagedFileDeletedFromSource(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.TargetConfigPath, targetConfigTOML(`[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`))
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	assertFileMissing(t, targetDir, ".github/workflows/ci.yaml")
	config := readFile(t, targetDir, driftline.TargetConfigPath)
	if strings.Contains(config, "github-workflow") || strings.Contains(config, "ci") {
		t.Fatalf("target config entry should be removed:\n%s", config)
	}
}

func TestUpdateManagedToTemplateLeavesTargetFileAndRemovesConfig(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.TargetConfigPath, targetConfigTOML(`[files.github-workflow]
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
	config := readFile(t, targetDir, driftline.TargetConfigPath)
	if strings.Contains(config, "release") || strings.Contains(config, "github-workflow") {
		t.Fatalf("target config entry should be removed:\n%s", config)
	}
}

func TestUpdateConflictDoesNotWriteAndForceOverwritesOnce(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.TargetConfigPath, targetConfigTOML(""))
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
	for _, want := range []string{"conflict github-workflow.ci: target already exists", "target: .github/workflows/ci.yaml", "rerun with --force github-workflow.ci"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("conflict output missing %q:\n%s", want, stdout.String())
		}
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("conflict must not overwrite target, got %q", got)
	}
	if config := readFile(t, targetDir, driftline.TargetConfigPath); strings.Contains(config, "github-workflow") {
		t.Fatalf("conflict must not update target config:\n%s", config)
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir, "--force", "github-workflow.ci"}, &stdout, &stderr); err != nil {
		t.Fatalf("forced update failed: %v", err)
	}
	if got := readFile(t, targetDir, ".github/workflows/ci.yaml"); got != "source\n" {
		t.Fatalf("forced update should overwrite target once, got %q", got)
	}
	config := readFile(t, targetDir, driftline.TargetConfigPath)
	if !strings.Contains(config, `[files.github-workflow]`) || strings.Contains(config, "force") {
		t.Fatalf("target config should contain only path entry, no force state:\n%s", config)
	}
}

func TestDiffReportsNonContentChanges(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.TargetConfigPath, targetConfigTOML(`[files.github-workflow]
ci = ".github/workflows/ci.yaml"
`))
	writeFile(t, targetDir, ".github/workflows/ci.yaml", "old\n")
	client := newCommandSourceClient("main", "version = 2\n", nil)

	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected diff drift")
	}
	for _, want := range []string{"remove github-workflow.ci: managed file removed from source config", "target-config-remove github-workflow.ci: remove target config entry"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("diff output missing %q:\n%s", want, stdout.String())
		}
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

	updateOpts, err := parseUpdateOptions([]string{"--target-dir", "/tmp/target", "--force", "github-workflow.ci"})
	if err != nil {
		t.Fatalf("parse update options failed: %v", err)
	}
	if updateOpts.TargetDir != "/tmp/target" || updateOpts.ForceKey != "github-workflow.ci" {
		t.Fatalf("unexpected update options: %#v", updateOpts)
	}
	if _, err := parseTargetOptions([]string{"--force", "github-workflow.ci"}); err == nil {
		t.Fatal("check/diff target options must not accept --force")
	}
}

func targetConfigTOML(files string) string {
	return `version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"
` + files
}

func newCommandSourceClient(ref string, sourceManifest string, files map[string]string) commandFakeSourceClient {
	commit := "0123456789abcdef0123456789abcdef01234567"
	client := commandFakeSourceClient{
		defaultRef:    ref,
		defaultCommit: commit,
		refs:          map[string]string{"y-writings/source-repo@" + ref: commit},
		files: map[string][]byte{
			"y-writings/source-repo@" + commit + ":" + driftline.SourceManifestPath: []byte(sourceManifest),
		},
	}
	for path, content := range files {
		client.files["y-writings/source-repo@"+commit+":"+path] = []byte(content)
	}
	return client
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
