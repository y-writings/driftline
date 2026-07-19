# TOML Managed/Template Sync Design

<!-- markdownlint-disable MD013 -->

## Status

This document is the canonical design for the next driftline configuration redesign.

This is an intentionally breaking change. Implementation agents must replace the existing YAML, lock-file, path override, and `if_not_exists` model instead of preserving it.

Historical design documents, plans, README examples, schemas, tests, and fixtures that describe `.driftline-source.yaml`, `.driftline-target.yaml`, `driftline-lock.yaml`, `path_overrides`, `if_not_exists`, or standalone `prune` are obsolete for this work. Do not use them as design references or compatibility requirements. They may only be used to locate stale implementation, tests, docs, or fixtures that must be removed or rewritten to match this document.

## Goal

Make driftline a source-to-target file sync tool with a small, readable TOML configuration model.

The Source Repository defines the Contract. The Target Repository defines target placement for actively Managed files. Template files are initial placement aids and are not tracked in the Sync manifest after creation.

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

driftline does not act as a package manager and does not retain historical ownership state. The current Contract plus current Sync manifest define the desired Managed set.

The Contract owns:

- group identifiers,
- file identifiers,
- source paths,
- file mode: `managed` or `template`.

The Sync manifest owns:

- source repository and ref,
- target paths for currently managed files only.

## Terminology

- Group ID: a cognitive grouping under `[files.<group>]`, such as `github-workflow` or `mise`.
- File ID: a stable identifier inside a group, such as `ci` or `release`.
- File key: `<group>.<file>`, for example `github-workflow.ci`.
- Managed file: a source-controlled file that driftline keeps synchronized.
- Template file: a source-provided initial file that becomes a normal target-owned file after creation.
- Contract: `.driftline/contract.toml`, the Source Repository's ref-scoped file declaration.
- Sync manifest: `.driftline/sync.toml`, the Target Repository's current Managed file mapping.

Group IDs and file IDs must be restricted to TOML bare-key-friendly names: letters, numbers, `_`, and `-`. Do not allow dots, slashes, whitespace, or quoted-key-only identifiers.

## Contract

The Source Repository owns `.driftline/contract.toml`.

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

- The Contract is parsed as TOML 1.1.
- `version = 2` is required.
- `[files.<group>]` groups related files for readability.
- Each entry under `[files.<group>]` is one file keyed by stable file ID.
- Each file entry must be an inline table with `path` and `mode`.
- `path` is the source repository path.
- `mode` must be `managed` or `template`.
- Unknown fields are invalid.
- Duplicate normalized source paths are invalid.
- Root `gitignore` behavior from the old YAML design remains removed. Marker-scoped Contract `[gitignore]` behavior is defined separately by `2026-07-19-contract-gitignore-section-design.md`; it is not compatibility with the old append-only behavior.

## Sync Manifest

The Target Repository owns `.driftline/sync.toml`.

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

- The Sync manifest is parsed as TOML 1.1.
- `version = 2` is required.
- `[source]` is required.
- `source.repository` is required and uses `owner/repo` form.
- `source.ref` is required.
- `[files.<group>]` contains Managed files only.
- Each file value is the Target Repository path for that Managed file.
- Template files are not recorded in the Sync manifest.
- Unknown fields are invalid.
- Duplicate normalized target paths are invalid.

`.driftline/sync.toml` is both human-editable and driftline-updatable. `driftline update` may rewrite it to add entries for newly Managed files, remove entries for no-longer-Managed files, and remove entries for files that changed from Managed to Template.

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

The complete `.driftline/` subtree is reserved for driftline metadata. A Managed or Template source or target path must not equal `.driftline` or start with `.driftline/`.

## Mode Semantics

### Managed

Managed files are source-controlled.

For an active Managed file:

- If the Sync manifest has a target path, sync source bytes to that path.
- If the Sync manifest is missing the File key, add it with the default target path equal to the source path.
- If the chosen target path does not exist, create it.
- If the chosen target path exists and is not already in the Sync manifest for that File key, report a conflict instead of overwriting.
- If source content differs from the target file, update the target file.

When a Managed file is removed from the Contract:

- Remove its Sync manifest entry.
- Delete its target file using the target path currently declared in the Sync manifest.
- Remove an empty `[files.<group>]` table from the Sync manifest.

When a file changes from `managed` to `template` in the Contract:

- Remove its Sync manifest entry.
- Leave the target file untouched.
- Treat the target file as target-owned from that point forward.

### Template

Template files are source-provided initial files. They are not ongoing sync targets.

For a template file:

- Create it only during initial template placement when the target path is missing.
- Do not record it in the Sync manifest.
- Do not update it after creation.
- Do not delete it when the Contract removes it.
- If a template path already exists, skip it without modifying the file.

Template placement is part of initial adoption. Normal `update` must not silently add newly introduced templates to an existing target repository.

## Command Semantics

### init

`driftline init <owner/repo>` reads `.driftline/contract.toml` from the Source Repository and creates `.driftline/sync.toml` in the Target Repository.

`init` behavior:

- Write Sync manifest entries for Contract files with `mode = "managed"`.
- Use the source path as the default target path for each managed file.
- Place template files at their source paths only if the target path is missing.
- Skip template files whose target path already exists.
- Do not record Template files in the Sync manifest.
- Fail before writing if the Sync manifest already exists.
- Fail before writing when a managed default target path already exists, unless `--force` is provided.
- With `--force`, adopt existing regular files at Managed file target paths into the Sync manifest without overwriting or comparing content.
- With `--force`, still reject an existing Sync manifest, directories, symlinks, broken symlinks, parent path collisions, and reserved target paths.
- Without `--force`, advertise force only for existing regular files at Managed file target paths that force can adopt.

### check

`driftline check` builds the same plan as `update` and reports changes without writing files or the Sync manifest.

`check` must report:

- managed file additions,
- managed file updates,
- managed file removals,
- Sync manifest additions,
- Sync manifest removals,
- mode transitions,
- conflicts.

### diff

`driftline diff` reports content diffs for Managed file additions and updates. It also reports non-content plan items such as Sync manifest changes, removals, mode transitions, and conflicts.

### update

`driftline update` applies the managed sync plan.

`update` behavior:

- Resolve the full plan before writing anything.
- If any conflict exists, fail without writing files or the Sync manifest.
- Add missing Managed entries to the Sync manifest.
- Remove no-longer-Managed entries from the Sync manifest.
- Remove Sync manifest entries for files that changed from Managed to Template.
- Write target files for managed adds and updates.
- Delete target files for removed managed entries.
- Do not apply newly introduced templates to existing target repositories.
- Do not write any lock file or state file.

### prune

There is no standalone `prune` responsibility in the new design. Remove the standalone `prune` command from the CLI. Do not keep an alias or compatibility stub.

## Conflict Handling

Conflict handling must prefer user safety over silent overwrite.

A conflict occurs when driftline needs to write a Managed file to a target path that already exists but is not currently declared in the Sync manifest for that File key.

Typical conflict causes:

- A formerly template-created file now has a source `managed` entry at the same path.
- A user-created target file occupies the Managed file default target path.
- Two managed entries resolve to the same target path.

Conflict output must explain the exact file key and target path, then give actionable choices:

```text
conflict github-workflow.ci: target already exists
  target: .github/workflows/ci.yaml
  source mode: managed

Choose one:
  1. set another target path in .driftline/sync.toml
  2. move the existing target file
  3. rerun with --force github-workflow.ci to overwrite
```

Implement force overwrite as a one-time `driftline update --force <group.file>` CLI action. Do not persist force behavior in the Sync manifest.

## No Lock Or Historical State

Do not write or read `driftline-lock.yaml`.

The new design intentionally does not preserve previous target ownership state. The current Contract and current Sync manifest are the only sources of truth.

This means:

- Managed files can be deleted because they are listed in the current Sync manifest,
- Template files are safe from deletion because they are not listed in the Sync manifest,
- old unknown files are target-owned and ignored,
- old stale Sync manifest entries are removed as Managed sync cleanup.

## Migration Policy

This repository is pre-release. Implement this redesign directly.

Historical migration context: the earlier TOML redesign replaced `.driftline-source.yaml` and `.driftline-target.yaml` with `.driftline-source.toml` and `.driftline-target.toml`. Those root-level TOML paths are also old and unsupported under the current metadata layout. Implementations read only `.driftline/contract.toml` and `.driftline/sync.toml`.

Required migration behavior:

- Remove `driftline-lock.yaml` behavior.
- Remove YAML schemas or stop presenting them as current schemas.
- Rewrite README examples to TOML.
- Rewrite tests and fixtures to TOML.
- Do not read old YAML files or old root-level TOML metadata files.

Do not add migration commands, compatibility shims, fallback readers, deprecated aliases, dual-format parsing, or old-output compatibility unless the user explicitly asks for them later.

## Implementation Notes

Keep the internal planning model simple:

- Parse the Contract into active source entries keyed by `<group>.<file>`.
- Parse the Sync manifest into Managed entries keyed by `<group>.<file>`.
- Build a desired managed set from source entries with `mode = "managed"`.
- Compare desired Managed keys with Sync manifest keys.
- Add missing Managed keys to the Sync manifest with default target path equal to source path.
- Remove Sync manifest keys that are absent from the desired Managed set.
- Resolve target paths and detect conflicts before any write.
- Apply Sync manifest changes and file changes only after conflict-free planning.

The source path is not duplicated in the Sync manifest. File identity is the stable `<group>.<file>` key, so source-side renames are followed by keeping the same File key and changing only the source `path`.

## Testing Requirements

Tests must cover:

- parsing valid Contract TOML,
- parsing valid Sync manifest TOML,
- rejecting unknown Contract fields,
- rejecting unknown Sync manifest fields,
- rejecting invalid modes,
- rejecting invalid paths,
- rejecting invalid group and file IDs,
- `init` writes the Sync manifest and places missing templates,
- `init` does not record templates in the Sync manifest,
- `check` reports Sync manifest additions and removals,
- `update` auto-updates the Sync manifest,
- managed file add/update/delete behavior,
- managed-to-template transition leaves target file and removes Sync manifest entry,
- template-to-managed transition conflicts when the target path already exists,
- Contract deletion of Template does nothing because Template is not in the Sync manifest,
- no command reads or writes `driftline-lock.yaml`,
- standalone `prune` command is removed from the CLI without a compatibility alias.

## Out Of Scope

- Multi-source Sync manifests.
- Historical ownership tracking.
- Automatic migration from YAML.
- Persisted force-overwrite settings.
- Template updates after initial placement.
- Interactive conflict resolution.
