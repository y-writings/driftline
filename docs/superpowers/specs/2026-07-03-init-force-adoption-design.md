# Init Force Adoption Design

## Status

Approved for implementation planning.

## Context

`driftline init <owner/repo>` creates `.driftline-target.toml` for initial Target Repository adoption. It records Managed files in the Target manifest and places missing Template files, but it does not copy or inspect Managed file bytes during `init`.

The current TOML design intentionally fails before writing when a Managed file default target path already exists. That preserves safety, but it blocks adopting an existing Target Repository that already has files at the paths the Source Config wants to manage.

This design adds an explicit `init --force` escape hatch for that adoption case. In this command, force means adopting existing regular files into the Target manifest. It does not mean overwriting file content.

## Goal

Allow `driftline init` to complete when Managed file default target paths already contain regular files, but only when the user explicitly passes `--force`.

## Non-Goals

- Do not make `init` copy Managed file bytes.
- Do not make `init` read Managed source bytes or compare content.
- Do not overwrite existing Target Repository files during `init --force`.
- Do not allow `--force` to recreate or overwrite an existing `.driftline-target.toml`.
- Do not change Template file behavior.
- Do not introduce persisted force state in `.driftline-target.toml`.
- Do not add compatibility with YAML, lock files, `path_overrides`, `if_not_exists`, or standalone `prune` behavior.

## CLI Semantics

`init` accepts a value-less boolean `--force` flag.

Accepted forms:

```sh
driftline init --force owner/repo
driftline init owner/repo --force
```

Rejected forms:

```sh
driftline init owner/repo --force=true
driftline init owner/repo --force github-workflow.ci
```

`init --force` means: adopt existing regular files at Managed file default target paths into the initial Target manifest.

`init --force` must not allow re-initializing a Target Repository that already has `.driftline-target.toml`.

The `update --force <group.file>` behavior remains separate. `update --force` is a one-time overwrite for a specific Managed file conflict. `init --force` is a boolean adoption decision and does not overwrite.

## Adoption Rules

Without `--force`, existing behavior remains: if a Managed file default target path already exists, `init` fails before writing the Target manifest or placing Template files.

When the existing path is a regular file that `--force` can adopt, the failure should guide the user to rerun with `--force`, for example:

```text
managed target already exists: .github/workflows/ci.yaml (rerun with --force to adopt existing regular files)
```

When the existing path is non-regular, such as a directory or symlink, the failure should not suggest `--force` because `--force` cannot adopt it.

With `--force`, `init` allows an existing file at a Managed file default target path only when `os.Lstat` reports a regular file at that exact path.

With `--force`, `init` still rejects:

- existing `.driftline-target.toml`,
- directories,
- symlinks, including symlinks to regular files,
- broken symlinks,
- parent path collisions, such as `.github` being a file for `.github/workflows/ci.yaml`,
- reserved target paths.

Existing regular files at Managed file target paths are left untouched. Their bytes are not compared with source bytes during `init`.

Template behavior does not change:

- Missing Template files are placed.
- Existing Template paths are skipped.
- Template files are never recorded in the Target manifest.
- `--force` does not overwrite Template files.

If multiple Managed file default target paths already exist, `init --force` adopts all existing regular files. If any Source Config entry fails preflight, no Target manifest is committed and no Template file is placed.

## Flow And Boundaries

`runInit` remains responsible for:

- CLI parsing,
- repository and ref validation,
- Target Repository directory validation,
- early existing Target manifest detection before Source Repository access,
- Source Config loading,
- deriving the initial Target manifest,
- stdout,
- passing the force-adopt choice into the Initial adoption Module.

The Initial adoption Module remains responsible for Target Repository preflight and writes. Its options gain a boolean such as `AdoptExistingManagedTargets`.

The module preflights every Source Config entry before writing anything:

- Reserved target paths fail.
- Managed entries fail if their target path exists and `AdoptExistingManagedTargets` is false.
- Managed entries are accepted when their target path exists as a regular file and `AdoptExistingManagedTargets` is true.
- Managed entries still fail for non-regular existing paths.
- Template entries are queued for placement only when the target path is missing.
- Missing Template source bytes are read and queued before any Target Repository write.

The write order remains:

```text
preflight all Source Config entries
prepare Target manifest temp file
write missing Template files
commit Target manifest
```

This preserves the current commit-last safety rule: the Target manifest must not move ahead of Target Repository file operations.

## User Output And Follow-Up Behavior

Successful `init --force` keeps the existing success output:

```text
created .driftline-target.toml from owner/repo@commit
```

`init --force` does not print an adopted-file list.

After `init --force`, adopted files are normal Managed files because they are recorded in `.driftline-target.toml`. A following `check` reports content drift if existing target bytes differ from source. A following `update` can synchronize those files without another `--force` because the files are no longer target-owned from driftline's perspective.

## Documentation Updates

Update README and command help to describe `init --force` as adopting existing regular files at Managed file target paths. Do not describe it as overwriting.

The canonical TOML managed/template sync design must be updated so its `init` section allows this explicit force-adoption path instead of stating that existing Managed file default target paths always fail.

## Testing Plan

Add command parsing tests for:

- accepting value-less `--force` before the repository,
- accepting value-less `--force` after the repository,
- rejecting `--force=true`,
- rejecting `--force <value>`.

Add command behavior tests for:

- `init` without `--force` still fails before writes on an existing file at a Managed file target path and includes force guidance,
- `init --force` succeeds when the existing file at a Managed file target path is a regular file,
- `init --force` does not modify existing target file bytes,
- `init --force` still places missing Template files and skips existing Template files,
- `init --force` still rejects existing `.driftline-target.toml`.

Add Initial adoption Module tests for:

- accepting existing regular files at Managed file target paths only when force adoption is enabled,
- rejecting directories at Managed file target paths with force adoption enabled,
- rejecting symlinks at Managed file target paths with force adoption enabled,
- rejecting broken symlinks at Managed file target paths with force adoption enabled,
- rejecting parent path collisions with force adoption enabled,
- preserving no-write behavior when any preflight check fails.

## Migration Notes

This is a pre-release CLI behavior change. No compatibility layer is required.
