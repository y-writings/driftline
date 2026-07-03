# Init Force Adoption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `driftline init --force` so initial adoption can record existing regular Managed target files without overwriting them.

**Architecture:** Keep `init` as a Target manifest creation command: it still does not copy or inspect Managed file bytes. Add a boolean force-adoption option at the CLI layer and pass it to the Initial adoption Module, where preflight allows existing regular Managed target files only when that option is enabled. Documentation must describe `init --force` as adoption, not overwrite.

**Tech Stack:** Go, standard library filesystem APIs, existing `SourceClient`, existing TOML config reader/writer, existing `go test ./...` suite.

---

## File Structure

- Modify `src/internal/driftline/initial_adoption.go`: add `InitialAdoptionOptions.AdoptExistingManagedTargets`, distinguish regular-file existing targets from non-regular targets, and keep preflight-before-write ordering.
- Modify `src/internal/driftline/initial_adoption_test.go`: add module-level tests for force adoption and force rejection of non-regular paths.
- Modify `src/internal/driftline/commands/run.go`: add `InitOptions.Force`, parse value-less `init --force`, reject valued `init --force`, and update help text.
- Modify `src/internal/driftline/commands/init.go`: pass the new force-adoption option to `AdoptInitialTargetRepository`.
- Modify `src/internal/driftline/commands/commands_test.go`: add parser and command integration coverage for `init --force`.
- Modify `README.md`: document `init --force` as adopting existing regular Managed target files.
- Modify `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`: update canonical `init` semantics to include explicit force adoption.

Do not commit during execution unless the user explicitly requests it.

## Task 1: Initial Adoption Domain Behavior

**Files:**
- Modify: `src/internal/driftline/initial_adoption.go`
- Test: `src/internal/driftline/initial_adoption_test.go`

- [ ] **Step 1: Add failing Initial adoption tests for force adoption**

Add these tests after `TestAdoptInitialTargetRepositoryRejectsExistingManagedTarget` in `src/internal/driftline/initial_adoption_test.go`:

```go
func TestAdoptInitialTargetRepositoryForceAdoptsExistingManagedRegularFile(t *testing.T) {
	root := t.TempDir()
	writeInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml", "target-owned\n")
	writeInitialAdoptionTestFile(t, root, "templates/existing.txt", "target-template\n")
	source := &fakeInitialAdoptionSource{files: map[string][]byte{
		"y-writings/source-repo@abc123:templates/missing.txt": []byte("missing template\n"),
	}}

	err := AdoptInitialTargetRepository(InitialAdoptionOptions{
		Root:                        root,
		Source:                      source,
		Repository:                  "y-writings/source-repo",
		Commit:                      "abc123",
		Manifest:                    initialAdoptionManifest(),
		TargetConfig:                initialAdoptionTargetConfig(),
		AdoptExistingManagedTargets: true,
	})
	if err != nil {
		t.Fatalf("initial adoption failed: %v", err)
	}
	if got := readInitialAdoptionTestFile(t, root, ".github/workflows/ci.yaml"); got != "target-owned\n" {
		t.Fatalf("force adoption must not overwrite managed target, got %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/missing.txt"); got != "missing template\n" {
		t.Fatalf("missing template content mismatch: %q", got)
	}
	if got := readInitialAdoptionTestFile(t, root, "templates/existing.txt"); got != "target-template\n" {
		t.Fatalf("existing template should be skipped, got %q", got)
	}
	config := readInitialAdoptionTestFile(t, root, TargetConfigPath)
	if !strings.Contains(config, `[files.github-workflow]`) || !strings.Contains(config, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("target config should record adopted managed target:\n%s", config)
	}
	if got, want := source.reads, []string{"y-writings/source-repo@abc123:templates/missing.txt"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("force adoption must not read managed source bytes: got %#v want %#v", got, want)
	}
}

func TestAdoptInitialTargetRepositoryForceRejectsNonRegularManagedTargets(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, ".github/workflows/ci.yaml"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink to regular file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, "real-ci.yaml", "real\n")
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "real-ci.yaml"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "broken symlink",
			setup: func(t *testing.T, root string) {
				path := filepath.Join(root, ".github/workflows/ci.yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing-target", path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "parent path is file",
			setup: func(t *testing.T, root string) {
				writeInitialAdoptionTestFile(t, root, ".github", "not a directory\n")
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
				Root:                        root,
				Source:                      source,
				Repository:                  "y-writings/source-repo",
				Commit:                      "abc123",
				Manifest:                    initialAdoptionManifest(),
				TargetConfig:                initialAdoptionTargetConfig(),
				AdoptExistingManagedTargets: true,
			})
			if err == nil {
				t.Fatal("expected non-regular managed target to fail")
			}
			if initialAdoptionTestPathExists(t, root, TargetConfigPath) {
				t.Fatal("target manifest must not be written after non-regular managed target error")
			}
			if initialAdoptionTestPathExists(t, root, "templates/missing.txt") {
				t.Fatal("template files must not be written after non-regular managed target error")
			}
			if len(source.reads) != 0 {
				t.Fatalf("source files must not be read after non-regular managed target error: %#v", source.reads)
			}
		})
	}
}
```

In the existing `TestAdoptInitialTargetRepositoryRejectsExistingManagedTarget`, replace the exact error assertion with this guidance-aware assertion:

```go
			if err == nil || !strings.Contains(err.Error(), "managed target already exists") || !strings.Contains(err.Error(), "rerun with --force") {
				t.Fatalf("expected managed target guidance error, got %v", err)
			}
```

- [ ] **Step 2: Run domain tests and verify they fail**

Run:

```sh
go test ./src/internal/driftline -run 'TestAdoptInitialTargetRepositoryForce' -count=1
```

Expected: FAIL because `InitialAdoptionOptions` has no `AdoptExistingManagedTargets` field.

- [ ] **Step 3: Implement force adoption in the Initial adoption Module**

In `src/internal/driftline/initial_adoption.go`, replace `InitialAdoptionOptions` with:

```go
type InitialAdoptionOptions struct {
	Root                        string
	Source                      SourceClient
	Repository                  string
	Commit                      string
	Manifest                    SourceManifest
	TargetConfig                TargetConfig
	AdoptExistingManagedTargets bool
}
```

Replace `collectTemplates` and `initialAdoptionPathExists` with:

```go
func (a initialAdoption) collectTemplates(root string) ([]initialAdoptionTemplate, error) {
	type missingTemplate struct {
		sourcePath string
		targetPath string
	}

	missingTemplates := []missingTemplate{}
	templates := []initialAdoptionTemplate{}
	for _, entry := range SourceEntries(a.opts.Manifest) {
		if IsReservedTargetPath(entry.Path) {
			return nil, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := PathWithin(root, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return nil, err
		}
		info, exists, err := initialAdoptionPathInfo(targetPath)
		if err != nil {
			return nil, err
		}

		switch entry.Mode {
		case ModeManaged:
			if !exists {
				continue
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("managed target is not a regular file: %s", entry.Path)
			}
			if !a.opts.AdoptExistingManagedTargets {
				return nil, fmt.Errorf("managed target already exists: %s (rerun with --force to adopt existing regular files)", entry.Path)
			}
		case ModeTemplate:
			if exists {
				continue
			}
			missingTemplates = append(missingTemplates, missingTemplate{sourcePath: entry.Path, targetPath: targetPath})
		}
	}
	for _, template := range missingTemplates {
		data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, template.sourcePath)
		if err != nil {
			return nil, fmt.Errorf("source template not found in source repository: %w", err)
		}
		templates = append(templates, initialAdoptionTemplate{targetPath: template.targetPath, sourceBytes: data})
	}
	return templates, nil
}

func initialAdoptionPathInfo(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		return info, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

func initialAdoptionPathExists(path string) (bool, error) {
	_, exists, err := initialAdoptionPathInfo(path)
	return exists, err
}
```

- [ ] **Step 4: Run domain tests and verify they pass**

Run:

```sh
go test ./src/internal/driftline -run 'TestAdoptInitialTargetRepository|TestInitialAdoption' -count=1
```

Expected: PASS.

## Task 2: CLI Parsing And Init Wiring

**Files:**
- Modify: `src/internal/driftline/commands/run.go`
- Modify: `src/internal/driftline/commands/init.go`
- Test: `src/internal/driftline/commands/commands_test.go`

- [ ] **Step 1: Add failing parser tests for `init --force`**

In `src/internal/driftline/commands/commands_test.go`, add this block inside `TestParseOptionsAcceptsStandardFlagFormsAndUpdateForce` after the existing `initOpts` assertion:

```go
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
```

- [ ] **Step 2: Run parser tests and verify they fail**

Run:

```sh
go test ./src/internal/driftline/commands -run TestParseOptionsAcceptsStandardFlagFormsAndUpdateForce -count=1
```

Expected: FAIL because `InitOptions` has no `Force` field and `parseInitOptions` rejects `--force`.

- [ ] **Step 3: Implement value-less init force parsing**

In `src/internal/driftline/commands/run.go`, replace `InitOptions` with:

```go
type InitOptions struct {
	Repository string
	Ref        string
	TargetDir  string
	Force      bool
}
```

In `parseInitOptions`, add this block after the `target-dir` option block and before the `len(args[i]) > 0 && args[i][0] == '-'` check:

```go
		if ok, err := parseBoolOption(args, &i, "force"); err != nil {
			return opts, err
		} else if ok {
			if opts.Force {
				return opts, fmt.Errorf("--force may be provided once")
			}
			opts.Force = true
			continue
		}
```

Add this helper after `parseStringOption`:

```go
func parseBoolOption(args []string, index *int, name string) (bool, error) {
	arg := args[*index]
	for _, prefix := range []string{"--" + name, "-" + name} {
		if arg == prefix {
			return true, nil
		}
		if strings.HasPrefix(arg, prefix+"=") {
			return true, fmt.Errorf("%s does not accept a value", prefix)
		}
	}
	return false, nil
}
```

- [ ] **Step 4: Run parser tests and verify they pass**

Run:

```sh
go test ./src/internal/driftline/commands -run TestParseOptionsAcceptsStandardFlagFormsAndUpdateForce -count=1
```

Expected: PASS.

- [ ] **Step 5: Add failing command integration tests for `init --force`**

In `src/internal/driftline/commands/commands_test.go`, add this test after `TestInitCreatesTargetConfigAndPlacesTemplates`:

```go
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
	config := readFile(t, targetDir, driftline.TargetConfigPath)
	if !strings.Contains(config, `[files.github-workflow]`) || !strings.Contains(config, `ci = ".github/workflows/ci.yaml"`) {
		t.Fatalf("target config should record adopted managed target:\n%s", config)
	}
	if strings.Contains(config, "force") || strings.Contains(config, "release") || strings.Contains(config, "mise") {
		t.Fatalf("target config should not persist force or template entries:\n%s", config)
	}
	if !strings.Contains(stdout.String(), "created .driftline-target.toml from y-writings/source-repo@0123456789abcdef0123456789abcdef01234567") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
```

Then replace `TestInitFailsBeforeWritingWhenConfigOrManagedTargetExists` with:

```go
func TestInitFailsBeforeWritingWhenConfigOrManagedTargetExists(t *testing.T) {
	for name, tt := range map[string]struct {
		setup         func(string)
		wantGuidance  bool
	}{
		"target config exists": {
			setup: func(targetDir string) {
				writeFile(t, targetDir, driftline.TargetConfigPath, "existing\n")
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
```

- [ ] **Step 6: Run command init tests and verify the new force test fails**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestInitForceAdoptsExistingManagedRegularFile|TestInitFailsBeforeWritingWhenConfigOrManagedTargetExists' -count=1
```

Expected: FAIL because `runInit` parses `--force` but does not pass `opts.Force` to the Initial adoption Module yet.

- [ ] **Step 7: Wire init force into Initial adoption**

In `src/internal/driftline/commands/init.go`, add `AdoptExistingManagedTargets: opts.Force,` to the `driftline.InitialAdoptionOptions` literal:

```go
	if err := driftline.AdoptInitialTargetRepository(driftline.InitialAdoptionOptions{
		Root:                        opts.TargetDir,
		Source:                      source,
		Repository:                  opts.Repository,
		Commit:                      commit,
		Manifest:                    manifest,
		TargetConfig:                config,
		AdoptExistingManagedTargets: opts.Force,
	}); err != nil {
		return err
	}
```

- [ ] **Step 8: Run command init tests and verify they pass**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestInit|TestParseOptions' -count=1
```

Expected: PASS.

## Task 3: Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`

- [ ] **Step 1: Update README init documentation**

In `README.md`, add this section after the `--ref` example and before `## File Modes`:

````markdown
Use `--force` with `init` to adopt existing regular files at Managed target paths into the Target Config:

```sh
driftline init y-writings/source-repo --force
```

`init --force` does not overwrite file content. It records existing regular Managed target files in `.driftline-target.toml`; later `check` reports drift and `update` synchronizes those Managed files if their content differs from the Source Repository.
````

- [ ] **Step 2: Update the canonical TOML design**

In `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`, replace this `init` bullet:

```markdown
- Fail before writing when a managed default target path already exists. Do not automatically adopt or overwrite pre-existing managed target files during `init`.
```

with:

```markdown
- Fail before writing when a managed default target path already exists, unless `--force` is provided.
- With `--force`, adopt existing regular managed target files into target config without overwriting or comparing content.
- With `--force`, still reject existing target config, directories, symlinks, broken symlinks, parent path collisions, and reserved target paths.
- Without `--force`, advertise force only for existing regular managed target files that force can adopt.
```

- [ ] **Step 3: Run docs-adjacent command tests**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestHelp|TestInit|TestParseOptions' -count=1
```

Expected: PASS.

## Task 4: Final Verification

**Files:**
- Verify: `src/internal/driftline/initial_adoption.go`
- Verify: `src/internal/driftline/initial_adoption_test.go`
- Verify: `src/internal/driftline/commands/run.go`
- Verify: `src/internal/driftline/commands/init.go`
- Verify: `src/internal/driftline/commands/commands_test.go`
- Verify: `README.md`
- Verify: `docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md`

- [ ] **Step 1: Run focused domain tests**

Run:

```sh
go test ./src/internal/driftline -run 'TestAdoptInitialTargetRepository|TestInitialAdoption' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused command tests**

Run:

```sh
go test ./src/internal/driftline/commands -run 'TestInit|TestHelp|TestParseOptions' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Review final diff**

Run:

```sh
git diff -- src/internal/driftline/initial_adoption.go src/internal/driftline/initial_adoption_test.go src/internal/driftline/commands/run.go src/internal/driftline/commands/init.go src/internal/driftline/commands/commands_test.go README.md docs/superpowers/specs/2026-06-27-toml-managed-template-sync-design.md docs/superpowers/specs/2026-07-03-init-force-adoption-design.md docs/superpowers/plans/2026-07-03-init-force-adoption.md
```

Expected: diff shows only the approved `init --force` adoption behavior, docs, tests, and this plan.

- [ ] **Step 5: Check invariants before reporting completion**

Verify these statements from the final code and tests:

```text
init --force is value-less and boolean
init --force adopts existing regular Managed target files without overwriting
init --force does not read Managed source bytes
init without --force still fails on existing Managed target paths before writes
existing Target manifest still fails before Source Repository access
Template behavior is unchanged
non-regular Managed targets still fail with force enabled
force behavior is not persisted in Target config
update --force <group.file> behavior remains separate
canonical TOML design no longer contradicts init --force
```
