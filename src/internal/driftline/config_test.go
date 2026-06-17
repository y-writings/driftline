package driftline

import (
	"strings"
	"testing"
)

func TestLoadSourceManifestStrictValidation(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 2\ngitignore:\n  - ' .cache/tool '\n  - ''\nfiles:\n  - id: example\n    name: Example files\n    paths:\n      example:\n        name: Example file\n        path: templates/example.txt\n      extra:\n        path: templates/example-extra.txt\n  - id: local-config\n    paths:\n      config:\n        path: templates/config.local\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 2 || len(manifest.Files) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if manifest.Files[0].Name != "Example files" {
		t.Fatalf("unexpected manifest file name: %#v", manifest.Files[0])
	}
	if len(manifest.Files[0].Paths) != 2 || manifest.Files[0].Paths[0].ID != "example" || manifest.Files[0].Paths[0].Name != "Example file" || manifest.Files[0].Paths[0].Path != "templates/example.txt" || manifest.Files[0].Paths[1].ID != "extra" || manifest.Files[0].Paths[1].Path != "templates/example-extra.txt" {
		t.Fatalf("unexpected source paths: %#v", manifest.Files[0])
	}
	if len(manifest.GitIgnore) != 2 {
		t.Fatalf("gitignore entries should be preserved before write-time trimming: %#v", manifest.GitIgnore)
	}
}

func TestLoadSourceManifestRejectsUnknownAndDuplicateKeys(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root":         "version: 2\nextra: true\nfiles: []\n",
		"duplicate root":       "version: 2\nversion: 2\nfiles: []\n",
		"unknown file":         "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n    extra: true\n",
		"old source path":      "version: 2\nfiles:\n  - id: sample\n    source_path: sample.txt\n",
		"target file":          "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n    target: sample.txt\n",
		"source if_not_exists": "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n    if_not_exists: true\n",
		"unknown path field":   "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: sample.txt\n        target: custom.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSourceManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadSourceManifestRejectsInvalidPaths(t *testing.T) {
	for name, input := range map[string]string{
		"missing paths":     "version: 2\nfiles:\n  - id: sample\n",
		"empty paths":       "version: 2\nfiles:\n  - id: sample\n    paths: {}\n",
		"invalid path":      "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        path: ../sample.txt\n",
		"duplicate path":    "version: 2\nfiles:\n  - id: sample\n    paths:\n      first:\n        path: same.txt\n      second:\n        path: ./same.txt\n",
		"blank path id":     "version: 2\nfiles:\n  - id: sample\n    paths:\n      ' ':\n        path: sample.txt\n",
		"non-scalar name":   "version: 2\nfiles:\n  - id: sample\n    paths:\n      sample:\n        name:\n          nested: true\n        path: sample.txt\n",
		"duplicate file id": "version: 2\nfiles:\n  - id: sample\n    paths:\n      one:\n        path: one.txt\n  - id: sample\n    paths:\n      two:\n        path: two.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSourceManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadTargetConfigDecodesPathOverridesAndIfNotExists(t *testing.T) {
	config, err := LoadTargetConfigBytes([]byte("version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: inherited\n  - id: explicit\n    path_overrides:\n      source: custom.txt\n    if_not_exists: true\n  - id: explicit-false\n    if_not_exists: false\n"))
	if err != nil {
		t.Fatalf("load target config failed: %v", err)
	}
	if config.Files[0].IfNotExists {
		t.Fatalf("expected omitted if_not_exists to default false")
	}
	if len(config.Files[1].PathOverrides) != 1 || config.Files[1].PathOverrides["source"] != "custom.txt" {
		t.Fatalf("expected path_overrides to decode, got %#v", config.Files[1])
	}
	if !config.Files[1].IfNotExists {
		t.Fatalf("expected explicit true, got %#v", config.Files[1])
	}
	if config.Files[2].IfNotExists {
		t.Fatalf("expected explicit false, got %#v", config.Files[2])
	}
}

func TestLoadTargetConfigRejectsOldTargetPathKey(t *testing.T) {
	_, err := LoadTargetConfigBytes([]byte("version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: explicit\n    target_path: custom.txt\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target_path to be rejected as an unknown key, got %v", err)
	}
}

func TestLoadTargetConfigRejectsInvalidPathOverrides(t *testing.T) {
	for name, input := range map[string]string{
		"empty overrides":     "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides: {}\n",
		"invalid override id": "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      ' ': custom.txt\n",
		"invalid target path": "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      source: ../custom.txt\n",
		"old override list":   "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n    path_overrides:\n      - from: source\n        to: custom.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadTargetConfigBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTargetConfigFromSourceManifestRejectsDuplicateDefaultTargets(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 2\nfiles:\n  - id: first\n    paths:\n      first:\n        path: same.txt\n  - id: second\n    paths:\n      second:\n        path: ./same.txt\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}

	_, err = TargetConfigFromSourceManifest("y-writings/source-repo", "main", manifest)
	if err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("expected duplicate default target error, got %v", err)
	}
}

func TestLoadTargetConfigRejectsBadRepositoryAndDuplicateFilesKey(t *testing.T) {
	for name, input := range map[string]string{
		"url repository":    "version: 2\nsource:\n  repository: https://github.com/y-writings/source-repo\n  ref: main\nfiles: []\n",
		"missing files":     "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\n",
		"duplicate file id": "version: 2\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n  - id: sample\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadTargetConfigBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadLockFileRejectsDuplicateTarget(t *testing.T) {
	_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: first\n    target_path: same.txt\n  - id: second\n    target_path: same.txt\n"))
	if err == nil {
		t.Fatal("expected duplicate target error")
	}
}

func TestLoadLockFileRejectsOldTargetKey(t *testing.T) {
	_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target: sample.txt\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target to be rejected as an unknown key, got %v", err)
	}
}

func TestLoadLockFileRejectsHashFields(t *testing.T) {
	_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target_path: sample.txt\n    source_sha256: old\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected source_sha256 to be rejected as an unknown key, got %v", err)
	}

	_, err = LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: sample\n    target_path: sample.txt\n    target_sha256: old\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected target_sha256 to be rejected as an unknown key, got %v", err)
	}
}

func TestValidateConfigPath(t *testing.T) {
	valid := []string{".github/workflows/ci.yml", "templates/my file.txt", "config/.env.example"}
	for _, path := range valid {
		if err := ValidateConfigPath(path, "test"); err != nil {
			t.Fatalf("expected %q to be valid: %v", path, err)
		}
	}

	invalid := []string{"", " ", "/abs", ".", "..", "../x", "a/../b", "a\\b", "templates/", " leading.txt", "trailing.txt "}
	for _, path := range invalid {
		if err := ValidateConfigPath(path, "test"); err == nil || !strings.Contains(err.Error(), "test") {
			t.Fatalf("expected labelled validation error for %q, got %v", path, err)
		}
	}
}
