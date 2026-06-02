package driftline

import (
	"strings"
	"testing"
)

func TestLoadSourceManifestStrictValidation(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 1\ngitignore:\n  - ' .cache/tool '\n  - ''\nfiles:\n  - id: example\n    source: templates/example.txt\n  - id: local-config\n    source: templates/config.local\n    if_not_exists: true\n"))
	if err != nil {
		t.Fatalf("load source manifest failed: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Files) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if !manifest.Files[1].IfNotExists {
		t.Fatalf("expected if_not_exists true")
	}
	if len(manifest.GitIgnore) != 2 {
		t.Fatalf("gitignore entries should be preserved before write-time trimming: %#v", manifest.GitIgnore)
	}
}

func TestLoadSourceManifestRejectsUnknownAndDuplicateKeys(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root":   "version: 1\nextra: true\nfiles: []\n",
		"duplicate root": "version: 1\nversion: 1\nfiles: []\n",
		"unknown file":   "version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    extra: true\n",
		"target file":    "version: 1\nfiles:\n  - id: sample\n    source: sample.txt\n    target: sample.txt\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSourceManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadTargetConfigDistinguishesOmittedAndExplicitFalse(t *testing.T) {
	config, err := LoadTargetConfigBytes([]byte("version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: inherited\n  - id: explicit\n    target: custom.txt\n    if_not_exists: false\n"))
	if err != nil {
		t.Fatalf("load target config failed: %v", err)
	}
	if config.Files[0].IfNotExists != nil {
		t.Fatalf("expected omitted if_not_exists to stay nil")
	}
	if config.Files[1].IfNotExists == nil || *config.Files[1].IfNotExists {
		t.Fatalf("expected explicit false override, got %#v", config.Files[1].IfNotExists)
	}
}

func TestTargetConfigFromSourceManifestRejectsDuplicateDefaultTargets(t *testing.T) {
	manifest, err := LoadSourceManifestBytes([]byte("version: 1\nfiles:\n  - id: first\n    source: same.txt\n  - id: second\n    source: ./same.txt\n"))
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
		"url repository":    "version: 1\nsource:\n  repository: https://github.com/y-writings/source-repo\n  ref: main\nfiles: []\n",
		"missing files":     "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\n",
		"duplicate file id": "version: 1\nsource:\n  repository: y-writings/source-repo\n  ref: main\nfiles:\n  - id: sample\n  - id: sample\n",
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
	_, err := LoadLockBytes([]byte("version: 1\nrepository: y-writings/source-repo\nref: main\ncommit: 0123456789abcdef0123456789abcdef01234567\nfiles:\n  - id: first\n    target: same.txt\n    source_sha256: a\n    target_sha256: a\n  - id: second\n    target: same.txt\n    source_sha256: b\n    target_sha256: b\n"))
	if err == nil {
		t.Fatal("expected duplicate target error")
	}
}

func TestValidateConfigPath(t *testing.T) {
	valid := []string{".github/workflows/ci.yml", "templates/my file.txt", "config/.env.example"}
	for _, path := range valid {
		if err := ValidateConfigPath(path, "test"); err != nil {
			t.Fatalf("expected %q to be valid: %v", path, err)
		}
	}

	invalid := []string{"", " ", "/abs", ".", "..", "../x", "a/../b", "a\\b", " leading.txt", "trailing.txt "}
	for _, path := range invalid {
		if err := ValidateConfigPath(path, "test"); err == nil || !strings.Contains(err.Error(), "test") {
			t.Fatalf("expected labelled validation error for %q, got %v", path, err)
		}
	}
}
