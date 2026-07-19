package driftline

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestLoadContractTOML(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if contract.Version != 2 {
		t.Fatalf("unexpected version: %d", contract.Version)
	}
	ci := contract.Files["github-workflow"]["ci"]
	if ci.Path != ".github/workflows/ci.yaml" || ci.Mode != ModeManaged {
		t.Fatalf("unexpected ci entry: %#v", ci)
	}
	release := contract.Files["github-workflow"]["release"]
	if release.Path != ".github/workflows/release.yaml" || release.Mode != ModeTemplate {
		t.Fatalf("unexpected release entry: %#v", release)
	}
	config := contract.Files["mise"]["config"]
	if config.Path != ".mise/config.toml" || config.Mode != ModeTemplate {
		t.Fatalf("unexpected mise config entry: %#v", config)
	}
}

func TestLoadContractGitIgnoreEntries(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[gitignore]
entries = [
  "",
  "  ",
  ".env",
  ".env",
  " *.log ",
  "!/dist/.gitkeep",
  "# DO NOT EDIT: this section is managed automatically by driftline.",
  "# start driftline from invalid/.driftline/contract.toml",
]

[files.root]
ignore = { path = ".gitignore", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if contract.Version != 2 {
		t.Fatalf("unexpected version: %d", contract.Version)
	}
	if contract.GitIgnore == nil {
		t.Fatal("expected gitignore configuration")
	}
	want := []string{
		"",
		"  ",
		".env",
		".env",
		" *.log ",
		"!/dist/.gitkeep",
		"# DO NOT EDIT: this section is managed automatically by driftline.",
		"# start driftline from invalid/.driftline/contract.toml",
	}
	if !slices.Equal(contract.GitIgnore.Entries, want) {
		t.Fatalf("unexpected gitignore entries: %#v", contract.GitIgnore.Entries)
	}
	if got := contract.Files["root"]["ignore"]; got.Path != GitIgnorePath || got.Mode != ModeTemplate {
		t.Fatalf("unexpected Template .gitignore entry: %#v", got)
	}
}

func TestLoadContractGitIgnoreAcceptsExplicitEmptyEntries(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[gitignore]
entries = []
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if contract.GitIgnore == nil {
		t.Fatal("expected gitignore configuration")
	}
	if len(contract.GitIgnore.Entries) != 0 {
		t.Fatalf("expected empty gitignore entries, got %#v", contract.GitIgnore.Entries)
	}
}

func TestLoadContractRejectsGitIgnoreKeyAliases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "noncanonical table missing entries",
			input: `version = 2

[GitIgnore]
`,
			wantErr: `Contract contains unknown key "GitIgnore"`,
		},
		{
			name: "noncanonical table with entries",
			input: `version = 2

[GitIgnore]
entries = []
`,
			wantErr: `Contract contains unknown key "GitIgnore"`,
		},
		{
			name: "uppercase table",
			input: `version = 2

[GITIGNORE]
entries = []
`,
			wantErr: `Contract contains unknown key "GITIGNORE"`,
		},
		{
			name: "canonical and noncanonical tables",
			input: `version = 2

[gitignore]
entries = ["canonical"]

[GitIgnore]
entries = ["alias"]
`,
			wantErr: `Contract contains unknown key "GitIgnore"`,
		},
		{
			name: "noncanonical entries",
			input: `version = 2

[gitignore]
Entries = []
`,
			wantErr: `Contract contains unknown key "gitignore.Entries"`,
		},
		{
			name: "uppercase entries",
			input: `version = 2

[gitignore]
ENTRIES = []
`,
			wantErr: `Contract contains unknown key "gitignore.ENTRIES"`,
		},
		{
			name: "canonical and noncanonical entries",
			input: `version = 2

[gitignore]
entries = ["canonical"]
Entries = ["alias"]
`,
			wantErr: `Contract contains unknown key "gitignore.Entries"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadContractBytes([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadContractRejectsInvalidGitIgnoreConfiguration(t *testing.T) {
	for name, tt := range map[string]struct {
		input   string
		wantErr string
	}{
		"missing entries": {
			input: `version = 2

[gitignore]
`,
			wantErr: "must define entries",
		},
		"unknown field": {
			input: `version = 2

[gitignore]
entries = []
unknown = true
`,
			wantErr: "contains unknown key",
		},
		"decoded multiline entry": {
			input: `version = 2

[gitignore]
entries = ["""first
second"""]
`,
			wantErr: "contains CR or LF",
		},
		"entry containing CR": {
			input: `version = 2

[gitignore]
entries = ["first\rsecond"]
`,
			wantErr: "contains CR or LF",
		},
		"end marker entry": {
			input: `version = 2

[gitignore]
entries = ["# end driftline"]
`,
			wantErr: "conflicts with a driftline marker",
		},
		"start marker entry": {
			input: `version = 2

[gitignore]
entries = ["# start driftline from y-writings/source-repo/.driftline/contract.toml"]
`,
			wantErr: "conflicts with a driftline marker",
		},
		"Managed exact path with empty entries": {
			input: `version = 2

[gitignore]
entries = []

[files.root]
ignore = { path = ".gitignore", mode = "managed" }
`,
			wantErr: "cannot manage .gitignore",
		},
		"Managed normalized leading-dot exact path": {
			input: `version = 2

[gitignore]
entries = []

[files.root]
ignore = { path = "./.gitignore", mode = "managed" }
`,
			wantErr: "cannot manage .gitignore",
		},
		"Managed normalized trailing-dot exact path": {
			input: `version = 2

[gitignore]
entries = []

[files.root]
ignore = { path = ".gitignore/.", mode = "managed" }
`,
			wantErr: "cannot manage .gitignore",
		},
		"Managed descendant with entries": {
			input: `version = 2

[gitignore]
entries = [".env"]

[files.root]
ignore = { path = ".gitignore/rules", mode = "managed" }
`,
			wantErr: "cannot be below .gitignore",
		},
		"Managed normalized descendant with entries": {
			input: `version = 2

[gitignore]
entries = [".env"]

[files.root]
ignore = { path = ".gitignore/./rules", mode = "managed" }
`,
			wantErr: "cannot be below .gitignore",
		},
		"Template descendant with entries": {
			input: `version = 2

[gitignore]
entries = [".env"]

[files.root]
ignore = { path = ".gitignore/rules", mode = "template" }
`,
			wantErr: "cannot be below .gitignore",
		},
		"Template normalized descendant with entries": {
			input: `version = 2

[gitignore]
entries = [".env"]

[files.root]
ignore = { path = ".gitignore/./rules", mode = "template" }
`,
			wantErr: "cannot be below .gitignore",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadContractBytes([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadContractAcceptsTOML11MultilineInlineTables(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = {
  path = ".github/workflows/ci.yaml",
  mode = "managed",
}
release = {
  path = ".github/workflows/release.yaml",
  mode = "template",
}
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if got := contract.Files["github-workflow"]["ci"].Mode; got != ModeManaged {
		t.Fatalf("unexpected ci mode: %q", got)
	}
	if got := contract.Files["github-workflow"]["release"].Mode; got != ModeTemplate {
		t.Fatalf("unexpected release mode: %q", got)
	}
}

func TestLoadContractRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":          "version = 2\nextra = true\n",
		"unknown Contract file field": "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"managed\", extra = true }\n",
		"invalid mode":                "version = 2\n[files.github-workflow]\nci = { path = \"ci.yaml\", mode = \"copy\" }\n",
		"missing path":                "version = 2\n[files.github-workflow]\nci = { mode = \"managed\" }\n",
		"invalid group id":            "version = 2\n[files.\"github.workflow\"]\nci = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid file id":             "version = 2\n[files.github-workflow]\n\"bad/id\" = { path = \"ci.yaml\", mode = \"managed\" }\n",
		"invalid source path":         "version = 2\n[files.github-workflow]\nci = { path = \"../ci.yaml\", mode = \"managed\" }\n",
		"duplicate source path":       "version = 2\n[files.first]\nci = { path = \"./same.yaml\", mode = \"managed\" }\n[files.second]\nci = { path = \"same.yaml\", mode = \"template\" }\n",
		"old yaml shape":              "version: 2\nfiles:\n  - id: ci\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadContractBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadContractRejectsReservedMetadataPaths(t *testing.T) {
	paths := []string{
		".driftline",
		".driftline/contract.toml",
		".driftline/future/file",
		"./.driftline/future",
		".driftline/./future",
	}
	for _, path := range paths {
		for _, mode := range []FileMode{ModeManaged, ModeTemplate} {
			t.Run(fmt.Sprintf("%s/%s", mode, path), func(t *testing.T) {
				_, err := LoadContractBytes([]byte(fmt.Sprintf(`version = 2

[files.metadata]
file = { path = %q, mode = %q }
`, path, mode)))
				if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "reserved driftline metadata path") {
					t.Fatalf("expected reserved metadata error containing authored path %q, got %v", path, err)
				}
			})
		}
	}
}

func TestLoadSyncManifestTOML(t *testing.T) {
	manifest, err := LoadSyncManifestBytes([]byte(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
`))
	if err != nil {
		t.Fatalf("load Sync manifest failed: %v", err)
	}
	if manifest.Version != 2 || manifest.Source.Repository != "y-writings/source-repo" || manifest.Source.Ref != "main" {
		t.Fatalf("unexpected Sync manifest: %#v", manifest)
	}
	if got := manifest.Files["github-workflow"]["ci"]; got != ".github/workflows/project-ci.yaml" {
		t.Fatalf("unexpected target path: %q", got)
	}
}

func TestLoadSyncManifestRejectsInvalidTOMLModel(t *testing.T) {
	for name, input := range map[string]string{
		"unknown root field":       "version = 2\nextra = true\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n",
		"unknown source field":     "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\nextra = true\n",
		"missing source":           "version = 2\n",
		"bad repository":           "version = 2\n[source]\nrepository = \"https://github.com/y-writings/source-repo\"\nref = \"main\"\n",
		"invalid group id":         "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.\"github.workflow\"]\nci = \"ci.yaml\"\n",
		"invalid file id":          "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\n\"bad/id\" = \"ci.yaml\"\n",
		"invalid target path":      "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.github-workflow]\nci = \"../ci.yaml\"\n",
		"duplicate target path":    "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[files.first]\nci = \"./same.yaml\"\n[files.second]\nci = \"same.yaml\"\n",
		"old path_overrides shape": "version = 2\n[source]\nrepository = \"y-writings/source-repo\"\nref = \"main\"\n[[files]]\nid = \"ci\"\npath_overrides = { ci = \"custom.yaml\" }\n",
		"old yaml shape":           "version: 2\nsource:\n  repository: y-writings/source-repo\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSyncManifestBytes([]byte(input))
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadSyncManifestRejectsReservedMetadataPaths(t *testing.T) {
	paths := []string{
		".driftline",
		".driftline/sync.toml",
		".driftline/future/file",
		"./.driftline/future",
		".driftline/./future",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			_, err := LoadSyncManifestBytes([]byte(fmt.Sprintf(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.metadata]
file = %q
`, path)))
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "reserved driftline metadata path") {
				t.Fatalf("expected reserved metadata error containing authored path %q, got %v", path, err)
			}
		})
	}
}

func TestMetadataNearMissesAreOrdinaryPaths(t *testing.T) {
	for _, path := range []string{".driftline-file", ".driftliner/file", "nested/.driftline/file"} {
		t.Run(path, func(t *testing.T) {
			for _, mode := range []FileMode{ModeManaged, ModeTemplate} {
				t.Run("Contract "+string(mode), func(t *testing.T) {
					_, err := LoadContractBytes([]byte(fmt.Sprintf(`version = 2

[files.ordinary]
file = { path = %q, mode = %q }
`, path, mode)))
					if err != nil {
						t.Fatalf("load Contract with ordinary path %q: %v", path, err)
					}
				})
			}

			t.Run("Sync manifest", func(t *testing.T) {
				_, err := LoadSyncManifestBytes([]byte(fmt.Sprintf(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.ordinary]
file = %q
`, path)))
				if err != nil {
					t.Fatalf("load Sync manifest with ordinary path %q: %v", path, err)
				}
			})
		})
	}
}

func TestOldMetadataArtifactNamesAreOrdinaryPaths(t *testing.T) {
	for _, path := range []string{".driftline-source.toml", ".driftline-target.toml", "driftline-lock.yaml"} {
		t.Run(path, func(t *testing.T) {
			for _, mode := range []FileMode{ModeManaged, ModeTemplate} {
				t.Run("Contract "+string(mode), func(t *testing.T) {
					_, err := LoadContractBytes([]byte(fmt.Sprintf(`version = 2

[files.ordinary]
file = { path = %q, mode = %q }
`, path, mode)))
					if err != nil {
						t.Fatalf("load Contract with ordinary old artifact path %q: %v", path, err)
					}
				})
			}

			t.Run("Sync manifest", func(t *testing.T) {
				_, err := LoadSyncManifestBytes([]byte(fmt.Sprintf(`version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.ordinary]
file = %q
`, path)))
				if err != nil {
					t.Fatalf("load Sync manifest with ordinary old artifact path %q: %v", path, err)
				}
			})
		})
	}
}

func TestMetadataErrorsUseContractAndSyncManifestLabels(t *testing.T) {
	tests := []struct {
		name    string
		load    func([]byte) error
		input   string
		wantErr string
	}{
		{
			name: "Contract parse error",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version =",
			wantErr: "parse Contract",
		},
		{
			name: "Contract unknown key",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version = 2\nextra = true\n",
			wantErr: `Contract contains unknown key "extra"`,
		},
		{
			name: "Contract version",
			load: func(data []byte) error {
				_, err := LoadContractBytes(data)
				return err
			},
			input:   "version = 1\n",
			wantErr: "unsupported Contract version 1",
		},
		{
			name: "Sync manifest parse error",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version =",
			wantErr: "parse Sync manifest",
		},
		{
			name: "Sync manifest unknown key",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version = 2\nextra = true\n",
			wantErr: `Sync manifest contains unknown key "extra"`,
		},
		{
			name: "Sync manifest version",
			load: func(data []byte) error {
				_, err := LoadSyncManifestBytes(data)
				return err
			},
			input:   "version = 1\n",
			wantErr: "unsupported Sync manifest version 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.load([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSyncManifestFromContractIncludesManagedFilesOnly(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}

	manifest, err := SyncManifestFromContract("y-writings/source-repo", "main", contract)
	if err != nil {
		t.Fatalf("create Sync manifest failed: %v", err)
	}
	if got := manifest.Files["github-workflow"]["ci"]; got != ".github/workflows/ci.yaml" {
		t.Fatalf("managed file default target mismatch: %q", got)
	}
	if _, ok := manifest.Files["github-workflow"]["release"]; ok {
		t.Fatalf("template file must not be recorded in Sync manifest: %#v", manifest.Files)
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
