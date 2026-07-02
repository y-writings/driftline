package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptInitialTargetRepositoryHappyPath(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-owned\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt":     []byte("missing template\n"),
		"y-writings/source-repo@abc123:.github/workflows/ci.yaml": []byte("managed source\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Manifest:     initialAdoptionManifest(),
		TargetConfig: initialAdoptionTargetConfig(),
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}

	config, err := LoadTargetConfig(filepath.Join(root, TargetConfigPath))
	if err != nil {
		t.Fatalf("target config should parse: %v", err)
	}
	if got := config.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("managed target config entry mismatch: %q", got)
	}
	if _, ok := config.Files["templates"]; ok {
		t.Fatalf("template entries must not be recorded in target config: %#v", config.Files)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("missing template content mismatch: %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/existing.txt"); got != "target-owned\n" {
		t.Fatalf("existing template should be skipped, got %q", got)
	}
	if initialAdoptionTestPathExists(t, root, ".github/workflows/ci.yaml") {
		t.Fatal("managed files must not be copied during initial adoption")
	}
	if got, want := source.reads, []string{"y-writings/source-repo@abc123:templates/missing.txt"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected source reads: got %#v want %#v", got, want)
	}
}

func TestAdoptInitialTargetRepositoryDefaultsEmptyRootToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Source:       &fakeInitialAdoptionSource{},
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Manifest:     initialAdoptionManagedOnlyManifest(),
		TargetConfig: initialAdoptionManagedOnlyTargetConfig(),
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}
	if !initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("empty root should write target config in the working directory")
	}
}

func TestAdoptInitialTargetRepositoryRequiresSource(t *testing.T) {
	err := AdoptInitialTargetRepository(InitialAdoptionOptions{})
	if err == nil || err.Error() != "source client is required" {
		t.Fatalf("expected required source error, got %v", err)
	}
}

func TestAdoptInitialTargetRepositoryRejectsExistingTargetConfigBeforeWrites(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, TargetConfigPath, "existing\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Manifest:     initialAdoptionManifest(),
		TargetConfig: initialAdoptionTargetConfig(),
	})
	if err == nil || err.Error() != "target config already exists: .driftline-target.toml" {
		t.Fatalf("expected existing target config error, got %v", err)
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template files must not be written after existing target config error")
	}
	if len(source.reads) != 0 {
		t.Fatalf("source files must not be read after existing target config error: %#v", source.reads)
	}
}

func TestAdoptInitialTargetRepositoryRejectsExistingManagedTarget(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
			},
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing-target", path); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:         root,
				Source:       source,
				Repository:   "y-writings/source-repo",
				Commit:       "abc123",
				Manifest:     initialAdoptionManifest(),
				TargetConfig: initialAdoptionTargetConfig(),
			})
			if err == nil || err.Error() != "managed target already exists: .github/workflows/ci.yaml" {
				t.Fatalf("expected managed target error, got %v", err)
			}
			if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
				t.Fatal("target manifest must not be written after managed target conflict")
			}
			if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
				t.Fatal("template files must not be written after managed target conflict")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after managed target conflict: %#v", source.reads)
			}
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsReservedTargetPath(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manifest SourceManifest
	}{
		{
			name: "managed",
			manifest: SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
				"driftline": {"target": {Path: TargetConfigPath, Mode: ModeManaged}},
			}},
		},
		{
			name: "template",
			manifest: SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
				"driftline": {"target": {Path: TargetConfigPath, Mode: ModeTemplate}},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			source := &fakeInitialAdoptionSource{files: map[string][]byte{
				"y-writings/source-repo@abc123:" + TargetConfigPath: []byte("reserved\n"),
			}}

			err := AdoptInitialTargetRepository(InitialAdoptionOptions{
				Root:         root,
				Source:       source,
				Repository:   "y-writings/source-repo",
				Commit:       "abc123",
				Manifest:     tt.manifest,
				TargetConfig: initialAdoptionNoFilesTargetConfig(),
			})
			if err == nil || !strings.Contains(err.Error(), "reserved target path") {
				t.Fatalf("expected reserved path error, got %v", err)
			}
			if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
				t.Fatal("target manifest must not be written after reserved path error")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after reserved path error: %#v", source.reads)
			}
		})
	}
}

func TestAdoptInitialTargetRepositoryRejectsMissingTemplateSourceBeforeWrites(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:         root,
		Source:       source,
		Repository:   "y-writings/source-repo",
		Commit:       "abc123",
		Manifest:     initialAdoptionTemplateOnlyManifest(),
		TargetConfig: initialAdoptionNoFilesTargetConfig(),
	})
	if err == nil || !strings.Contains(err.Error(), "source template not found in source repository") {
		t.Fatalf("expected missing template source error, got %v", err)
	}
	if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("target manifest must not be written after missing source template")
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template file must not be written after missing source template")
	}
}

func TestAdoptInitialTargetRepositoryDoesNotWriteTemplatesWhenTargetConfigPrepareFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:       root,
		Source:     source,
		Repository: "y-writings/source-repo",
		Commit:     "abc123",
		Manifest:   initialAdoptionTemplateOnlyManifest(),
		TargetConfig: TargetConfig{
			Version: 2,
			Source:  TargetSource{Repository: "invalid", Ref: "main"},
		},
	})
	if err == nil {
		t.Fatal("expected target config prepare failure")
	}
	if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
		t.Fatal("template file must not be written when target config prepare fails")
	}
	if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("target manifest must not be committed when prepare fails")
	}
}

func TestInitialAdoptionDoesNotCommitTargetConfigWhenTemplateWriteFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}
	var writes []string

	err := initialAdoption{
		opts: InitialAdoptionOptions{
			Root:         root,
			Source:       source,
			Repository:   "y-writings/source-repo",
			Commit:       "abc123",
			Manifest:     initialAdoptionTemplateOnlyManifest(),
			TargetConfig: initialAdoptionNoFilesTargetConfig(),
		},
		writeFileBytes: func(target string, data []byte) error {
			writes = append(writes, target)
			return errors.New("write failed")
		},
	}.adopt()
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("expected write failure, got %v", err)
	}
	if len(writes) != 1 || writes[0] != filepath.Join(root, "templates/missing.txt") {
		t.Fatalf("unexpected template writes: %#v", writes)
	}
	if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("target manifest must not be committed after template write failure")
	}
}

func TestInitialAdoptionDoesNotOverwriteForcedManagedTargetWhenTemplateWriteFails(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-template\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:.github/workflows/ci.yaml": []byte("source\n"),
		"y-writings/source-repo@abc123:templates/missing.txt":     []byte("missing template\n"),
	}}

	err := initialAdoption{
		opts: InitialAdoptionOptions{
			Root:         root,
			Source:       source,
			Repository:   "y-writings/source-repo",
			Commit:       "abc123",
			Manifest:     initialAdoptionManifest(),
			TargetConfig: initialAdoptionTargetConfig(),
			ForceKey:     "github-workflow.ci",
		},
		writeFileBytes: func(target string, data []byte) error {
			if filepath.Base(target) == "missing.txt" {
				return errors.New("template write failed")
			}
			return WriteFileBytes(target, data)
		},
	}.adopt()
	if err == nil || err.Error() != "template write failed" {
		t.Fatalf("expected template write failure, got %v", err)
	}
	if got := readInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("forced managed target must stay untouched after template failure, got %q", got)
	}
	if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("target manifest must not be committed after template write failure")
	}
}

func TestInitialAdoptionLeavesTemplatesWhenTargetConfigCommitFails(t *testing.T) {
	root := t.TempDir()
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := initialAdoption{
		opts: InitialAdoptionOptions{
			Root:         root,
			Source:       source,
			Repository:   "y-writings/source-repo",
			Commit:       "abc123",
			Manifest:     initialAdoptionTemplateOnlyManifest(),
			TargetConfig: initialAdoptionNoFilesTargetConfig(),
		},
		prepareTargetConfigWrite: func(path string, config TargetConfig) (func() error, func() error, error) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, nil, err
			}
			temp, err := os.CreateTemp(filepath.Dir(path), ".driftline-target-*.toml")
			if err != nil {
				return nil, nil, err
			}
			if _, err := temp.WriteString(FormatTargetConfig(config)); err != nil {
				temp.Close()
				os.Remove(temp.Name())
				return nil, nil, err
			}
			if err := temp.Close(); err != nil {
				os.Remove(temp.Name())
				return nil, nil, err
			}
			cleanup := func() error { return os.Remove(temp.Name()) }
			commit := func() error { return errors.New("commit failed") }
			return commit, cleanup, nil
		},
	}.adopt()
	if err == nil || err.Error() != "commit failed" {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("template file should remain after commit failure, got %q", got)
	}
	if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
		t.Fatal("target manifest path must be missing after commit failure")
	}
}

type fakeInitialAdoptionSource struct {
	files map[string][]byte
	reads []string
}

func (s *fakeInitialAdoptionSource) ResolveDefaultRef(repository string) (string, string, error) {
	return "", "", errors.New("ResolveDefaultRef is not used by initial adoption")
}

func (s *fakeInitialAdoptionSource) ResolveRef(repository string, ref string) (string, error) {
	return "", errors.New("ResolveRef is not used by initial adoption")
}

func (s *fakeInitialAdoptionSource) ReadFile(repository string, commit string, path string) ([]byte, error) {
	key := fmt.Sprintf("%s@%s:%s", repository, commit, path)
	s.reads = append(s.reads, key)
	data, ok := s.files[key]
	if !ok {
		return nil, fmt.Errorf("missing source file %s", key)
	}
	return append([]byte(nil), data...), nil
}

func initialAdoptionManifest() SourceManifest {
	return SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
		"templates": {
			"existing": {Path: "templates/existing.txt", Mode: ModeTemplate},
			"missing":  {Path: "templates/missing.txt", Mode: ModeTemplate},
		},
	}}
}

func initialAdoptionManagedOnlyManifest() SourceManifest {
	return SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"github-workflow": {"ci": {Path: ".github/workflows/ci.yaml", Mode: ModeManaged}},
	}}
}

func initialAdoptionTemplateOnlyManifest() SourceManifest {
	return SourceManifest{Version: 2, Files: map[string]map[string]SourceManifestFile{
		"templates": {"missing": {Path: "templates/missing.txt", Mode: ModeTemplate}},
	}}
}

func initialAdoptionTargetConfig() TargetConfig {
	return TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{
		"github-workflow": {"ci": ".github/workflows/ci.yaml"},
	}}
}

func initialAdoptionManagedOnlyTargetConfig() TargetConfig {
	return initialAdoptionTargetConfig()
}

func initialAdoptionNoFilesTargetConfig() TargetConfig {
	return TargetConfig{Version: 2, Source: TargetSource{Repository: "y-writings/source-repo", Ref: "main"}}
}

func writeInitialAdoptionTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readInitialAdoptionTestFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func initialAdoptionTestPathExists(t *testing.T, root, path string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, path))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatal(err)
	return false
}
