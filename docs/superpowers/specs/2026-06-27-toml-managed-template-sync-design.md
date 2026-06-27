# TOML Managed/Template Sync Design

## Status

This document is the canonical design for the next driftline configuration redesign.

This is an intentionally breaking change. Implementation agents must replace the existing YAML, lock-file, path override, and `if_not_exists` model instead of preserving it.

Historical design documents, plans, README examples, schemas, tests, and fixtures that describe `.driftline-source.yaml`, `.driftline-target.yaml`, `driftline-lock.yaml`, `path_overrides`, `if_not_exists`, or standalone `prune` are obsolete for this work. Do not use them as design references or compatibility requirements. They may only be used to locate stale implementation, tests, docs, or fixtures that must be removed or rewritten to match this document.

## Goal

Make driftline a source-to-target file sync tool with a small, readable TOML configuration model.

The source repository defines the file contract. The target repository defines target placement for actively managed files. Template files are initial placement aids and are not tracked in the target manifest after creation.

## Non-Negotiable Constraints

- Do not add compatibility parsing for the old YAML formats.
- Do not keep `driftline-lock.yaml` or replace it with another historical state file.
- Do not keep `if_not_exists`; replace it with source-owned `mode`.
- Do not keep `path_overrides`; target placement is the normal target-side path, not an override concept.
- Do not keep standalone prune behavior; removal is part of managed sync.
- Do not keep generated JSON schemas for the old YAML formats as current documentation.
- Do not preserve old command behavior, output shape, fixtures, or tests when they conflict with this design.
- Do not treat earlier config design docs as design authority for this work.

## Product Responsibility

driftline synchronizes files from a source repository into a target repository.

driftline does not act as a package manager and does not retain historical ownership state. Current source config plus current target config define the desired managed set.

Source config owns:

- group identifiers,
- file identifiers,
- source paths,
- file mode: `managed` or `template`.

Target config owns:

- source repository and ref,
- target paths for currently managed files only.

## Terminology

- Group ID: a cognitive grouping under `[files.<group>]`, such as `github-workflow` or `mise`.
- File ID: a stable identifier inside a group, such as `ci` or `release`.
- File key: `<group>.<file>`, for example `github-workflow.ci`.
- Managed file: a source-controlled file that driftline keeps synchronized.
- Template file: a source-provided initial file that becomes a normal target-owned file after creation.
- Target manifest: `.driftline-target.toml`.

Group IDs and file IDs must be restricted to TOML bare-key-friendly names: letters, numbers, `_`, and `-`. Do not allow dots, slashes, whitespace, or quoted-key-only identifiers.

## Source Config

The source repository owns `.driftline-source.toml` at its repository root.

Canonical shape:

```toml
version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
```

Rules:

- `version = 2` is required.
- `[files.<group>]` groups related files for readability.
- Each entry under `[files.<group>]` is one file keyed by stable file ID.
- Each file entry must be an inline table with `path` and `mode`.
- `path` is the source repository path.
- `mode` must be `managed` or `template`.
- Unknown fields are invalid.
- Duplicate normalized source paths are invalid.
- Root `gitignore` behavior from the old YAML design is removed. If a source repository needs to provide `.gitignore`, define it as a normal managed or template file.

## Target Config

The target repository owns `.driftline-target.toml` at its repository root.

Canonical shape:

```toml
version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
```

Rules:

- `version = 2` is required.
- `[source]` is required.
- `source.repository` is required and uses `owner/repo` form.
- `source.ref` is required.
- `[files.<group>]` contains managed files only.
- Each file value is the target repository path for that managed file.
- Template files are not recorded in target config.
- Unknown fields are invalid.
- Duplicate normalized target paths are invalid.

`.driftline-target.toml` is both human-editable and driftline-updatable. `driftline update` may rewrite it to add newly managed files, remove no-longer-managed files, and remove files that changed from managed to template.

## Path Validation

Source and target paths must be normalized repository-relative paths.

Invalid paths include:

- empty paths,
- absolute paths,
- paths with leading or trailing whitespace,
- paths containing `..` path segments,
- paths containing backslashes,
- paths ending in `/`,
- `.`.

## Mode Semantics

### Managed

Managed files are source-controlled.

For an active managed file:

- If target config has a target path, sync source bytes to that path.
- If target config is missing the file key, add it with the default target path equal to the source path.
- If the chosen target path does not exist, create it.
- If the chosen target path exists and is not already in target config for that file key, report a conflict instead of overwriting.
- If source content differs from the target file, update the target file.

When a managed file is removed from source config:

- Remove its target config entry.
- Delete its target file using the target path currently declared in target config.
- Remove an empty `[files.<group>]` table from target config.

When a file changes from `managed` to `template` in source config:

- Remove its target config entry.
- Leave the target file untouched.
- Treat the target file as target-owned from that point forward.

### Template

Template files are source-provided initial files. They are not ongoing sync targets.

For a template file:

- Create it only during initial template placement when the target path is missing.
- Do not record it in target config.
- Do not update it after creation.
- Do not delete it when source config removes it.
- If a template path already exists, skip it without modifying the file.

Template placement is part of initial adoption. Normal `update` must not silently add newly introduced templates to an existing target repository.

## Command Semantics

### init

`driftline init <owner/repo>` reads `.driftline-source.toml` from the source repository and creates `.driftline-target.toml` in the target repository.

`init` behavior:

- Write target config entries for source files with `mode = "managed"`.
- Use the source path as the default target path for each managed file.
- Place template files at their source paths only if the target path is missing.
- Skip template files whose target path already exists.
- Do not record template files in target config.
- Fail before writing if target config already exists.
- Fail before writing when a managed default target path already exists. Do not automatically adopt or overwrite pre-existing managed target files during `init`.

### check

`driftline check` builds the same plan as `update` and reports changes without writing files or config.

`check` must report:

- managed file additions,
- managed file updates,
- managed file removals,
- target config additions,
- target config removals,
- mode transitions,
- conflicts.

### diff

`driftline diff` reports content diffs for managed file additions and updates. It also reports non-content plan items such as target config changes, removals, mode transitions, and conflicts.

### update

`driftline update` applies the managed sync plan.

`update` behavior:

- Resolve the full plan before writing anything.
- If any conflict exists, fail without writing files or target config.
- Add missing managed entries to target config.
- Remove no-longer-managed entries from target config.
- Remove entries that changed from managed to template.
- Write managed target files for adds and updates.
- Delete target files for removed managed entries.
- Do not apply newly introduced templates to existing target repositories.
- Do not write any lock file or state file.

### prune

There is no standalone `prune` responsibility in the new design. Remove the standalone `prune` command from the CLI. Do not keep an alias or compatibility stub.

## Conflict Handling

Conflict handling must prefer user safety over silent overwrite.

A conflict occurs when driftline needs to write a managed file to a target path that already exists but is not currently declared in target config for that file key.

Typical conflict causes:

- A formerly template-created file now has a source `managed` entry at the same path.
- A user-created target file occupies the default managed target path.
- Two managed entries resolve to the same target path.

Conflict output must explain the exact file key and target path, then give actionable choices:

```text
conflict github-workflow.ci: target already exists
  target: .github/workflows/ci.yaml
  source mode: managed

Choose one:
  1. set another target path in .driftline-target.toml
  2. move the existing target file
  3. rerun with --force github-workflow.ci to overwrite
```

Implement force overwrite as a one-time `driftline update --force <group.file>` CLI action. Do not persist force behavior in target config.

## No Lock Or Historical State

Do not write or read `driftline-lock.yaml`.

The new design intentionally does not preserve previous target ownership state. The current source config and current target config are the only sources of truth.

This means:

- managed files can be deleted because they are listed in current target config,
- template files are safe from deletion because they are not listed in target config,
- old unknown files are target-owned and ignored,
- old stale target config entries are removed as managed sync cleanup.

## Migration Policy

This repository is pre-release. Implement this redesign directly.

Required migration behavior:

- Replace `.driftline-source.yaml` with `.driftline-source.toml`.
- Replace `.driftline-target.yaml` with `.driftline-target.toml`.
- Remove `driftline-lock.yaml` behavior.
- Remove YAML schemas or stop presenting them as current schemas.
- Rewrite README examples to TOML.
- Rewrite tests and fixtures to TOML.
- Reject old YAML files instead of parsing them.

Do not add migration commands, compatibility shims, fallback readers, deprecated aliases, dual-format parsing, or old-output compatibility unless the user explicitly asks for them later.

## Implementation Notes

Keep the internal planning model simple:

- Parse source config into active source entries keyed by `<group>.<file>`.
- Parse target config into managed target entries keyed by `<group>.<file>`.
- Build a desired managed set from source entries with `mode = "managed"`.
- Compare desired managed keys with target config keys.
- Add missing managed keys to target config with default target path equal to source path.
- Remove target config keys that are absent from the desired managed set.
- Resolve target paths and detect conflicts before any write.
- Apply target config changes and file changes only after conflict-free planning.

The source path is not duplicated in target config. File identity is the stable `<group>.<file>` key, so source-side renames are followed by keeping the same file key and changing only the source `path`.

## Testing Requirements

Tests must cover:

- parsing valid source TOML,
- parsing valid target TOML,
- rejecting unknown source fields,
- rejecting unknown target fields,
- rejecting invalid modes,
- rejecting invalid paths,
- rejecting invalid group and file IDs,
- `init` writes managed target config and places missing templates,
- `init` does not record templates in target config,
- `check` reports target config additions and removals,
- `update` auto-updates target config,
- managed file add/update/delete behavior,
- managed-to-template transition leaves target file and removes target config entry,
- template-to-managed transition conflicts when the target path already exists,
- source deletion of template does nothing because template is not in target config,
- no command reads or writes `driftline-lock.yaml`,
- standalone `prune` command is removed from the CLI without a compatibility alias.

## Out Of Scope

- Multi-source target configs.
- Historical ownership tracking.
- Automatic migration from YAML.
- Persisted force-overwrite settings.
- Template updates after initial placement.
- Interactive conflict resolution.
