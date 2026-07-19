# Contract Gitignore Section Implementation Plan

<!-- markdownlint-disable MD010 MD013 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Contract-owned, marker-delimited generated section to the Target Repository's root `.gitignore`, reconciled by `check`, `diff`, and `update` while preserving target-owned bytes outside the markers.

**Architecture:** Extend Contract version 2 with an optional `[gitignore]` table, but keep the Sync manifest unchanged. Implement marker parsing and byte transformation as pure functions, represent the result as a dedicated `GitIgnoreSectionChange` on `Plan`, and use a separate no-follow, stale-checked, atomic target writer during apply.

**Tech Stack:** Go 1.26, BurntSushi TOML 1.1 parsing, standard-library filesystem APIs, existing `git diff --no-index` command integration, Go tests, Prettier, markdownlint-cli2.

**Design:** `docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md`

---

## File Structure

- Create `src/internal/driftline/gitignore_section.go`: marker constants, exact line parser, renderer, and pure desired-byte transformation.
- Create `src/internal/driftline/gitignore_section_test.go`: byte-level transformation, marker structure, raw entry, provenance, and line-ending tests.
- Create `src/internal/driftline/gitignore_target.go`: no-follow target inspection, planning adapter, stale revalidation, temporary-file preparation, and atomic rename.
- Create `src/internal/driftline/gitignore_target_test.go`: non-regular target, stale-state, mode preservation, and atomic preparation tests.
- Modify `src/internal/driftline/types.go`: Contract Gitignore model, dedicated plan change type, and `Plan` drift support types.
- Modify `src/internal/driftline/config.go`: required-key validation, raw entry validation, and Contract path coexistence validation.
- Modify `src/internal/driftline/config_test.go`: Contract parsing and invalid configuration coverage.
- Modify `src/internal/driftline/plan.go`: resolved Managed collision checks, ownership-transition precedence, and Gitignore section plan integration.
- Modify `src/internal/driftline/plan_test.go`: planner status, filesystem state, collision, source-change, and transition tests.
- Modify `src/internal/driftline/target_repository.go`: prepare the Gitignore write before mutations and commit it before the Sync manifest.
- Modify `src/internal/driftline/target_repository_test.go`: apply ordering, stale abort, failure, and no-rollback tests.
- Modify `src/internal/driftline/commands/check.go`: print the complete plan and include Gitignore drift.
- Modify `src/internal/driftline/commands/diff.go`: render the dedicated full-file Gitignore diff.
- Modify `src/internal/driftline/commands/update.go`: print the complete plan before conflict return and after apply.
- Modify `src/internal/driftline/commands/run.go`: describe the expanded command responsibilities in help output.
- Modify `src/internal/driftline/commands/commands_test.go`: `init`, `check`, `diff`, `update`, malformed-marker, and binary-diff integration tests.
- Modify `README.md`: document Contract syntax, generated format, command lifecycle, and ownership boundaries.
- Modify `CONTEXT.md`: add Gitignore section domain terminology.
- Modify `AGENTS.md`: name the focused Gitignore design as canonical for this behavior.
- Modify `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`: cross-reference the focused spec where the old root behavior was removed.

## Task 1: Parse And Validate Contract Gitignore Configuration

**Files:**

- Create: `src/internal/driftline/gitignore_section.go`
- Modify: `src/internal/driftline/types.go:12-20`
- Modify: `src/internal/driftline/config.go:14-31,147-188`
- Modify: `src/internal/driftline/config_test.go:9-106`

- [ ] **Step 1: Write failing Contract parsing tests**

Add these tests to `config_test.go`:

```go
func TestLoadContractGitIgnore(t *testing.T) {
	contract, err := LoadContractBytes([]byte(`version = 2

[gitignore]
entries = [".env", "/dist/", "!/dist/.gitkeep", "", "  spaced  ", "# DO NOT EDIT: this section is managed automatically by driftline."]

[files.templates]
ignore = { path = ".gitignore", mode = "template" }
`))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	want := []string{".env", "/dist/", "!/dist/.gitkeep", "", "  spaced  ", "# DO NOT EDIT: this section is managed automatically by driftline."}
	if contract.GitIgnore == nil || !slices.Equal(contract.GitIgnore.Entries, want) {
		t.Fatalf("unexpected gitignore entries: %#v", contract.GitIgnore)
	}
	if contract.Version != 2 {
		t.Fatalf("Contract version changed: %d", contract.Version)
	}
}

func TestLoadContractRejectsInvalidGitIgnore(t *testing.T) {
	tests := map[string]string{
		"missing entries": `version = 2
[gitignore]
`,
		"unknown field": `version = 2
[gitignore]
entries = [".env"]
extra = true
`,
		"multiline entry": `version = 2
[gitignore]
entries = ["first\nsecond"]
`,
		"end marker entry": `version = 2
[gitignore]
entries = ["# end driftline"]
`,
		"start marker entry": `version = 2
[gitignore]
entries = ["# start driftline from other/repo/.driftline/contract.toml"]
`,
		"managed root gitignore": `version = 2
[gitignore]
entries = [".env"]
[files.tool]
ignore = { path = ".gitignore", mode = "managed" }
`,
		"template descendant": `version = 2
[gitignore]
entries = [".env"]
[files.tool]
ignore = { path = ".gitignore/rules", mode = "template" }
`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadContractBytes([]byte(input)); err == nil {
				t.Fatal("expected Contract validation error")
			}
		})
	}
}

func TestLoadContractAcceptsExplicitEmptyGitIgnore(t *testing.T) {
	contract, err := LoadContractBytes([]byte("version = 2\n[gitignore]\nentries = []\n"))
	if err != nil {
		t.Fatalf("load Contract failed: %v", err)
	}
	if contract.GitIgnore == nil || len(contract.GitIgnore.Entries) != 0 {
		t.Fatalf("explicit empty entries were not preserved: %#v", contract.GitIgnore)
	}
}
```

Add `slices` to the test imports.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestLoadContract(GitIgnore|RejectsInvalidGitIgnore|AcceptsExplicitEmptyGitIgnore)$' -count=1
```

Expected: compilation fails because `Contract.GitIgnore` and `ContractGitIgnore` do not exist.

- [ ] **Step 3: Add the Contract model and marker identity helpers**

Add to `types.go`:

```go
type Contract struct {
	Version   int                                `toml:"version"`
	GitIgnore *ContractGitIgnore                 `toml:"gitignore"`
	Files     map[string]map[string]ContractFile `toml:"files"`
}

type ContractGitIgnore struct {
	Entries []string `toml:"entries"`
}
```

Create `gitignore_section.go` with the stable marker vocabulary used by both validation and later transformation:

```go
package driftline

import (
	"strings"
)

const (
	GitIgnorePath         = ".gitignore"
	gitIgnoreStartPrefix  = "# start driftline from "
	gitIgnoreContractTail = "/" + ContractPath
	gitIgnoreWarning      = "# DO NOT EDIT: this section is managed automatically by driftline."
	gitIgnoreEndMarker    = "# end driftline"
)

func gitIgnoreStartRepository(line []byte) (string, bool) {
	text := string(line)
	if !strings.HasPrefix(text, gitIgnoreStartPrefix) || !strings.HasSuffix(text, gitIgnoreContractTail) {
		return "", false
	}
	repository := strings.TrimSuffix(strings.TrimPrefix(text, gitIgnoreStartPrefix), gitIgnoreContractTail)
	if ValidateRepository(repository) != nil {
		return "", false
	}
	return repository, true
}

func isGitIgnoreStartMarker(line []byte) bool {
	_, ok := gitIgnoreStartRepository(line)
	return ok
}
```

- [ ] **Step 4: Validate authored presence, entries, and direct Contract paths**

In `LoadContractBytes`, use TOML metadata to distinguish a missing key from an explicit empty array before calling `validateContract`:

```go
	if metadata.IsDefined("gitignore") && !metadata.IsDefined("gitignore", "entries") {
		return contract, errors.New("Contract gitignore must define entries")
	}
```

At the end of `validateContract`, call this new helper:

```go
func validateContractGitIgnore(contract Contract) error {
	if contract.GitIgnore == nil {
		return nil
	}
	for i, entry := range contract.GitIgnore.Entries {
		if strings.ContainsAny(entry, "\r\n") {
			return fmt.Errorf("Contract gitignore entry %d must be a single line", i)
		}
		if entry == gitIgnoreEndMarker || isGitIgnoreStartMarker([]byte(entry)) {
			return fmt.Errorf("Contract gitignore entry %d conflicts with a driftline marker", i)
		}
	}
	for _, entry := range ContractEntries(contract) {
		if entry.Mode == ModeManaged && entry.Path == GitIgnorePath {
			return fmt.Errorf("Contract file %q cannot be managed at %s while [gitignore] is present", entry.Key, GitIgnorePath)
		}
		if len(contract.GitIgnore.Entries) > 0 && isPathAncestor(GitIgnorePath, entry.Path) {
			return fmt.Errorf("Contract file %q path conflicts with generated %s: %s", entry.Key, GitIgnorePath, entry.Path)
		}
	}
	return nil
}
```

Return `validateContractGitIgnore(contract)` after the existing file loop. Keep exact Template `.gitignore` valid.

- [ ] **Step 5: Format and run the focused and package tests**

Run:

```bash
gofmt -w src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/config_test.go src/internal/driftline/gitignore_section.go
go test ./src/internal/driftline -run 'TestLoadContract' -count=1
go test ./src/internal/driftline -count=1
```

Expected: all commands pass.

- [ ] **Step 6: Commit the Contract schema slice**

```bash
git add src/internal/driftline/types.go src/internal/driftline/config.go src/internal/driftline/config_test.go src/internal/driftline/gitignore_section.go
git commit -m "feat: parse contract gitignore entries"
```

## Task 2: Implement Pure Marker Parsing And Byte Transformation

**Files:**

- Modify: `src/internal/driftline/gitignore_section.go`
- Create: `src/internal/driftline/gitignore_section_test.go`

- [ ] **Step 1: Write table-driven transformation tests**

Create `gitignore_section_test.go` with these core cases:

```go
package driftline

import (
	"bytes"
	"strings"
	"testing"
)

func TestTransformGitIgnoreSection(t *testing.T) {
	lfBlock := "# start driftline from y-writings/source-repo/.driftline/contract.toml\n" +
		gitIgnoreWarning + "\n.env\n/dist/\n" + gitIgnoreEndMarker + "\n"
	tests := []struct {
		name       string
		current    []byte
		missing    bool
		config     *ContractGitIgnore
		repository string
		wantStatus Status
		want       []byte
	}{
		{
			name:       "create missing file",
			missing:    true,
			config:     &ContractGitIgnore{Entries: []string{".env", "/dist/"}},
			repository: "y-writings/source-repo",
			wantStatus: StatusAdd,
			want:       []byte(lfBlock),
		},
		{
			name:       "append without deduplicating target-owned line",
			current:    []byte(".env\n"),
			config:     &ContractGitIgnore{Entries: []string{".env", "/dist/"}},
			repository: "y-writings/source-repo",
			wantStatus: StatusAdd,
			want:       []byte(".env\n\n" + lfBlock),
		},
		{
			name:       "whitespace-only line is not an empty separator",
			current:    []byte("base\n \n"),
			config:     &ContractGitIgnore{Entries: []string{".env", "/dist/"}},
			repository: "y-writings/source-repo",
			wantStatus: StatusAdd,
			want:       []byte("base\n \n\n" + lfBlock),
		},
		{
			name: "replace in place and update provenance with CRLF",
			current: []byte("before\r\n# start driftline from old/source/.driftline/contract.toml\r\n" +
				"edited\r\n# end driftline\r\nafter\r\n"),
			config:     &ContractGitIgnore{Entries: []string{".env"}},
			repository: "new/source",
			wantStatus: StatusUpdate,
			want: []byte("before\r\n# start driftline from new/source/.driftline/contract.toml\r\n" +
				gitIgnoreWarning + "\r\n.env\r\n# end driftline\r\nafter\r\n"),
		},
		{
			name:       "remove block but keep separator",
			current:    []byte("base\n\n" + lfBlock),
			config:     nil,
			repository: "y-writings/source-repo",
			wantStatus: StatusRemove,
			want:       []byte("base\n\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformGitIgnoreSection(tt.current, tt.missing, tt.repository, tt.config)
			if err != nil {
				t.Fatalf("transform failed: %v", err)
			}
			if !got.Changed || got.Status != tt.wantStatus || !bytes.Equal(got.DesiredBytes, tt.want) {
				t.Fatalf("unexpected transform: %#v\nwant bytes %q\ngot bytes  %q", got, tt.want, got.DesiredBytes)
			}
		})
	}
}

func TestTransformGitIgnoreSectionPreservesRawEntriesAndMixedLineEndings(t *testing.T) {
	current := []byte("first\r\nsecond\n")
	config := &ContractGitIgnore{Entries: []string{"dup", "dup", "", "  spaced  ", "# comment", "!keep"}}
	got, err := transformGitIgnoreSection(current, false, "y-writings/source-repo", config)
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := "# start driftline from y-writings/source-repo/.driftline/contract.toml\r\n" +
		gitIgnoreWarning + "\r\ndup\r\ndup\r\n\r\n  spaced  \r\n# comment\r\n!keep\r\n# end driftline\r\n"
	if !bytes.HasPrefix(got.DesiredBytes, current) || !strings.HasSuffix(string(got.DesiredBytes), wantSuffix) {
		t.Fatalf("raw entries or mixed line ending changed: %q", got.DesiredBytes)
	}
}
```

Add the remaining byte-boundary cases:

```go
func TestTransformGitIgnoreSectionNoChangeAndBytePreservation(t *testing.T) {
	block := []byte("# start driftline from y-writings/source-repo/.driftline/contract.toml\n" + gitIgnoreWarning + "\n.env\n# end driftline\n")
	got, err := transformGitIgnoreSection(block, false, "y-writings/source-repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil || got.Changed {
		t.Fatalf("matching block should not drift: %#v err=%v", got, err)
	}

	invalidUTF8 := []byte{0xff, '\n'}
	got, err = transformGitIgnoreSection(invalidUTF8, false, "y-writings/source-repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.DesiredBytes[:len(invalidUTF8)], invalidUTF8) {
		t.Fatalf("target-owned invalid UTF-8 changed: %v", got.DesiredBytes)
	}

	unterminated := []byte("# start driftline from y-writings/source-repo/.driftline/contract.toml\nold\n# end driftline")
	got, err = transformGitIgnoreSection(unterminated, false, "y-writings/source-repo", &ContractGitIgnore{Entries: []string{"new"}})
	if err != nil || !bytes.HasSuffix(got.DesiredBytes, []byte("new\n# end driftline\n")) {
		t.Fatalf("unterminated block was not replaced correctly: %q err=%v", got.DesiredBytes, err)
	}

	for _, config := range []*ContractGitIgnore{nil, {Entries: []string{}}} {
		got, err = transformGitIgnoreSection([]byte("target-owned\n"), false, "y-writings/source-repo", config)
		if err != nil || got.Changed {
			t.Fatalf("absent or empty config without markers should not drift: %#v err=%v", got, err)
		}
	}

	nearMisses := []byte(" # start driftline from other/repo/.driftline/contract.toml\n# end driftline \n")
	got, err = transformGitIgnoreSection(nearMisses, false, "y-writings/source-repo", &ContractGitIgnore{Entries: []string{".env"}})
	if err != nil || !bytes.HasPrefix(got.DesiredBytes, nearMisses) {
		t.Fatalf("non-matching marker-like comments were not preserved: %q err=%v", got.DesiredBytes, err)
	}
}
```

- [ ] **Step 2: Write failing malformed-marker tests**

Add:

```go
func TestTransformGitIgnoreSectionRejectsMalformedMarkers(t *testing.T) {
	tests := map[string]string{
		"missing end":    "# start driftline from y-writings/source-repo/.driftline/contract.toml\nvalue\n",
		"missing start":  "value\n# end driftline\n",
		"reversed":       "# end driftline\n# start driftline from y-writings/source-repo/.driftline/contract.toml\n",
		"nested":         "# start driftline from y-writings/source-repo/.driftline/contract.toml\n# start driftline from other/repo/.driftline/contract.toml\n# end driftline\n",
		"duplicate block": "# start driftline from one/repo/.driftline/contract.toml\n# end driftline\n# start driftline from two/repo/.driftline/contract.toml\n# end driftline\n",
	}
	for name, current := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := transformGitIgnoreSection([]byte(current), false, "y-writings/source-repo", &ContractGitIgnore{Entries: []string{".env"}})
			if err == nil || !strings.Contains(err.Error(), "invalid driftline section in .gitignore") {
				t.Fatalf("expected structural error, got %v", err)
			}
		})
	}
}
```

- [ ] **Step 3: Run the transformation tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestTransformGitIgnoreSection' -count=1
```

Expected: compilation fails because `transformGitIgnoreSection` is undefined.

- [ ] **Step 4: Implement exact byte-line parsing and structural validation**

Add these internal types and functions to `gitignore_section.go`:

```go
type gitIgnoreLine struct {
	start      int
	contentEnd int
	end        int
	ending     []byte
}

type gitIgnoreSection struct {
	start  int
	end    int
	ending []byte
}

func splitGitIgnoreLines(data []byte) []gitIgnoreLine {
	lines := []gitIgnoreLine{}
	for start := 0; start < len(data); {
		relativeLF := bytes.IndexByte(data[start:], '\n')
		if relativeLF < 0 {
			lines = append(lines, gitIgnoreLine{start: start, contentEnd: len(data), end: len(data)})
			break
		}
		lf := start + relativeLF
		contentEnd := lf
		endingStart := lf
		if contentEnd > start && data[contentEnd-1] == '\r' {
			contentEnd--
			endingStart = contentEnd
		}
		lines = append(lines, gitIgnoreLine{
			start:      start,
			contentEnd: contentEnd,
			end:        lf + 1,
			ending:     append([]byte(nil), data[endingStart:lf+1]...),
		})
		start = lf + 1
	}
	return lines
}

func parseGitIgnoreSection(data []byte) (*gitIgnoreSection, error) {
	starts := []gitIgnoreLine{}
	ends := []gitIgnoreLine{}
	for _, line := range splitGitIgnoreLines(data) {
		content := data[line.start:line.contentEnd]
		if isGitIgnoreStartMarker(content) {
			starts = append(starts, line)
		}
		if bytes.Equal(content, []byte(gitIgnoreEndMarker)) {
			ends = append(ends, line)
		}
	}
	switch {
	case len(starts) == 0 && len(ends) == 0:
		return nil, nil
	case len(starts) == 0:
		return nil, fmt.Errorf("invalid driftline section in %s: end marker without start marker", GitIgnorePath)
	case len(ends) == 0:
		return nil, fmt.Errorf("invalid driftline section in %s: start marker without end marker", GitIgnorePath)
	case len(starts) > 1:
		return nil, fmt.Errorf("invalid driftline section in %s: multiple start markers", GitIgnorePath)
	case len(ends) > 1:
		return nil, fmt.Errorf("invalid driftline section in %s: multiple end markers", GitIgnorePath)
	case starts[0].start > ends[0].start:
		return nil, fmt.Errorf("invalid driftline section in %s: end marker precedes start marker", GitIgnorePath)
	}
	return &gitIgnoreSection{start: starts[0].start, end: ends[0].end, ending: starts[0].ending}, nil
}
```

Add `bytes` and `fmt` imports. Lone CR remains content because only LF delimits lines.

- [ ] **Step 5: Implement rendering, separator selection, and transformation**

Add:

```go
type gitIgnoreTransform struct {
	Changed      bool
	Status       Status
	Reason       string
	DesiredBytes []byte
}

func transformGitIgnoreSection(current []byte, targetMissing bool, repository string, config *ContractGitIgnore) (gitIgnoreTransform, error) {
	section, err := parseGitIgnoreSection(current)
	if err != nil {
		return gitIgnoreTransform{}, err
	}
	entries := []string(nil)
	if config != nil {
		entries = config.Entries
	}
	if len(entries) == 0 {
		if section == nil {
			return gitIgnoreTransform{}, nil
		}
		desired := append([]byte(nil), current[:section.start]...)
		desired = append(desired, current[section.end:]...)
		return gitIgnoreTransform{Changed: true, Status: StatusRemove, Reason: "generated section is no longer declared", DesiredBytes: desired}, nil
	}

	ending := firstGitIgnoreLineEnding(current)
	status := StatusAdd
	reason := "generated section is missing"
	if section != nil {
		ending = section.ending
		status = StatusUpdate
		reason = "generated section differs"
	}
	if len(ending) == 0 {
		ending = []byte{'\n'}
	}
	generated := renderGitIgnoreSection(repository, entries, ending)
	var desired []byte
	if section == nil {
		desired = appendGeneratedGitIgnoreSection(current, generated, ending)
	} else {
		desired = append([]byte(nil), current[:section.start]...)
		desired = append(desired, generated...)
		desired = append(desired, current[section.end:]...)
	}
	if bytes.Equal(current, desired) && !targetMissing {
		return gitIgnoreTransform{}, nil
	}
	return gitIgnoreTransform{Changed: true, Status: status, Reason: reason, DesiredBytes: desired}, nil
}

func renderGitIgnoreSection(repository string, entries []string, ending []byte) []byte {
	lines := []string{gitIgnoreStartPrefix + repository + gitIgnoreContractTail, gitIgnoreWarning}
	lines = append(lines, entries...)
	lines = append(lines, gitIgnoreEndMarker)
	return []byte(strings.Join(lines, string(ending)) + string(ending))
}

func firstGitIgnoreLineEnding(data []byte) []byte {
	for _, line := range splitGitIgnoreLines(data) {
		if len(line.ending) > 0 {
			return line.ending
		}
	}
	return nil
}

func appendGeneratedGitIgnoreSection(current, generated, ending []byte) []byte {
	if len(current) == 0 {
		return append([]byte(nil), generated...)
	}
	desired := append([]byte(nil), current...)
	lines := splitGitIgnoreLines(current)
	last := lines[len(lines)-1]
	if len(last.ending) == 0 {
		desired = append(desired, ending...)
		desired = append(desired, ending...)
	} else if last.contentEnd > last.start {
		desired = append(desired, ending...)
	}
	return append(desired, generated...)
}
```

Add `strings` to the imports already used by marker recognition.

- [ ] **Step 6: Format and run pure transformation tests**

Run:

```bash
gofmt -w src/internal/driftline/gitignore_section.go src/internal/driftline/gitignore_section_test.go
go test ./src/internal/driftline -run 'TestTransformGitIgnoreSection' -count=1
go test ./src/internal/driftline -count=1
```

Expected: all commands pass.

- [ ] **Step 7: Commit pure Gitignore transformation**

```bash
git add src/internal/driftline/gitignore_section.go src/internal/driftline/gitignore_section_test.go
git commit -m "feat: transform generated gitignore section"
```

## Task 3: Add Gitignore Section Changes To The Sync Plan

**Files:**

- Create: `src/internal/driftline/gitignore_target.go`
- Modify: `src/internal/driftline/types.go:48-72`
- Modify: `src/internal/driftline/plan.go:18-35,88-193,365-372`
- Modify: `src/internal/driftline/plan_test.go`

- [ ] **Step 1: Write failing basic planner tests**

Add these tests to `plan_test.go`:

```go
func TestBuildPlanAddsMissingGitIgnoreSection(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	client := newPlanSourceClient("version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	change := plan.GitIgnore
	if change == nil || change.Status != StatusAdd || !change.TargetMissing || change.Reason != "generated section is missing" {
		t.Fatalf("unexpected Gitignore change: %#v", change)
	}
	if !bytes.Contains(change.DesiredBytes, []byte(".env\n# end driftline\n")) || !plan.HasDrift() {
		t.Fatalf("missing desired Gitignore drift: %#v", change)
	}
}

func TestBuildPlanRemovesUndeclaredGitIgnoreSection(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	writePlanFile(t, targetDir, GitIgnorePath, "base\n\n# start driftline from old/source/.driftline/contract.toml\n"+gitIgnoreWarning+"\n.env\n# end driftline\n")
	client := newPlanSourceClient("version = 2\n", nil)

	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if plan.GitIgnore == nil || plan.GitIgnore.Status != StatusRemove || string(plan.GitIgnore.DesiredBytes) != "base\n\n" {
		t.Fatalf("unexpected Gitignore removal: %#v", plan.GitIgnore)
	}
}

func TestBuildPlanRejectsUnsupportedGitIgnoreTargetForNonEmptyEntries(t *testing.T) {
	for _, state := range []string{"directory", "symlink", "broken symlink"} {
		t.Run(state, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
			path := filepath.Join(targetDir, GitIgnorePath)
			switch state {
			case "directory":
				if err := os.Mkdir(path, 0o755); err != nil { t.Fatal(err) }
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil { t.Fatal(err) }
				if err := os.Symlink(outside, path); err != nil { t.Fatal(err) }
			case "broken symlink":
				if err := os.Symlink("missing", path); err != nil { t.Fatal(err) }
			}
			client := newPlanSourceClient("version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)
			_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
			if err == nil || !strings.Contains(err.Error(), ".gitignore is not a regular file") {
				t.Fatalf("expected unsupported target error, got %v", err)
			}
		})
	}
}
```

Add `bytes` to the imports. Also add the inactive non-regular case:

```go
func TestBuildPlanIgnoresNonRegularGitIgnoreWithoutEntries(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(targetDir, GitIgnorePath)); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: newPlanSourceClient("version = 2\n", nil)})
	if err != nil {
		t.Fatalf("inactive Gitignore section should ignore symlink: %v", err)
	}
	if plan.GitIgnore != nil || plan.HasDrift() {
		t.Fatalf("inactive non-regular target should not drift: %#v", plan)
	}
}

func TestBuildPlanRejectsResolvedManagedGitIgnoreDescendant(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML("[files.tool]\nconfig = \".gitignore/rules\"\n"))
	client := newPlanSourceClient(`version = 2
[gitignore]
entries = [".env"]
[files.tool]
config = { path = "source.txt", mode = "managed" }
`, map[string]string{"source.txt": "source\n"})
	_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
	if err == nil || !strings.Contains(err.Error(), "conflicts with generated .gitignore") {
		t.Fatalf("expected resolved descendant collision, got %v", err)
	}
}

func TestBuildPlanRejectsUnreadableRegularGitIgnore(t *testing.T) {
	targetDir := t.TempDir()
	writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML(""))
	path := filepath.Join(targetDir, GitIgnorePath)
	writePlanFile(t, targetDir, GitIgnorePath, "target-owned\n")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	client := newPlanSourceClient("version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)
	if _, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client}); err == nil || !strings.Contains(err.Error(), "read .gitignore") {
		t.Fatalf("expected unreadable target error, got %v", err)
	}
}
```

- [ ] **Step 2: Run the planner tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestBuildPlan(AddsMissingGitIgnoreSection|RemovesUndeclaredGitIgnoreSection|RejectsUnsupportedGitIgnoreTargetForNonEmptyEntries)$' -count=1
```

Expected: compilation fails because `Plan.GitIgnore`, `GitIgnoreSectionChange`, and `Plan.HasDrift` do not exist.

- [ ] **Step 3: Add the dedicated plan change type and drift method**

Add to `types.go`:

```go
type GitIgnoreSectionChange struct {
	Status        Status
	Reason        string
	TargetPath    string
	TargetMissing bool
	OriginalBytes []byte
	DesiredBytes  []byte
}
```

Add `GitIgnore *GitIgnoreSectionChange` to `Plan`. Add a plan-level method while retaining the existing slice helper until command integration is complete:

```go
func (p Plan) HasDrift() bool {
	if p.GitIgnore != nil {
		return true
	}
	return HasDrift(p.Changes)
}
```

Update the existing planner test that calls `HasDrift(plan.Changes)` to call `plan.HasDrift()`.

- [ ] **Step 4: Implement no-follow planning inspection**

Create `gitignore_target.go`:

```go
package driftline

import (
	"errors"
	"fmt"
	"os"
)

type gitIgnoreTargetState struct {
	missing bool
	regular bool
	path    string
	bytes   []byte
}

func inspectGitIgnoreTarget(root string, requireRegular bool) (gitIgnoreTargetState, error) {
	path, err := PathWithin(root, GitIgnorePath, "Gitignore target")
	if err != nil {
		return gitIgnoreTargetState{}, err
	}
	state := gitIgnoreTargetState{path: path}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		state.missing = true
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("inspect %s: %w", GitIgnorePath, err)
	}
	if !info.Mode().IsRegular() {
		if requireRegular {
			return state, fmt.Errorf("%s is not a regular file", GitIgnorePath)
		}
		return state, nil
	}
	state.regular = true
	state.bytes, err = os.ReadFile(path)
	if err != nil {
		return state, fmt.Errorf("read %s: %w", GitIgnorePath, err)
	}
	return state, nil
}

func planGitIgnoreSectionChange(root, repository string, config *ContractGitIgnore, replaceAfterManagedDelete bool) (*GitIgnoreSectionChange, error) {
	requireRegular := config != nil && len(config.Entries) > 0
	state, err := inspectGitIgnoreTarget(root, requireRegular)
	if err != nil {
		return nil, err
	}
	if !state.missing && !state.regular {
		return nil, nil
	}
	transformBytes := state.bytes
	transformMissing := state.missing
	if replaceAfterManagedDelete {
		transformBytes = nil
		transformMissing = true
	}
	result, err := transformGitIgnoreSection(transformBytes, transformMissing, repository, config)
	if err != nil {
		return nil, err
	}
	if !result.Changed {
		return nil, nil
	}
	return &GitIgnoreSectionChange{
		Status:        result.Status,
		Reason:        result.Reason,
		TargetPath:    state.path,
		TargetMissing: state.missing,
		OriginalBytes: append([]byte(nil), state.bytes...),
		DesiredBytes:  result.DesiredBytes,
	}, nil
}
```

- [ ] **Step 5: Integrate ordinary Gitignore planning**

In `planBuilder.build`, track whether a desired Managed target resolves to `.gitignore`. Reject coexistence and invoke `planGitIgnoreSectionChange` after Managed removals are assembled. Task 4 then adds the approved transition precedence:

```go
	desiredManagedGitIgnore := false
```

Inside the resolved Managed loop, after choosing `resolved.target`:

```go
		if b.contract.GitIgnore != nil && resolved.target == GitIgnorePath {
			return Plan{}, fmt.Errorf("managed target %q cannot use %s while [gitignore] is present", resolved.Key, GitIgnorePath)
		}
		if b.contract.GitIgnore != nil && len(b.contract.GitIgnore.Entries) > 0 && isPathAncestor(GitIgnorePath, resolved.target) {
			return Plan{}, fmt.Errorf("managed target %q conflicts with generated %s: %s", resolved.Key, GitIgnorePath, resolved.target)
		}
		if resolved.target == GitIgnorePath {
			desiredManagedGitIgnore = true
		}
```

After stale Managed changes are appended:

```go
	if !desiredManagedGitIgnore {
		gitIgnoreChange, err := planGitIgnoreSectionChange(b.opts.TargetDir, b.syncManifest.Source.Repository, b.contract.GitIgnore, false)
		if err != nil {
			return Plan{}, err
		}
		plan.GitIgnore = gitIgnoreChange
	}
	if len(plan.Changes) == 0 && plan.GitIgnore == nil {
		plan.Changes = append(plan.Changes, Change{Status: StatusSynced})
	}
```

Remove the old unconditional synced sentinel block. Keep `HasDrift(changes []Change)` so command packages compile until Task 6 switches them to `Plan.HasDrift`.

- [ ] **Step 6: Format and run planner tests**

Run:

```bash
gofmt -w src/internal/driftline/types.go src/internal/driftline/plan.go src/internal/driftline/plan_test.go src/internal/driftline/gitignore_target.go
go test ./src/internal/driftline -run 'TestBuildPlan' -count=1
go test ./src/internal/driftline -count=1
go test ./... -count=1
```

Expected: all commands pass.

- [ ] **Step 7: Commit basic planner integration**

```bash
git add src/internal/driftline/types.go src/internal/driftline/plan.go src/internal/driftline/plan_test.go src/internal/driftline/gitignore_target.go
git commit -m "feat: plan gitignore section changes"
```

## Task 4: Implement Ownership Transition Precedence

**Files:**

- Modify: `src/internal/driftline/plan.go:88-193`
- Modify: `src/internal/driftline/plan_test.go`

- [ ] **Step 1: Write the five transition-matrix tests**

Add one table-driven test to `plan_test.go`. Each case writes an existing Sync manifest mapping `old.ignore` to `.gitignore`, writes the current `.gitignore`, builds the next Contract, and asserts the Managed removal plus dedicated change combination:

```go
func TestBuildPlanGitIgnoreOwnershipTransitions(t *testing.T) {
	tests := []struct {
		name               string
		contract           string
		files              map[string]string
		forceKey           string
		wantManagedStatus  Status
		wantGitIgnore      bool
		wantGitIgnoreBytes string
	}{
		{
			name: "section to managed uses whole file only",
			contract: `version = 2
[files.new]
ignore = { path = ".gitignore", mode = "managed" }
`,
			files:             map[string]string{".gitignore": "managed\n"},
			forceKey:          "new.ignore",
			wantManagedStatus: StatusUpdate,
		},
		{
			name: "managed removed to non-empty section deletes then recreates",
			contract: `version = 2
[gitignore]
entries = [".env"]
`,
			wantManagedStatus:  StatusRemove,
			wantGitIgnore:      true,
			wantGitIgnoreBytes: "# start driftline from y-writings/source-repo/.driftline/contract.toml\n" + gitIgnoreWarning + "\n.env\n# end driftline\n",
		},
		{
			name:              "managed removed to absent section deletes only",
			contract:          "version = 2\n",
			wantManagedStatus: StatusRemove,
		},
		{
			name: "managed to template plus section preserves then appends",
			contract: `version = 2
[gitignore]
entries = [".env"]
[files.old]
ignore = { path = ".gitignore", mode = "template" }
`,
			wantManagedStatus:  StatusModeTransition,
			wantGitIgnore:      true,
			wantGitIgnoreBytes: "managed-before\n\n# start driftline from y-writings/source-repo/.driftline/contract.toml\n" + gitIgnoreWarning + "\n.env\n# end driftline\n",
		},
		{
			name: "managed to template without section leaves file untouched",
			contract: `version = 2
[files.old]
ignore = { path = ".gitignore", mode = "template" }
`,
			wantManagedStatus: StatusModeTransition,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML("[files.old]\nignore = \".gitignore\"\n"))
			writePlanFile(t, targetDir, GitIgnorePath, "managed-before\n")
			client := newPlanSourceClient(tt.contract, tt.files)
			plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client, ForceKey: tt.forceKey})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}
			if tt.wantManagedStatus != "" {
				_ = planChange(t, plan, tt.wantManagedStatus, map[Status]string{StatusUpdate: "new.ignore", StatusRemove: "old.ignore", StatusModeTransition: "old.ignore"}[tt.wantManagedStatus])
			}
			if (plan.GitIgnore != nil) != tt.wantGitIgnore {
				t.Fatalf("unexpected Gitignore change: %#v", plan.GitIgnore)
			}
			if tt.wantGitIgnore && string(plan.GitIgnore.DesiredBytes) != tt.wantGitIgnoreBytes {
				t.Fatalf("unexpected desired Gitignore bytes: %q", plan.GitIgnore.DesiredBytes)
			}
		})
	}
}
```

Add explicit coexistence tests:

```go
func TestBuildPlanGitIgnoreCoexistenceUsesDesiredManagedSet(t *testing.T) {
	t.Run("stale mapping scheduled for removal is allowed", func(t *testing.T) {
		targetDir := t.TempDir()
		writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML("[files.old]\nignore = \".gitignore\"\n"))
		writePlanFile(t, targetDir, GitIgnorePath, "managed-before\n")
		client := newPlanSourceClient("version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)
		plan, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
		if err != nil {
			t.Fatalf("stale mapping should transition: %v", err)
		}
		if plan.GitIgnore == nil || plan.GitIgnore.Status != StatusAdd {
			t.Fatalf("missing generated-only replacement: %#v", plan.GitIgnore)
		}
	})

	t.Run("desired Managed override is rejected", func(t *testing.T) {
		targetDir := t.TempDir()
		writePlanFile(t, targetDir, SyncManifestPath, syncManifestTOML("[files.tool]\nconfig = \".gitignore\"\n"))
		client := newPlanSourceClient(`version = 2
[gitignore]
entries = [".env"]
[files.tool]
config = { path = "source.txt", mode = "managed" }
`, map[string]string{"source.txt": "source\n"})
		_, err := BuildPlan(PlanOptions{TargetDir: targetDir, Source: client})
		if err == nil || !strings.Contains(err.Error(), "cannot use .gitignore") {
			t.Fatalf("expected desired Managed coexistence error, got %v", err)
		}
	})
}
```

- [ ] **Step 2: Run the transition test and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestBuildPlanGitIgnoreOwnershipTransitions' -count=1
```

Expected: at least the Managed-to-section and Managed-to-Template cases fail because generic section planning uses the wrong pre-transition bytes or removes a section during a leave-untouched transition.

- [ ] **Step 3: Classify current `.gitignore` Managed ownership before generic section planning**

After building `contractByKey` and `syncByKey`, derive:

```go
	currentGitIgnoreKey := declaredTargets[GitIgnorePath]
	managedGitIgnoreRemoved := false
	managedGitIgnoreToTemplate := false
	if currentGitIgnoreKey != "" {
		source, exists := contractByKey[currentGitIgnoreKey]
		managedGitIgnoreRemoved = !exists
		managedGitIgnoreToTemplate = exists && source.Mode == ModeTemplate
	}
	gitIgnoreEntriesNonEmpty := b.contract.GitIgnore != nil && len(b.contract.GitIgnore.Entries) > 0
```

Replace the ordinary call added in Task 3 with explicit precedence:

```go
	if !desiredManagedGitIgnore {
		skipGitIgnorePlan := (managedGitIgnoreRemoved || managedGitIgnoreToTemplate) && !gitIgnoreEntriesNonEmpty
		if !skipGitIgnorePlan {
			gitIgnoreChange, err := planGitIgnoreSectionChange(
				b.opts.TargetDir,
				b.syncManifest.Source.Repository,
				b.contract.GitIgnore,
				managedGitIgnoreRemoved && gitIgnoreEntriesNonEmpty,
			)
			if err != nil {
				return Plan{}, err
			}
			plan.GitIgnore = gitIgnoreChange
		}
	}
```

Keep coexistence validation scoped to desired Managed files, so `currentGitIgnoreKey` may be removed or transition to Template.

- [ ] **Step 4: Run transition and full package tests**

Run:

```bash
gofmt -w src/internal/driftline/plan.go src/internal/driftline/plan_test.go
go test ./src/internal/driftline -run 'TestBuildPlanGitIgnore' -count=1
go test ./src/internal/driftline -count=1
```

Expected: all commands pass.

- [ ] **Step 5: Commit transition handling**

```bash
git add src/internal/driftline/plan.go src/internal/driftline/plan_test.go
git commit -m "feat: handle gitignore ownership transitions"
```

## Task 5: Prepare And Apply Atomic Gitignore Rewrites

**Files:**

- Modify: `src/internal/driftline/gitignore_target.go`
- Create: `src/internal/driftline/gitignore_target_test.go`
- Modify: `src/internal/driftline/target_repository.go:10-53`
- Modify: `src/internal/driftline/target_repository_test.go`

- [ ] **Step 1: Write failing stale-state and permission tests**

Create `gitignore_target_test.go`:

```go
package driftline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareGitIgnoreRewriteRejectsStaleTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, GitIgnorePath)
	if err := os.WriteFile(path, []byte("current\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	change := GitIgnoreSectionChange{TargetPath: path, OriginalBytes: []byte("planned\n"), DesiredBytes: []byte("desired\n")}
	_, _, err := PrepareGitIgnoreRewrite(change)
	if err == nil || !strings.Contains(err.Error(), "stale .gitignore") {
		t.Fatalf("expected stale error, got %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "current\n" {
		t.Fatalf("stale target changed: %q err=%v", got, err)
	}
}

func TestPrepareGitIgnoreRewriteCommitsAtomicallyAndPreservesMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, GitIgnorePath)
	if err := os.WriteFile(path, []byte("current\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	commit, cleanup, err := PrepareGitIgnoreRewrite(GitIgnoreSectionChange{
		TargetPath: path, OriginalBytes: []byte("current\n"), DesiredBytes: []byte("desired\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got, err := os.ReadFile(path); err != nil || string(got) != "current\n" {
		t.Fatalf("prepare changed target: %q err=%v", got, err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode was not preserved: mode=%v", info.Mode())
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "desired\n" {
		t.Fatalf("commit did not replace target: %q err=%v", got, err)
	}
}
```

Add state-change, new-file, and cleanup cases:

```go
func TestPrepareGitIgnoreRewriteRejectsPathStateChanges(t *testing.T) {
	tests := []struct {
		name    string
		change  func(root string) GitIgnoreSectionChange
		mutate  func(t *testing.T, root string)
	}{
		{
			name: "missing became present",
			change: func(root string) GitIgnoreSectionChange {
				return GitIgnoreSectionChange{TargetPath: filepath.Join(root, GitIgnorePath), TargetMissing: true, DesiredBytes: []byte("desired\n")}
			},
			mutate: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, GitIgnorePath), []byte("new\n"), 0o644); err != nil { t.Fatal(err) }
			},
		},
		{
			name: "regular became missing",
			change: func(root string) GitIgnoreSectionChange {
				return GitIgnoreSectionChange{TargetPath: filepath.Join(root, GitIgnorePath), OriginalBytes: []byte("old\n"), DesiredBytes: []byte("desired\n")}
			},
			mutate: func(t *testing.T, root string) {},
		},
		{
			name: "regular became symlink",
			change: func(root string) GitIgnoreSectionChange {
				return GitIgnoreSectionChange{TargetPath: filepath.Join(root, GitIgnorePath), OriginalBytes: []byte("old\n"), DesiredBytes: []byte("desired\n")}
			},
			mutate: func(t *testing.T, root string) {
				if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(root, GitIgnorePath)); err != nil { t.Fatal(err) }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			change := tt.change(root)
			tt.mutate(t, root)
			if _, _, err := PrepareGitIgnoreRewrite(change); err == nil || !strings.Contains(err.Error(), "stale .gitignore") {
				t.Fatalf("expected stale path-state error, got %v", err)
			}
		})
	}
}

func TestPrepareGitIgnoreRewriteCreatesNewFileAndCleansUncommittedTemp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, GitIgnorePath)
	change := GitIgnoreSectionChange{TargetPath: path, TargetMissing: true, DesiredBytes: []byte("desired\n")}
	commit, cleanup, err := PrepareGitIgnoreRewrite(change)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup created target: %v", err)
	}
	commit, cleanup, err = PrepareGitIgnoreRewrite(change)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&^0o644 != 0 || info.Mode().Perm()&0o200 == 0 {
		t.Fatalf("new mode is not 0644 subject to umask: mode=%v", info.Mode())
	}
}
```

- [ ] **Step 2: Run target preparation tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestPrepareGitIgnoreRewrite' -count=1
```

Expected: compilation fails because `PrepareGitIgnoreRewrite` is undefined.

- [ ] **Step 3: Implement stale revalidation and temporary-file preparation**

Add to `gitignore_target.go`:

```go
func PrepareGitIgnoreRewrite(change GitIgnoreSectionChange) (commit func() error, cleanup func() error, err error) {
	mode, err := revalidateGitIgnoreTarget(change)
	if err != nil {
		return nil, nil, err
	}
	temp, err := createGitIgnoreTemp(filepath.Dir(change.TargetPath), mode)
	if err != nil {
		return nil, nil, fmt.Errorf("create .gitignore temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup = func() error {
		err := os.Remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	fail := func(err error) (func() error, func() error, error) {
		temp.Close()
		cleanup()
		return nil, nil, err
	}
	if _, err := temp.Write(change.DesiredBytes); err != nil {
		return fail(fmt.Errorf("write .gitignore temp file: %w", err))
	}
	if !change.TargetMissing {
		if err := temp.Chmod(mode); err != nil {
			return fail(fmt.Errorf("chmod .gitignore temp file: %w", err))
		}
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close .gitignore temp file: %w", err)
	}
	commit = func() error {
		if err := os.Rename(tempName, change.TargetPath); err != nil {
			return fmt.Errorf("commit .gitignore: %w", err)
		}
		return nil
	}
	return commit, cleanup, nil
}

func createGitIgnoreTemp(directory string, mode os.FileMode) (*os.File, error) {
	for attempt := 0; attempt < 100; attempt++ {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(directory, fmt.Sprintf(".gitignore.driftline-%x", suffix))
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("allocate unique .gitignore temp file")
}

func revalidateGitIgnoreTarget(change GitIgnoreSectionChange) (os.FileMode, error) {
	info, err := os.Lstat(change.TargetPath)
	if change.TargetMissing {
		if errors.Is(err, os.ErrNotExist) {
			return 0o644, nil
		}
		if err != nil {
			return 0, fmt.Errorf("inspect %s: %w", GitIgnorePath, err)
		}
		return 0, fmt.Errorf("stale %s: target appeared after planning", GitIgnorePath)
	}
	if errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("stale %s: target disappeared after planning", GitIgnorePath)
	}
	if err != nil {
		return 0, fmt.Errorf("inspect %s: %w", GitIgnorePath, err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("stale %s: target is no longer a regular file", GitIgnorePath)
	}
	current, err := os.ReadFile(change.TargetPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", GitIgnorePath, err)
	}
	if !bytes.Equal(current, change.OriginalBytes) {
		return 0, fmt.Errorf("stale %s: target changed after planning", GitIgnorePath)
	}
	return info.Mode().Perm(), nil
}
```

Add `bytes`, `crypto/rand`, and `path/filepath` imports. Creating a new temp file with `os.OpenFile(..., 0644)` applies the process umask; replacing an existing file calls `Chmod` afterward to restore its exact permission bits.

- [ ] **Step 4: Write failing apply-order and Sync commit tests**

Add to `target_repository_test.go`:

```go
func TestTargetRepositoryApplyRejectsStaleGitIgnoreBeforeManagedWrite(t *testing.T) {
	targetDir := t.TempDir()
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, syncManifestTOMLForApplyTest(""))
	writeTargetRepositoryTestFile(t, targetDir, GitIgnorePath, "changed-after-plan\n")
	managedPath := filepath.Join(targetDir, "managed.txt")
	plan := Plan{
		Changes: []Change{{ID: "tool.config", Target: "managed.txt", TargetPath: managedPath, SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true}},
		GitIgnore: &GitIgnoreSectionChange{
			Status: StatusUpdate, TargetPath: filepath.Join(targetDir, GitIgnorePath), OriginalBytes: []byte("planned\n"), DesiredBytes: []byte("desired\n"),
		},
	}
	err := (TargetRepository{Root: targetDir}).Apply(plan)
	if err == nil || !strings.Contains(err.Error(), "stale .gitignore") {
		t.Fatalf("expected stale Gitignore error, got %v", err)
	}
	if _, err := os.Stat(managedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Managed write happened before Gitignore preflight: %v", err)
	}
}

func TestTargetRepositoryApplyDoesNotCommitSyncManifestWhenGitIgnoreCommitFails(t *testing.T) {
	targetDir := t.TempDir()
	original := syncManifestTOMLForApplyTest("")
	writeTargetRepositoryTestFile(t, targetDir, SyncManifestPath, original)
	managedPath := filepath.Join(targetDir, "managed.txt")
	plan := Plan{
		Changes: []Change{
			{ID: "tool.config", Status: StatusSyncManifestAdd},
			{ID: "tool.config", Target: "managed.txt", TargetPath: managedPath, SourceBytes: []byte("source\n"), Status: StatusAdd, WritesTarget: true},
		},
		GitIgnore: &GitIgnoreSectionChange{Status: StatusAdd, TargetPath: filepath.Join(targetDir, GitIgnorePath), TargetMissing: true, DesiredBytes: []byte("generated\n")},
		NextSyncManifest: SyncManifest{Version: 2, Source: SyncSource{Repository: "y-writings/source-repo", Ref: "main"}, Files: map[string]map[string]string{"tool": {"config": "managed.txt"}}},
	}
	repository := TargetRepository{
		Root: targetDir,
		prepareGitIgnoreRewrite: func(GitIgnoreSectionChange) (func() error, func() error, error) {
			return func() error { return errors.New("commit failed") }, func() error { return nil }, nil
		},
	}
	if err := repository.Apply(plan); err == nil || err.Error() != "commit failed" {
		t.Fatalf("expected Gitignore commit error, got %v", err)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, SyncManifestPath); got != original {
		t.Fatalf("Sync manifest committed after Gitignore failure:\n%s", got)
	}
	if got := readTargetRepositoryTestFile(t, targetDir, "managed.txt"); got != "source\n" {
		t.Fatalf("existing no-rollback behavior changed: %q", got)
	}
}
```

- [ ] **Step 5: Run apply tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestTargetRepositoryApply(RejectsStaleGitIgnoreBeforeManagedWrite|DoesNotCommitSyncManifestWhenGitIgnoreCommitFails)$' -count=1
```

Expected: tests fail because `Apply` does not prepare or commit `Plan.GitIgnore`, and `TargetRepository` lacks the injected preparation function.

- [ ] **Step 6: Integrate prepare-before-mutation and commit-before-Sync ordering**

Extend `TargetRepository`:

```go
type TargetRepository struct {
	Root                    string
	prepareGitIgnoreRewrite func(GitIgnoreSectionChange) (func() error, func() error, error)
}
```

After preparing the optional Sync manifest rewrite and before sorting/applying Managed changes, add:

```go
	var commitGitIgnore func() error
	if plan.GitIgnore != nil {
		prepare := r.prepareGitIgnoreRewrite
		if prepare == nil {
			prepare = PrepareGitIgnoreRewrite
		}
		commit, cleanup, err := prepare(*plan.GitIgnore)
		if err != nil {
			return err
		}
		defer cleanup()
		commitGitIgnore = commit
	}
```

After Managed writes and before `commitSyncManifest`, add:

```go
	if commitGitIgnore != nil {
		if err := commitGitIgnore(); err != nil {
			return err
		}
	}
```

Do not add rollback. The existing Managed deletion-before-write order remains unchanged and therefore supports Managed-to-Gitignore delete-then-create.

- [ ] **Step 7: Format and run target/apply tests**

Run:

```bash
gofmt -w src/internal/driftline/gitignore_target.go src/internal/driftline/gitignore_target_test.go src/internal/driftline/target_repository.go src/internal/driftline/target_repository_test.go
go test ./src/internal/driftline -run 'Test(PrepareGitIgnoreRewrite|TargetRepositoryApply)' -count=1
go test ./src/internal/driftline -count=1
```

Expected: all commands pass.

- [ ] **Step 8: Commit atomic apply support**

```bash
git add src/internal/driftline/gitignore_target.go src/internal/driftline/gitignore_target_test.go src/internal/driftline/target_repository.go src/internal/driftline/target_repository_test.go
git commit -m "feat: apply gitignore section atomically"
```

## Task 6: Integrate Check, Diff, Update, And Init Behavior

**Files:**

- Modify: `src/internal/driftline/commands/check.go`
- Modify: `src/internal/driftline/commands/diff.go`
- Modify: `src/internal/driftline/commands/update.go`
- Modify: `src/internal/driftline/commands/run.go`
- Modify: `src/internal/driftline/commands/commands_test.go`
- Modify: `src/internal/driftline/plan.go:365-372`

- [ ] **Step 1: Write a failing end-to-end command lifecycle test**

Add to `commands_test.go`:

```go
func TestGitIgnoreSectionCommandLifecycle(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	writeFile(t, targetDir, driftline.GitIgnorePath, ".env\n")
	client := newCommandSourceClient("main", `version = 2
[gitignore]
entries = [".env", "/dist/"]
`, nil)
	runner := Runner{Source: client}

	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr); !errors.Is(err, errDrift) {
		t.Fatalf("check should report drift: %v", err)
	}
	if got, want := stdout.String(), "add gitignore: generated section is missing\n"; got != want {
		t.Fatalf("unexpected check output: %q", got)
	}

	stdout.Reset()
	if err := runner.Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr); !errors.Is(err, errDrift) {
		t.Fatalf("diff should report drift: %v", err)
	}
	for _, want := range []string{".env", "# start driftline from y-writings/source-repo/.driftline/contract.toml", "/dist/", "# end driftline"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("diff missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != ".env\n\n# start driftline from y-writings/source-repo/.driftline/contract.toml\n"+driftlineGitIgnoreWarningForTest+"\n.env\n/dist/\n# end driftline\n" {
		t.Fatalf("unexpected .gitignore: %q", got)
	}

	stdout.Reset()
	if err := runner.Run([]string{"check", "--target-dir", targetDir}, &stdout, &stderr); err != nil || stdout.String() != "synced\n" {
		t.Fatalf("expected synced check, err=%v output=%q", err, stdout.String())
	}
}
```

Define the test-only warning constant in `commands_test.go` because the production warning is intentionally unexported:

```go
const driftlineGitIgnoreWarningForTest = "# DO NOT EDIT: this section is managed automatically by driftline."
```

- [ ] **Step 2: Write failing init, removal, malformed, and binary-diff tests**

Add focused tests that assert:

```go
func TestInitValidatesButDoesNotApplyGitIgnoreSection(t *testing.T) {
	targetDir := t.TempDir()
	client := newCommandSourceClient("main", `version = 2
[gitignore]
entries = [".env"]
[files.templates]
ignore = { path = ".gitignore", mode = "template" }
`, map[string]string{".gitignore": "template-base\n"})
	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != "template-base\n" {
		t.Fatalf("init applied generated section: %q", got)
	}
}

func TestUpdateRejectsMalformedGitIgnoreBeforeManagedWrites(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	writeFile(t, targetDir, driftline.GitIgnorePath, "# end driftline\n")
	client := newCommandSourceClient("main", `version = 2
[gitignore]
entries = [".env"]
[files.tool]
config = { path = "managed.txt", mode = "managed" }
`, map[string]string{"managed.txt": "source\n"})
	var stdout, stderr bytes.Buffer
	err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir, "--force", "tool.config"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "invalid driftline section in .gitignore") {
		t.Fatalf("expected marker error, got %v", err)
	}
	assertFileMissing(t, targetDir, "managed.txt")
}
```

Add removal and binary-diff coverage:

```go
func TestUpdateRemovesUndeclaredGitIgnoreSectionAndKeepsFile(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	writeFile(t, targetDir, driftline.GitIgnorePath, "base\n\n# start driftline from old/source/.driftline/contract.toml\n"+driftlineGitIgnoreWarningForTest+"\n.env\n# end driftline\n")
	runner := Runner{Source: newCommandSourceClient("main", "version = 2\n", nil)}
	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "remove gitignore: generated section is no longer declared\n" {
		t.Fatalf("unexpected remove output: %q", stdout.String())
	}
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != "base\n\n" {
		t.Fatalf("remove changed outside bytes or deleted file: %q", got)
	}
}

func TestDiffReportsBinaryGitIgnoreChange(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	if err := os.WriteFile(filepath.Join(targetDir, driftline.GitIgnorePath), []byte{0, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Source: newCommandSourceClient("main", "version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)}
	var stdout, stderr bytes.Buffer
	err := runner.Run([]string{"diff", "--target-dir", targetDir}, &stdout, &stderr)
	if !errors.Is(err, errDrift) || !strings.Contains(stdout.String(), "Binary files") {
		t.Fatalf("expected binary drift output, err=%v output=%q", err, stdout.String())
	}
}

func TestInitDoesNotInspectExistingGitIgnoreMarkers(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.GitIgnorePath, "# end driftline\n")
	client := newCommandSourceClient("main", "version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)
	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"init", "y-writings/source-repo", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatalf("init inspected malformed marker: %v", err)
	}
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != "# end driftline\n" {
		t.Fatalf("init changed existing .gitignore: %q", got)
	}
}

func TestUpdateManagedToGitIgnoreRecreatesGeneratedOnlyFile(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML("[files.old]\nignore = \".gitignore\"\n"))
	writeFile(t, targetDir, driftline.GitIgnorePath, "managed-before\n")
	client := newCommandSourceClient("main", "version = 2\n[gitignore]\nentries = [\".env\"]\n", nil)
	var stdout, stderr bytes.Buffer
	if err := (Runner{Source: client}).Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	want := "# start driftline from y-writings/source-repo/.driftline/contract.toml\n" + driftlineGitIgnoreWarningForTest + "\n.env\n# end driftline\n"
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != want {
		t.Fatalf("former Managed bytes survived transition: %q", got)
	}
}

func TestUpdateGitIgnoreToManagedUsesExistingForceSemantics(t *testing.T) {
	targetDir := t.TempDir()
	writeFile(t, targetDir, driftline.SyncManifestPath, syncManifestTOML(""))
	writeFile(t, targetDir, driftline.GitIgnorePath, "# start driftline from old/source/.driftline/contract.toml\n"+driftlineGitIgnoreWarningForTest+"\n.env\n# end driftline\n")
	client := newCommandSourceClient("main", "version = 2\n[files.tool]\nignore = { path = \".gitignore\", mode = \"managed\" }\n", map[string]string{".gitignore": "managed\n"})
	runner := Runner{Source: client}
	var stdout, stderr bytes.Buffer
	if err := runner.Run([]string{"update", "--target-dir", targetDir}, &stdout, &stderr); !errors.Is(err, errDrift) {
		t.Fatalf("expected Managed conflict, got %v", err)
	}
	stdout.Reset()
	if err := runner.Run([]string{"update", "--target-dir", targetDir, "--force", "tool.ignore"}, &stdout, &stderr); err != nil {
		t.Fatalf("forced ownership transfer failed: %v", err)
	}
	if got := readFile(t, targetDir, driftline.GitIgnorePath); got != "managed\n" {
		t.Fatalf("Managed source did not replace generated section: %q", got)
	}
}
```

- [ ] **Step 3: Run command tests and verify RED**

Run:

```bash
go test ./src/internal/driftline/commands -run 'Test(GitIgnoreSectionCommandLifecycle|InitValidatesButDoesNotApplyGitIgnoreSection|UpdateRejectsMalformedGitIgnoreBeforeManagedWrites|UpdateRemovesUndeclaredGitIgnoreSectionAndKeepsFile|DiffReportsBinaryGitIgnoreChange|InitDoesNotInspectExistingGitIgnoreMarkers|UpdateManagedToGitIgnoreRecreatesGeneratedOnlyFile|UpdateGitIgnoreToManagedUsesExistingForceSemantics)$' -count=1
```

Expected: compilation or assertions fail because command reporting only accepts Managed `Changes` and `diff` ignores `Plan.GitIgnore`.

- [ ] **Step 4: Print complete plans in check and update**

Change `printChanges` to accept `driftline.Plan`:

```go
func printChanges(w io.Writer, plan driftline.Plan) {
	for _, change := range sortedChanges(plan.Changes) {
		if change.Status == driftline.StatusSynced {
			continue
		}
		printChange(w, change)
	}
	if plan.GitIgnore != nil {
		fmt.Fprintf(w, "%s gitignore: %s\n", plan.GitIgnore.Status, plan.GitIgnore.Reason)
	}
	if !plan.HasDrift() {
		fmt.Fprintln(w, "synced")
	}
}
```

Update `runCheck`:

```go
	printChanges(stdout, plan)
	if plan.HasDrift() {
		return errDrift
	}
```

Update both `runUpdate` print calls to pass `plan` rather than `plan.Changes`. After all command callers use `Plan.HasDrift`, remove the obsolete `HasDrift(changes []Change)` helper and inline its loop into `Plan.HasDrift`:

```go
func (p Plan) HasDrift() bool {
	if p.GitIgnore != nil {
		return true
	}
	for _, change := range p.Changes {
		if change.Status != StatusSynced {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Render the dedicated full-file diff**

After the existing Managed change loop in `runDiff`, add:

```go
	if plan.GitIgnore != nil {
		if err := printBytesDiff(stdout, plan.GitIgnore.DesiredBytes, plan.GitIgnore.TargetPath, plan.GitIgnore.TargetMissing); err != nil {
			return err
		}
	}
	if plan.HasDrift() {
		return errDrift
	}
```

Remove the old `HasDrift(plan.Changes)` check. The dedicated remove status still receives a content diff because the desired complete file bytes are available.

- [ ] **Step 6: Update command help language**

In `printUsage`, use:

```text
  check            check whether Target Repository state matches the Contract
  diff             show content diffs for planned Target Repository changes
  update           reconcile Managed files, the Gitignore section, and .driftline/sync.toml
```

Extend the existing help test with:

```go
if !strings.Contains(stdout.String(), "Gitignore section") {
	t.Fatalf("help does not describe Gitignore section reconciliation:\n%s", stdout.String())
}
```

Keep the existing loop that rejects obsolete YAML, lock, path override, `if_not_exists`, and `prune` surfaces.

- [ ] **Step 7: Format and run command and full Go tests**

Run:

```bash
gofmt -w src/internal/driftline/plan.go src/internal/driftline/commands/check.go src/internal/driftline/commands/diff.go src/internal/driftline/commands/update.go src/internal/driftline/commands/run.go src/internal/driftline/commands/commands_test.go
go test ./src/internal/driftline/commands -count=1
go test ./... -count=1
```

Expected: all commands pass.

- [ ] **Step 8: Commit command integration**

```bash
git add src/internal/driftline/plan.go src/internal/driftline/commands/check.go src/internal/driftline/commands/diff.go src/internal/driftline/commands/update.go src/internal/driftline/commands/run.go src/internal/driftline/commands/commands_test.go
git commit -m "feat: reconcile gitignore section in target commands"
```

## Task 7: Update Canonical Guidance And Run Full Verification

**Files:**

- Modify: `README.md:35-58,115-137`
- Modify: `CONTEXT.md:17-39`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md:77-89`
- Keep: `docs/superpowers/plans/2026-07-19-contract-gitignore-section.md`

- [ ] **Step 1: Update the README Contract example and rules**

Add this table before `[files.github-workflow]` in the Contract example:

```toml
[gitignore]
entries = [
  ".env",
  "/dist/",
]
```

Add a `## Gitignore Section` section containing the exact generated block:

```gitignore
# start driftline from y-writings/source-repo/.driftline/contract.toml
# DO NOT EDIT: this section is managed automatically by driftline.
.env
/dist/
# end driftline
```

Document these user-facing rules in prose: `init` validates but does not apply it; `check`, `diff`, and `update` reconcile it; outside bytes and duplicate outside entries are ignored; malformed or duplicate markers require manual repair; Template `.gitignore` may coexist; Managed `.gitignore` may not coexist while `[gitignore]` is present.

- [ ] **Step 2: Add the domain term to CONTEXT.md**

Add after Template file:

```markdown
**Gitignore section**:
The marker-delimited region in the Target Repository's root `.gitignore` that driftline reconciles from Contract `[gitignore].entries` while preserving target-owned bytes outside the markers.
_Avoid_: Managed `.gitignore`, appended ignore list, generated `.gitignore` file.
```

Update the Sync plan definition to include Contract Gitignore entries and Target Repository Gitignore state.

- [ ] **Step 3: Cross-link canonical instructions**

In `AGENTS.md`, add the focused design beside the two existing canonical redesign documents:

```markdown
The canonical design for Contract-managed root `.gitignore` sections is `docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md`. It supersedes only the root `gitignore` removal decision in the Managed/Template design.
```

Replace line 88 of the older Managed/Template spec with:

```markdown
- Root `gitignore` behavior from the old YAML design remains removed. Marker-scoped Contract `[gitignore]` behavior is defined separately by `2026-07-19-contract-gitignore-section-design.md`; it is not compatibility with the old append-only behavior.
```

- [ ] **Step 4: Format and lint documentation**

Run:

```bash
prettier --write README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md
markdownlint-cli2 README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md
git diff --check
```

Expected: Prettier completes and both checks report zero errors.

- [ ] **Step 5: Run the complete verification suite**

Run:

```bash
gofmt -w src/internal/driftline/*.go src/internal/driftline/commands/*.go
go test ./... -count=1
go vet ./...
prettier --check README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md
markdownlint-cli2 README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md
git diff --check
```

Expected: all Go tests pass, `go vet` exits zero, formatting is unchanged, markdownlint reports zero errors, and `git diff --check` prints nothing.

- [ ] **Step 6: Review the final diff against the design**

Run:

```bash
git status --short
git diff --stat
git diff -- src/internal/driftline src/internal/driftline/commands README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md
```

Verify that the diff contains no Sync manifest Gitignore field, no legacy YAML reader, no target-side opt-out, no generic partial-file framework, and no `init` section write.

- [ ] **Step 7: Commit documentation and final integration**

```bash
git add README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md
git commit -m "docs: document contract gitignore section"
```

- [ ] **Step 8: Confirm the implementation branch is clean**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: status is empty and the task commits are visible in order.
