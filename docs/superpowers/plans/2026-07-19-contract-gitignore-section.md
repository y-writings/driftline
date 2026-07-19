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
- Create `src/internal/driftline/gitignore_target.go`: common planning adapter, stale revalidation, temporary-file preparation, and dedicated test seams.
- Create `src/internal/driftline/gitignore_target_unix.go`, `gitignore_target_windows.go`, and `gitignore_target_unsupported.go`: platform-specific same-descriptor, no-follow reads and fail-closed fallback.
- Create `src/internal/driftline/gitignore_target_test.go`, platform-specific target tests, and `gitignore_target_fifo_test.go`: regular-file, symlink/reparse-point, nonblocking FIFO, and unsupported-read coverage.
- Create `src/internal/driftline/gitignore_commit_unix.go`, `gitignore_commit_windows.go`, and `gitignore_commit_unsupported.go`: platform-specific atomic-replacement capability and commit boundaries.
- Create `src/internal/driftline/gitignore_rewrite_test.go` and `gitignore_commit_unix_test.go`: stale-state, mode, cleanup-error, and atomic replacement tests on supported Unix-like platforms.
- Modify `src/internal/driftline/types.go`: Contract Gitignore model, dedicated plan change type, and `Plan` drift support types.
- Modify `src/internal/driftline/config.go`: required-key validation, raw entry validation, and Contract path coexistence validation.
- Modify `src/internal/driftline/config_test.go`: Contract parsing and invalid configuration coverage.
- Modify `src/internal/driftline/plan.go`: resolved Managed collision checks, ownership-transition precedence, and Gitignore section plan integration.
- Modify `src/internal/driftline/plan_test.go`: planner status, filesystem state, collision, source-change, and transition tests.
- Modify `src/internal/driftline/target_repository.go`: prepare the Gitignore write before mutations and commit it before the Sync manifest.
- Modify `src/internal/driftline/target_repository_test.go`: apply ordering, stale abort, failure, and no-rollback tests.
- Modify `src/internal/driftline/commands/check.go`: print the complete plan and include Gitignore drift.
- Modify `src/internal/driftline/commands/diff.go`: render the dedicated full-file Gitignore diff from planned byte snapshots with stable logical labels.
- Create `src/internal/driftline/commands/diff_test.go`: snapshot isolation and header-only relabeling tests.
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

Add `slices` to the test imports. Also add table-driven cases that reject `[GitIgnore]`, `[GITIGNORE]`, `[gitignore].Entries`, `[gitignore].ENTRIES`, and canonical-plus-noncanonical duplicate spellings as unknown keys. Add normalized coexistence cases for `./.gitignore`, `.gitignore/.`, and `.gitignore/./rules` so validation is exercised against the same normalized paths used by planning.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestLoadContract(GitIgnore|RejectsInvalidGitIgnore|RejectsGitIgnoreKeyAliases|AcceptsExplicitEmptyGitIgnore)$' -count=1
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

In `LoadContractBytes`, reject case-insensitive aliases before the ordinary undecoded-key check, then use TOML metadata to distinguish a missing key from an explicit empty array before calling `validateContract`. BurntSushi TOML can otherwise decode noncanonical casing into the canonical fields:

```go
	if err := rejectContractGitIgnoreKeyAliases(metadata.Keys()); err != nil {
		return contract, err
	}
	if metadata.IsDefined("gitignore") && !metadata.IsDefined("gitignore", "entries") {
		return contract, errors.New("Contract gitignore must define entries")
	}
```

Add the exact-spelling guard:

```go
func rejectContractGitIgnoreKeyAliases(keys []toml.Key) error {
	for _, key := range keys {
		if strings.EqualFold(key[0], "gitignore") && key[0] != "gitignore" {
			return rejectUndecoded("Contract", []toml.Key{key[:1]})
		}
		if len(key) > 1 && key[0] == "gitignore" && strings.EqualFold(key[1], "entries") && key[1] != "entries" {
			return rejectUndecoded("Contract", []toml.Key{key[:2]})
		}
	}
	return nil
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
- Create: `src/internal/driftline/gitignore_target_unix.go`
- Create: `src/internal/driftline/gitignore_target_windows.go`
- Create: `src/internal/driftline/gitignore_target_unsupported.go`
- Create: `src/internal/driftline/gitignore_target_test.go`
- Create: `src/internal/driftline/gitignore_target_unix_test.go`
- Create: `src/internal/driftline/gitignore_target_windows_test.go`
- Create: `src/internal/driftline/gitignore_target_unsupported_test.go`
- Create: `src/internal/driftline/gitignore_target_fifo_test.go`
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
			if err == nil || !strings.Contains(err.Error(), ".gitignore must be a regular file") {
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
	if err == nil || !strings.Contains(err.Error(), "cannot be below .gitignore") {
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
go test ./src/internal/driftline -run 'Test(BuildPlan(AddsMissingGitIgnoreSection|RemovesUndeclaredGitIgnoreSection|RejectsUnsupportedGitIgnoreTargetForNonEmptyEntries)|ReadRegularFileNoFollow)' -count=1
```

Expected: compilation fails because the dedicated plan types and platform-specific read helpers do not exist.

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

- [ ] **Step 4: Implement platform-specific no-follow planning inspection**

Keep path classification and transformation in common `gitignore_target.go`, but obtain bytes and permission bits only through `readRegularFileNoFollow`. `Lstat` is preliminary path classification; the helper revalidates the object it actually opened. Do not follow that descriptor check with `os.ReadFile` or any other pathname reopen:

```go
var errOpenedTargetNotRegular = errors.New("opened target is not a regular file")

func planGitIgnoreSectionChange(targetDir string, repository string, config *ContractGitIgnore, replaceAfterManagedDelete bool) (*GitIgnoreSectionChange, error) {
	targetPath, err := PathWithin(targetDir, GitIgnorePath, GitIgnorePath+" target")
	if err != nil {
		return nil, err
	}

	active := config != nil && len(config.Entries) > 0
	targetMissing := false
	info, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		targetMissing = true
	} else if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", GitIgnorePath, err)
	} else if !info.Mode().IsRegular() {
		if active {
			return nil, fmt.Errorf("%s must be a regular file when gitignore entries are configured", GitIgnorePath)
		}
		return nil, nil
	}

	if targetMissing && !active {
		return nil, nil
	}

	var original []byte
	if !targetMissing {
		original, _, err = readRegularFileNoFollow(targetPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", GitIgnorePath, err)
		}
	}

	logicalCurrent := original
	logicalTargetMissing := targetMissing
	if replaceAfterManagedDelete {
		logicalCurrent = nil
		logicalTargetMissing = true
	}
	transformed, err := transformGitIgnoreSection(logicalCurrent, logicalTargetMissing, repository, config)
	if err != nil {
		return nil, err
	}
	if !transformed.Changed {
		return nil, nil
	}
	return &GitIgnoreSectionChange{
		Status:        transformed.Status,
		Reason:        transformed.Reason,
		TargetPath:    targetPath,
		TargetMissing: targetMissing,
		OriginalBytes: original,
		DesiredBytes:  transformed.DesiredBytes,
	}, nil
}
```

On supported Unix-like systems, put the helper in `gitignore_target_unix.go` behind the exact supported build constraint. Open once with no-follow and nonblocking flags, verify the opened descriptor with `Stat`, then read from that same descriptor. `O_NONBLOCK` prevents a raced FIFO from hanging before the regular-file check:

```go
//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import (
	"io"
	"os"
	"syscall"
)

func readRegularFileNoFollow(path string) ([]byte, os.FileMode, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errOpenedTargetNotRegular
	}
	data, err := io.ReadAll(file)
	return data, info.Mode().Perm(), err
}
```

On Windows, put the helper in `gitignore_target_windows.go`. Open the reparse point itself, reject reparse points and non-regular handles, and read bytes and mode from the same handle:

```go
//go:build windows

package driftline

import (
	"io"
	"os"
	"syscall"
)

func readRegularFileNoFollow(path string) ([]byte, os.FileMode, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: err}
	}

	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		syscall.CloseHandle(handle)
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: syscall.EINVAL}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || !info.Mode().IsRegular() {
		return nil, 0, errOpenedTargetNotRegular
	}
	data, err := io.ReadAll(file)
	return data, info.Mode().Perm(), err
}
```

All remaining platforms compile `gitignore_target_unsupported.go` and fail safe when a regular target must be read:

```go
//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"fmt"
	"os"
	"runtime"
)

func readRegularFileNoFollow(string) ([]byte, os.FileMode, error) {
	return nil, 0, fmt.Errorf("safe .gitignore target reads are unsupported on %s", runtime.GOOS)
}
```

Add common regular-file/mode coverage, Unix final-symlink and FIFO rejection, Windows reparse-point rejection, and unsupported-platform error coverage in the corresponding build-tagged test files. The FIFO tests use the narrower Darwin, DragonFly, FreeBSD, Linux, NetBSD, and OpenBSD build boundary where `syscall.Mkfifo` is available.

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
gofmt -w src/internal/driftline/types.go src/internal/driftline/plan.go src/internal/driftline/plan_test.go src/internal/driftline/gitignore_target*.go
go test ./src/internal/driftline -run 'TestBuildPlan' -count=1
go test ./src/internal/driftline -count=1
go test ./... -count=1
```

Expected: all commands pass.

- [ ] **Step 7: Commit basic planner integration**

```bash
git add src/internal/driftline/types.go src/internal/driftline/plan.go src/internal/driftline/plan_test.go src/internal/driftline/gitignore_target.go src/internal/driftline/gitignore_target_unix.go src/internal/driftline/gitignore_target_windows.go src/internal/driftline/gitignore_target_unsupported.go src/internal/driftline/gitignore_target_test.go src/internal/driftline/gitignore_target_unix_test.go src/internal/driftline/gitignore_target_windows_test.go src/internal/driftline/gitignore_target_unsupported_test.go src/internal/driftline/gitignore_target_fifo_test.go
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

Also cover a currently Managed key whose next Contract entry is Template at a different source path. With non-empty Gitignore entries, section planning must use and preserve the former `.gitignore` target bytes; with absent or empty entries, it must leave those bytes untouched and skip marker inspection. Managed-to-Template classification follows the current File key and mode, not whether the Template's new source path still equals `.gitignore`.

- [ ] **Step 2: Run the transition test and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestBuildPlan(GitIgnoreOwnershipTransitions|ManagedToRenamedTemplateGitIgnore)' -count=1
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

Keep coexistence validation scoped to desired Managed files, so `currentGitIgnoreKey` may be removed or transition to Template. Determine the Template transition from the current File key's next mode even when that Template now has a different source path; do not add a `source.Path == GitIgnorePath` condition.

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
- Create: `src/internal/driftline/gitignore_rewrite_test.go`
- Create: `src/internal/driftline/gitignore_commit_unix.go`
- Create: `src/internal/driftline/gitignore_commit_unix_test.go`
- Create: `src/internal/driftline/gitignore_commit_windows.go`
- Create: `src/internal/driftline/gitignore_commit_unsupported.go`
- Modify: `src/internal/driftline/gitignore_target_windows_test.go`
- Modify: `src/internal/driftline/gitignore_target_unsupported_test.go`
- Modify: `src/internal/driftline/target_repository.go:10-53`
- Modify: `src/internal/driftline/target_repository_test.go`

- [ ] **Step 1: Write failing stale-state, atomicity, mode, and cleanup tests**

Create `gitignore_rewrite_test.go` with the supported Unix-like build constraint. Cover these exact responsibilities:

- `TestPrepareGitIgnoreRewriteRejectsAppearedMissingTarget`: regular, symlink, and directory states all fail stale without creating a temporary file.
- `TestPrepareGitIgnoreRewriteRejectsChangedRegularTarget`: changed bytes, missing, live symlink, broken symlink, and directory states all fail stale; no external bytes are read.
- `TestPrepareGitIgnoreRewritePreservesRevalidationErrorCause`: wrapped read errors remain discoverable with `errors.Is`.
- `TestPrepareGitIgnoreRewriteDefersAtomicReplacement` and `TestPrepareGitIgnoreRewriteCleanupRemovesUncommittedTemp`: preparation writes only the sibling temporary file, commit renames that file, and cleanup is repeatable.
- `TestPrepareGitIgnoreRewritePreservesExistingMode`, `TestPrepareGitIgnoreRewriteNewModeMatchesUmaskApplied0644`, and `TestPrepareGitIgnoreRewriteCommitsEmptyFileInsteadOfDeleting`: existing permission bits survive, new files use `0644` subject to umask, and an empty desired file remains present.
- `TestPrepareGitIgnoreRewriteJoinsWriteCloseAndRemoveFailures` and `TestPrepareGitIgnoreRewriteCleanupSurfacesFailureAndRemainsIdempotent`: injected write, close, and remove errors are joined rather than discarded, and failed cleanup can be retried.

Create `gitignore_commit_unix_test.go` to prove the prepared file is atomically renamed over the target and a failed rename leaves the temporary file for cleanup. Extend the Windows and unsupported-platform tests to prove both the direct preparation path and repository apply fail before creating a temporary file or mutating any target.

- [ ] **Step 2: Run target preparation tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'Test(PrepareGitIgnoreRewrite|CommitAtomicGitIgnoreReplacement)' -count=1
```

Expected: compilation fails because `PrepareGitIgnoreRewrite` and the platform-specific atomic capability and commit helpers are undefined.

- [ ] **Step 3: Implement gated stale revalidation and temporary-file preparation**

Add the common preparation code to `gitignore_target.go`. `PrepareGitIgnoreRewrite` performs its own capability check before inspecting the target or allocating a temporary file. Existing targets are revalidated with the same no-follow helper from Task 3, so bytes and mode come from one opened descriptor rather than a later pathname reopen:

```go
type gitIgnoreTempOperations struct {
	write  func(*os.File, []byte) error
	close  func(*os.File) error
	remove func(string) error
}

func PrepareGitIgnoreRewrite(change GitIgnoreSectionChange) (commit, cleanup func() error, err error) {
	return prepareGitIgnoreRewriteWithOperations(change, gitIgnoreTempOperations{})
}

func prepareGitIgnoreRewriteWithOperations(change GitIgnoreSectionChange, ops gitIgnoreTempOperations) (commit, cleanup func() error, err error) {
	if err := validateAtomicGitIgnoreReplacement(); err != nil {
		return nil, nil, err
	}
	if ops.write == nil {
		ops.write = func(file *os.File, data []byte) error {
			_, err := file.Write(data)
			return err
		}
	}
	if ops.close == nil {
		ops.close = (*os.File).Close
	}
	if ops.remove == nil {
		ops.remove = os.Remove
	}

	mode := os.FileMode(0o644)
	if change.TargetMissing {
		_, err := os.Lstat(change.TargetPath)
		if err == nil {
			return nil, nil, staleGitIgnorePlanError("target appeared", nil)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, staleGitIgnorePlanError("inspect target", err)
		}
	} else {
		current, currentMode, err := readRegularFileNoFollow(change.TargetPath)
		if err != nil {
			return nil, nil, staleGitIgnorePlanError("read target", err)
		}
		if !bytes.Equal(current, change.OriginalBytes) {
			return nil, nil, staleGitIgnorePlanError("target content changed", nil)
		}
		mode = currentMode
	}

	temp, err := createGitIgnoreTemp(filepath.Dir(change.TargetPath))
	if err != nil {
		return nil, nil, fmt.Errorf("create %s temp file: %w", GitIgnorePath, err)
	}
	tempName := temp.Name()
	cleanup = func() error {
		err := ops.remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return gitIgnoreTempOperationError("remove", err)
	}
	fail := func(primary error) (func() error, func() error, error) {
		closeErr := gitIgnoreTempOperationError("close", ops.close(temp))
		return nil, nil, errors.Join(primary, closeErr, cleanup())
	}

	if !change.TargetMissing {
		if err := temp.Chmod(mode); err != nil {
			return fail(fmt.Errorf("chmod %s temp file: %w", GitIgnorePath, err))
		}
	}
	if err := ops.write(temp, change.DesiredBytes); err != nil {
		return fail(fmt.Errorf("write %s temp file: %w", GitIgnorePath, err))
	}
	if err := ops.close(temp); err != nil {
		return nil, nil, errors.Join(gitIgnoreTempOperationError("close", err), cleanup())
	}

	commit = func() error {
		if err := commitAtomicGitIgnoreReplacement(tempName, change.TargetPath); err != nil {
			return fmt.Errorf("commit %s rewrite: %w", GitIgnorePath, err)
		}
		return nil
	}
	return commit, cleanup, nil
}

func gitIgnoreTempOperationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s temp file: %w", operation, GitIgnorePath, err)
}

func createGitIgnoreTemp(dir string) (*os.File, error) {
	for range 100 {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, fmt.Sprintf(".gitignore-%x.tmp", suffix))
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, fmt.Errorf("could not allocate a unique temp file")
}

func staleGitIgnorePlanError(reason string, cause error) error {
	if cause != nil {
		return fmt.Errorf("stale %s plan: %s: %w", GitIgnorePath, reason, cause)
	}
	return fmt.Errorf("stale %s plan: %s", GitIgnorePath, reason)
}
```

The temporary file is created with `0644`, allowing the process umask to determine new-file permissions. For an existing target, call `Chmod` with the descriptor-observed permission bits before writing. The operation seam exists only to inject write, close, and remove failures; the production entry point remains `PrepareGitIgnoreRewrite`.

Put the capability and commit implementation behind platform build boundaries. Supported Unix-like platforms use atomic same-directory rename:

```go
//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package driftline

import "os"

func validateAtomicGitIgnoreReplacement() error {
	return nil
}

func commitAtomicGitIgnoreReplacement(tempPath, targetPath string) error {
	return os.Rename(tempPath, targetPath)
}
```

Windows fails closed because the implementation has no documented atomic replacement primitive there:

```go
//go:build windows

package driftline

import "fmt"

func validateAtomicGitIgnoreReplacement() error {
	return fmt.Errorf("atomic %s replacement is unsupported on windows", GitIgnorePath)
}

func commitAtomicGitIgnoreReplacement(string, string) error {
	return validateAtomicGitIgnoreReplacement()
}
```

All other platforms compile the unsupported fallback and identify the runtime platform:

```go
//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package driftline

import (
	"fmt"
	"runtime"
)

func validateAtomicGitIgnoreReplacement() error {
	return fmt.Errorf("atomic %s replacement is unsupported on %s", GitIgnorePath, runtime.GOOS)
}

func commitAtomicGitIgnoreReplacement(string, string) error {
	return validateAtomicGitIgnoreReplacement()
}
```

- [ ] **Step 4: Write failing capability, apply-order, cleanup, and Sync commit tests**

Add repository-level tests with separate seams for capability validation, Sync preparation, and Gitignore preparation:

- A conflicted plan returns before capability validation or either preparation step.
- Unsupported atomic replacement returns before Sync temporary-file preparation, Gitignore preparation, Managed deletes, or Managed writes.
- A plan without a Gitignore change skips capability validation and preserves ordinary Managed apply behavior.
- Stale Gitignore state and Gitignore preparation failure abort before any Managed mutation.
- Gitignore commit failure leaves already-completed Managed mutations in place but prevents the Sync manifest commit.
- Gitignore cleanup failures are returned, and commit plus Gitignore cleanup plus Sync cleanup failures remain individually discoverable through `errors.Is`.
- Successful Managed-to-Gitignore replacement and Gitignore-only updates preserve the existing apply ordering and do not rewrite an unchanged Sync manifest.

The platform tests must invoke the real Windows and unsupported capability helpers. The common repository tests inject the seams explicitly so host support does not hide ordering failures.

- [ ] **Step 5: Run apply tests and verify RED**

Run:

```bash
go test ./src/internal/driftline -run 'TestTargetRepositoryApply' -count=1
```

Expected: tests fail because `Apply` lacks the early capability gate, independent preparation seams, cleanup-error joining, and Gitignore commit ordering.

- [ ] **Step 6: Integrate prepare-before-mutation and commit-before-Sync ordering**

Extend `TargetRepository` and use a named return error so deferred cleanup failures can be joined. The capability gate must run after conflict rejection but before root normalization or Sync manifest temporary-file preparation:

```go
type TargetRepository struct {
	Root                               string
	validateAtomicGitIgnoreReplacement func() error
	prepareSyncManifestRewrite         func(string, SyncManifest) (func() error, func() error, error)
	prepareGitIgnoreRewrite            func(GitIgnoreSectionChange) (func() error, func() error, error)
}

func (r TargetRepository) Apply(plan Plan) (err error) {
	if plan.HasConflicts() {
		return fmt.Errorf("cannot apply conflicted sync plan")
	}
	if plan.GitIgnore != nil {
		validate := r.validateAtomicGitIgnoreReplacement
		if validate == nil {
			validate = validateAtomicGitIgnoreReplacement
		}
		if err := validate(); err != nil {
			return err
		}
	}
	root := r.Root
	if root == "" {
		root = "."
	}

	var commitSyncManifest func() error
	if planHasSyncManifestChanges(plan.Changes) {
		prepare := r.prepareSyncManifestRewrite
		if prepare == nil {
			prepare = PrepareSyncManifestRewrite
		}
		commit, cleanup, prepareErr := prepare(root, plan.NextSyncManifest)
		if prepareErr != nil {
			return prepareErr
		}
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("cleanup Sync manifest rewrite: %w", cleanupErr))
			}
		}()
		commitSyncManifest = commit
	}

	var commitGitIgnore func() error
	if plan.GitIgnore != nil {
		prepare := r.prepareGitIgnoreRewrite
		if prepare == nil {
			prepare = PrepareGitIgnoreRewrite
		}
		commit, cleanup, prepareErr := prepare(*plan.GitIgnore)
		if prepareErr != nil {
			return prepareErr
		}
		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("cleanup %s rewrite: %w", GitIgnorePath, cleanupErr))
			}
		}()
		commitGitIgnore = commit
	}

	changes := SortedChanges(plan.Changes)
	for _, change := range changes {
		if change.Status == StatusRemove && change.DeletesTarget {
			if err := removeManagedTargetFile(change.TargetPath); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		if (change.Status == StatusAdd || change.Status == StatusUpdate) && change.WritesTarget {
			if err := WriteFileBytes(change.TargetPath, change.SourceBytes); err != nil {
				return err
			}
		}
	}
	if commitGitIgnore != nil {
		if err := commitGitIgnore(); err != nil {
			return err
		}
	}
	if commitSyncManifest != nil {
		if err := commitSyncManifest(); err != nil {
			return err
		}
	}
	return nil
}
```

Do not add rollback. The order remains capability gate, Sync preparation, Gitignore stale revalidation/preparation, Managed deletes, Managed writes, atomic Gitignore commit, and Sync commit last. This supports Managed-to-Gitignore delete-then-create while ensuring unsupported platforms fail before any preparation or mutation.

- [ ] **Step 7: Format and run target/apply tests**

Run:

```bash
gofmt -w src/internal/driftline/gitignore_target.go src/internal/driftline/gitignore_rewrite_test.go src/internal/driftline/gitignore_commit*.go src/internal/driftline/gitignore_target_windows_test.go src/internal/driftline/gitignore_target_unsupported_test.go src/internal/driftline/target_repository.go src/internal/driftline/target_repository_test.go
go test ./src/internal/driftline -run 'Test(PrepareGitIgnoreRewrite|TargetRepositoryApply)' -count=1
go test ./src/internal/driftline -count=1
```

Expected: all commands pass on the host. Compile the package tests for Windows and at least one unsupported target during final verification so every build-tag branch is checked.

- [ ] **Step 8: Commit atomic apply support**

```bash
git add src/internal/driftline/gitignore_target.go src/internal/driftline/gitignore_rewrite_test.go src/internal/driftline/gitignore_commit_unix.go src/internal/driftline/gitignore_commit_unix_test.go src/internal/driftline/gitignore_commit_windows.go src/internal/driftline/gitignore_commit_unsupported.go src/internal/driftline/gitignore_target_windows_test.go src/internal/driftline/gitignore_target_unsupported_test.go src/internal/driftline/target_repository.go src/internal/driftline/target_repository_test.go
git commit -m "feat: apply gitignore section atomically"
```

## Task 6: Integrate Check, Diff, Update, And Init Behavior

**Files:**

- Modify: `src/internal/driftline/commands/check.go`
- Modify: `src/internal/driftline/commands/diff.go`
- Create: `src/internal/driftline/commands/diff_test.go`
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

Add command tests for stable logical labels on existing, missing, and binary `.gitignore` diffs. Require `diff --git a/.gitignore b/.gitignore`, `--- a/.gitignore` or `--- /dev/null`, and `+++ b/.gitignore`; reject the Target Repository path, `os.TempDir()`, and `driftline-source-` or `driftline-diff-` temporary names. Also set Git color, external diff, and textconv configuration and prove the dedicated Gitignore diff remains uncolored and does not invoke those drivers.

Create `diff_test.go` with `TestPrintGitIgnoreDiffUsesPlannedSnapshotsInsteadOfLiveTarget`: after planning, replace the live target with changed bytes, a missing path, and a symlink, and assert the output still compares the planned original and desired snapshots without exposing target or external bytes. Add `TestPrintGitIgnoreDiffPreservesHeaderLikeContent` so content lines beginning with `---` or `+++` inside a hunk are not relabeled.

- [ ] **Step 3: Run command tests and verify RED**

Run:

```bash
go test ./src/internal/driftline/commands -run 'Test(GitIgnoreSectionCommandLifecycle|InitValidatesButDoesNotApplyGitIgnoreSection|UpdateRejectsMalformedGitIgnoreBeforeManagedWrites|UpdateRemovesUndeclaredGitIgnoreSectionAndKeepsFile|DiffReportsBinaryGitIgnoreChange|DiffMissingGitIgnoreUsesStableLogicalLabels|DiffDisablesGitColorConfiguration|DiffDisablesExternalAndTextconvDrivers|InitDoesNotInspectExistingGitIgnoreMarkers|UpdateManagedToGitIgnoreRecreatesGeneratedOnlyFile|UpdateGitIgnoreToManagedUsesExistingForceSemantics|PrintGitIgnoreDiffUsesPlannedSnapshotsInsteadOfLiveTarget|PrintGitIgnoreDiffPreservesHeaderLikeContent)$' -count=1
```

Expected: compilation or assertions fail because command reporting only accepts Managed `Changes`, `diff` ignores `Plan.GitIgnore`, and the dedicated snapshot renderer does not exist.

- [ ] **Step 4: Print complete plans in check and update**

Change the reporting helper to `printPlan` and accept `driftline.Plan`:

```go
func printPlan(w io.Writer, plan driftline.Plan) {
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
	printPlan(stdout, plan)
	if plan.HasDrift() {
		return errDrift
	}
```

Update both `runUpdate` reporting calls to pass `plan` rather than `plan.Changes`. After all command callers use `Plan.HasDrift`, remove the obsolete `HasDrift(changes []Change)` helper and inline its loop into `Plan.HasDrift`:

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

After the existing Managed change loop in `runDiff`, pass both planned snapshots to a dedicated renderer. Never pass `TargetPath` to this renderer or reopen the live `.gitignore` after planning:

```go
	if plan.GitIgnore != nil {
		if err := printGitIgnoreDiff(
			stdout,
			plan.GitIgnore.OriginalBytes,
			plan.GitIgnore.DesiredBytes,
			plan.GitIgnore.TargetMissing,
		); err != nil {
			return err
		}
	}
	if plan.HasDrift() {
		return errDrift
	}
```

Write the snapshots into a private temporary directory using fixed relative filenames and `0600` files. Run Git from that directory with color, external diff, and text conversion disabled. Use the platform null device as Git's missing left input, then normalize the public label to `/dev/null`:

```go
func printGitIgnoreDiff(w io.Writer, originalBytes, desiredBytes []byte, targetMissing bool) error {
	tempDir, err := os.MkdirTemp("", "driftline-diff-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	originalPath := filepath.Join(tempDir, "original")
	desiredPath := filepath.Join(tempDir, "desired")
	if err := os.WriteFile(originalPath, originalBytes, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(desiredPath, desiredBytes, 0o600); err != nil {
		return err
	}

	left := "original"
	if targetMissing {
		left = os.DevNull
	}
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv", "--", left, "desired")
	cmd.Dir = tempDir
	data, diffErr := cmd.CombinedOutput()
	data = relabelGitIgnoreDiff(data, targetMissing)
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if diffErr == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(diffErr, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("run git diff: %w", diffErr)
}

func relabelGitIgnoreDiff(data []byte, targetMissing bool) []byte {
	var output bytes.Buffer
	inHunk := false
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("@@ ")) {
			inHunk = true
		}
		if inHunk {
			output.Write(line)
			continue
		}
		switch {
		case bytes.HasPrefix(line, []byte("diff --git ")):
			fmt.Fprintf(&output, "diff --git a/%s b/%s\n", driftline.GitIgnorePath, driftline.GitIgnorePath)
		case bytes.HasPrefix(line, []byte("--- ")):
			if targetMissing {
				output.WriteString("--- /dev/null\n")
			} else {
				fmt.Fprintf(&output, "--- a/%s\n", driftline.GitIgnorePath)
			}
		case bytes.HasPrefix(line, []byte("+++ ")):
			fmt.Fprintf(&output, "+++ b/%s\n", driftline.GitIgnorePath)
		case bytes.HasPrefix(line, []byte("Binary files ")):
			if targetMissing {
				fmt.Fprintf(&output, "Binary files /dev/null and b/%s differ\n", driftline.GitIgnorePath)
			} else {
				fmt.Fprintf(&output, "Binary files a/%s and b/%s differ\n", driftline.GitIgnorePath, driftline.GitIgnorePath)
			}
		default:
			output.Write(line)
		}
	}
	return output.Bytes()
}
```

Relabel only Git's pre-hunk metadata and binary summary. Once the first `@@` hunk header appears, copy every remaining byte unchanged so header-like target content is never rewritten. The public output must contain only stable `.gitignore` labels and `/dev/null` missing semantics, never random temporary names or local absolute paths.

Remove the old `HasDrift(plan.Changes)` check. The dedicated remove status still receives a content diff because both complete planned byte snapshots are available.

- [ ] **Step 6: Update command help language**

In `printUsage`, use:

```text
  check            check Target Repository state against the Contract
  diff             show planned content changes
  update           reconcile Managed files, Gitignore section, and Sync manifest
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
gofmt -w src/internal/driftline/plan.go src/internal/driftline/commands/check.go src/internal/driftline/commands/diff.go src/internal/driftline/commands/diff_test.go src/internal/driftline/commands/update.go src/internal/driftline/commands/run.go src/internal/driftline/commands/commands_test.go
go test ./src/internal/driftline/commands -count=1
go test ./... -count=1
```

Expected: all commands pass.

- [ ] **Step 8: Commit command integration**

```bash
git add src/internal/driftline/plan.go src/internal/driftline/commands/check.go src/internal/driftline/commands/diff.go src/internal/driftline/commands/diff_test.go src/internal/driftline/commands/update.go src/internal/driftline/commands/run.go src/internal/driftline/commands/commands_test.go
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

Document these user-facing rules in prose: `init` validates but does not apply it; `check`, `diff`, and `update` reconcile it; outside bytes and duplicate outside entries are ignored; malformed or duplicate markers require manual repair; Template `.gitignore` may coexist; Managed `.gitignore` may not coexist while `[gitignore]` is present. State that packaged Linux and Darwin builds apply atomically, Windows retains safe no-follow parsing/check/diff but fails Gitignore updates before mutation, and other unsupported platforms may reject planning or apply according to safe-read and atomic-replacement support.

- [ ] **Step 2: Add the domain term to CONTEXT.md**

Add after Template file:

```markdown
**Gitignore section**:
The source-owned marker-delimited region in the Target Repository's root `.gitignore` that driftline reconciles from Contract `[gitignore].entries` while preserving target-owned bytes outside the markers.
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
set -e
base=$(git merge-base HEAD main)
test -z "$(git diff --name-only "$base" -- '*.go' | xargs gofmt -l)"
go test ./... -count=1
go test -race ./... -count=1
go vet ./...

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build_dir/driftline-linux-amd64" ./src/cmd/driftline
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$build_dir/driftline-windows-amd64.exe" ./src/cmd/driftline
for goos in linux windows plan9 freebsd; do
  extension=""
  if [ "$goos" = windows ]; then
    extension=".exe"
  fi
  CGO_ENABLED=0 GOOS="$goos" GOARCH=amd64 go test -c -o "$build_dir/driftline-$goos-core.test$extension" ./src/internal/driftline
  CGO_ENABLED=0 GOOS="$goos" GOARCH=amd64 go test -c -o "$build_dir/driftline-$goos-commands.test$extension" ./src/internal/driftline/commands
done

prettier --check README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md
markdownlint-cli2 README.md CONTEXT.md AGENTS.md docs/superpowers/plans/2026-07-19-contract-gitignore-section.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-19-contract-gitignore-section-design.md
git diff --check
```

Expected: changed Go files are already formatted; normal and race tests pass; `go vet` exits zero; Linux and Windows builds succeed; the core and command tests compile for Linux, Windows, Plan 9, and FreeBSD; temporary binaries are removed by the trap; formatting is unchanged; markdownlint reports zero errors; and `git diff --check` prints nothing.

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
