# Driftline Full Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `driftline` the canonical name for the Go module, CLI command, internal package, Docker binary, manifest file, manifest schema, and lock file.

**Architecture:** This is a breaking rename with no compatibility shim. Tests describe the new public behavior first, then the command, internal package, manifest schema, source-directory option, Dockerfile, and docs are updated to the new names.

**Tech Stack:** Go 1.26.3, `gopkg.in/yaml.v3`, Docker, Debian bookworm.

---

## File Structure

- Modify: `go.mod` - use module path `github.com/y-writings/driftline`.
- Create: `src/cmd/driftline/main.go` - entrypoint for the `driftline` CLI.
- Create: `src/internal/driftline/*.go` - core planning, I/O, and types under the `driftline` package.
- Create: `src/internal/driftline/commands/*.go` - CLI command handlers using the `driftline` internal package.
- Modify: `Dockerfile` - build and install `/usr/local/bin/driftline` from `./src/cmd/driftline`.
- Delete: stale root lock file from the previous naming scheme.
- Modify: docs under `docs/superpowers/` - keep current command/module/binary references aligned with `driftline`.

Commit steps are omitted from this plan because this environment requires an explicit user request before committing.

## Task 1: Update Tests To Describe Driftline

**Files:**
- Modify: command package tests under `src/internal/driftline/commands/`.

- [ ] **Step 1: Write failing tests for the new CLI names**

Use manifests named `driftline.yaml` with the `files` collection key. Use `--source-dir` for source input, assert usage output contains `usage: driftline`, and assert lock output is written to `.driftline.lock` by default.

Example test manifest content:

```yaml
version: 1
files:
  - id: sample
    source: sample.txt
    target: sample.txt
```

- [ ] **Step 2: Run command tests and confirm RED**

Run the command package test target before implementation.

Expected: FAIL because the current implementation has not yet exposed the new CLI option, manifest path, manifest schema, usage text, or lock path.

## Task 2: Rename Module, Command, Package, and Core Types

**Files:**
- Modify: `go.mod`
- Create: `src/cmd/driftline/main.go`
- Create: `src/internal/driftline/types.go`
- Create: `src/internal/driftline/io.go`
- Create: `src/internal/driftline/plan.go`
- Create: `src/internal/driftline/commands/*.go`

- [ ] **Step 1: Update `go.mod`**

Change `go.mod` to:

```go.mod
module github.com/y-writings/driftline

go 1.26

toolchain go1.26.3

require gopkg.in/yaml.v3 v3.0.1
```

- [ ] **Step 2: Create the CLI entrypoint**

Create `src/cmd/driftline/main.go` with:

```go
package main

import (
	"fmt"
	"os"

	"github.com/y-writings/driftline/src/internal/driftline/commands"
)

func main() {
	if err := commands.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Update core types**

Use package `driftline`, `Manifest.File []ManifestFile`, YAML key `files`, and `Options.SourceDir`.

Required default options:

```go
func DefaultOptions() Options {
	return Options{
		ManifestPath: "driftline.yaml",
		LockPath:     ".driftline.lock",
		SourceDir:    ".",
		TargetDir:    ".",
	}
}
```

- [ ] **Step 4: Update command parsing**

Use `driftline` as the flag set name and expose `--source-dir`.

Required usage prefix:

```text
usage: driftline <command> [options]
```

Required option defaults:

```text
--source-dir string  source directory (default ".")
--manifest string    manifest path relative to source dir (default "driftline.yaml")
--lock string        lock file path relative to target dir (default ".driftline.lock")
```

- [ ] **Step 5: Update command handlers**

Import `github.com/y-writings/driftline/src/internal/driftline`, use `driftline.Options`, `driftline.Change`, `driftline.ManifestFile`, and `manifest.File` throughout command handlers.

- [ ] **Step 6: Format and verify command package**

Run: `gofmt -w src/cmd/driftline src/internal/driftline`

Run: `go test ./src/internal/driftline/commands`

Expected: PASS.

## Task 3: Update Dockerfile and Existing Docs

**Files:**
- Modify: `Dockerfile`
- Modify: `docs/superpowers/specs/2026-05-23-dockerfile-go-toolchain-design.md`
- Modify: `docs/superpowers/plans/2026-05-23-dockerfile-go-toolchain.md`

- [ ] **Step 1: Update Dockerfile**

Use the renamed command path and binary name:

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/driftline ./src/cmd/driftline
COPY --from=builder /out/driftline /usr/local/bin/driftline
ENTRYPOINT ["driftline"]
```

- [ ] **Step 2: Update current docs**

Ensure current docs use these references:

```text
./src/cmd/driftline
/out/driftline
/usr/local/bin/driftline
github.com/y-writings/driftline
docker build -t driftline .
```

## Task 4: Delete Stale Lock File and Verify Rename Completeness

**Files:**
- Delete: stale root lock file from the previous naming scheme.
- Verify: repository source and docs.

- [ ] **Step 1: Delete stale root lock file**

Remove the old root lock file rather than renaming it.

- [ ] **Step 2: Run full Go tests**

Run: `go test ./...`

Expected: all packages pass, with package paths under `github.com/y-writings/driftline`.

- [ ] **Step 3: Build the renamed command**

Run: `go build ./src/cmd/driftline`

Expected: command exits successfully. Remove the local build artifact after confirming the build succeeds.

- [ ] **Step 4: Search for stale names in editable files**

Search editable source, tests, Dockerfile, and current docs for stale command, package, manifest, lock, module, and option names from the previous naming scheme.

Expected: no matches outside generated logs, Git metadata, or other historical session artifacts.

- [ ] **Step 5: Inspect working tree**

Run: `git status --short`

Expected: only intended paths are changed: `go.mod`, `Dockerfile`, files under `src/cmd` and `src/internal`, the deleted stale lock file, and docs under `docs/superpowers/`.

## Self-Review

- Spec coverage: module path, command path, package path, CLI usage, CLI flag, manifest path, manifest key, lock path, Docker binary, stale lock deletion, tests, and docs are covered by Tasks 1 through 4.
- Placeholder scan: no placeholder task remains.
- Type consistency: `ManifestFile`, `Manifest.File`, `Options.SourceDir`, `driftline.Options`, `--source-dir`, `driftline.yaml`, and `.driftline.lock` are used consistently across the plan.
