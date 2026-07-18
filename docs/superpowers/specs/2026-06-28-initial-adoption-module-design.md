# Initial Adoption Module Design

<!-- markdownlint-disable MD013 -->

## Status

Approved for implementation planning.

## Context

driftline's TOML redesign makes the current Contract and current Sync manifest the only sources of truth. `driftline init <owner/repo>` performs initial adoption for a Target Repository by creating `.driftline/sync.toml` and placing missing Template files.

The current `runInit` command owns too much Target Repository behavior. It validates Target Repository paths, rejects existing Managed file targets, collects Template file bytes, writes the Sync manifest, writes Template files, and prints the report. This makes the command Module shallow: the CLI Adapter knows the ordering rules that make initial adoption safe.

`runUpdate` already delegates Target Repository writes to a deeper Target Repository apply Module. Initial adoption should receive the same treatment without changing the public command behavior.

## Goal

Introduce a deep Initial adoption Module in the `driftline` package.

The Module must concentrate Target Repository initial adoption behavior while leaving Source Repository ref resolution, Contract loading, CLI option parsing, the early existing Sync manifest check, and user-facing output in the command layer.

## Non-Goals

- Do not change the `init` command output.
- Do not copy Managed file bytes during `init`.
- Do not apply newly introduced Template files during `update`.
- Do not introduce rollback for Template files already placed during `init`.
- Do not add compatibility with YAML, lock files, `path_overrides`, `if_not_exists`, or standalone `prune` behavior.
- Do not introduce a migration command or historical ownership state.

## Terminology

- **Source Repository**: the repository that defines file identity, source paths, and file mode.
- **Contract**: the Source Repository's ref-scoped declaration of file groups, stable file identifiers, source paths, and file modes.
- **Target Repository**: the repository where driftline places and synchronizes files from a Source Repository.
- **Sync manifest**: `.driftline/sync.toml`, the current record of Managed file placements.
- **Managed file**: a file that driftline keeps synchronized from the Source Repository.
- **Template file**: a Source Repository file used only for initial placement; after placement it becomes target-owned.
- **File key**: the stable `<group>.<file>` identity.

## Design

Add an Initial adoption Module in the `driftline` package. The command layer keeps the existing local Sync manifest check before Source Repository access so an already-initialized Target Repository fails deterministically without network access. The command layer calls the Module after resolving the Source Repository ref, reading the Contract, and deriving the Sync manifest.

Conceptual init flow:

```text
runInit
  validate CLI-level repository and target directory inputs
  reject existing Sync manifest before Source Repository access
  resolve Source Repository ref and commit
  read and parse Contract
  derive initial Sync manifest from Contract
  adopt Target Repository
  print current success message
```

The Initial adoption Module owns the Target Repository operation:

1. Defensively default an empty Target Repository root to `.`.
2. Reject adoption when `.driftline/sync.toml` already exists.
3. Reject any Contract entry targeting a reserved Target Repository path.
4. Reject adoption when a Managed file default target path already exists.
5. Skip Template file target paths that already exist.
6. Read source bytes for missing Template files and queue them for placement.
7. Prepare the Sync manifest write before writing Template files.
8. Place missing Template files.
9. Commit the Sync manifest last.

The command layer remains responsible for stdout and stderr. The Initial adoption Module returns only an error.

## Write Ordering

The write sequence must be:

```text
preflight all Contract entries
prepare Sync manifest temp file
write missing Template files
commit Sync manifest
```

This improves current failure behavior. Today, `runInit` writes the Sync manifest before Template file placement. If a Template file write fails, the Sync manifest can remain and a rerun fails immediately with `Sync manifest already exists`.

With the new sequence, Template file placement failure leaves the Sync manifest uncommitted. A rerun can skip any Template files that were already placed because Template files become target-owned after placement.

## Safety Guarantees

The Module provides these guarantees:

- If the Sync manifest already exists, no Target Repository write is attempted.
- If any Managed file default target already exists, no Target Repository write is attempted.
- If any Contract entry targets a reserved path, no Target Repository write is attempted.
- If any missing Template file cannot be read from the Source Repository, no Target Repository write is attempted.
- If Sync manifest preparation fails, no Template file write is attempted.
- If any Template file write fails, the Sync manifest is not committed.
- The Sync manifest is committed only after all queued Template files have been written.
- The Module does not attempt rollback after a Template file write succeeds.
- The Module does not write stdout or stderr.

The deliberately limited guarantee is the same shape as the Target Repository apply Module: the Sync manifest must not move ahead of Target Repository file operations. Full transactional rollback is out of scope because it would require preserving and restoring file content, symlink state, permissions, directory state, and partial filesystem failures.

## Interface Shape

Keep the adoption result minimal.

Expected behavior:

- Success returns `nil`.
- Failure returns an error.
- The Module does not return operation logs.
- The Module does not write stdout or stderr.
- The command layer remains responsible for the existing success message.

The exact Go names can be chosen during implementation, but the Module must represent Target Repository initial adoption rather than generic file utilities.

## Existing Behavior To Preserve

- `init` creates `.driftline/sync.toml` from the Contract.
- `init` writes Sync manifest entries for Managed files only.
- `init` preserves the input ref in the Sync manifest when `--ref` is provided.
- `init` places missing Template files at their source paths.
- `init` skips Template files whose target paths already exist.
- `init` does not record Template files in the Sync manifest.
- `init` does not copy Managed file bytes.
- `init` fails before writing when the Sync manifest already exists.
- `init` reports an existing Sync manifest before Source Repository access.
- `init` fails before writing when a Managed file default target already exists, including broken symlinks.
- `init` rejects reserved Target Repository paths.
- `init` prints the same success message as today.

## Testing Plan

Add focused tests so the Initial adoption Module has its own test surface.

Required cases:

- Adoption writes the Sync manifest, places missing Template files, skips existing Template files, and does not copy Managed file bytes.
- Existing Sync manifest returns an error before writing Template files.
- Existing Managed file target returns an error before writing the Sync manifest or Template files.
- Reserved target path returns an error before writing the Sync manifest or Template files.
- Missing Template file source bytes return an error before writing the Sync manifest or Template files.
- If Sync manifest preparation fails, Template files are not written.
- If Template file writing fails, the Sync manifest is not committed.
- If Sync manifest commit fails after Template files are written, the Module returns an error and does not rollback Template files.
- The `init` command still preserves current CLI output and option behavior by delegating only Target Repository adoption.

Existing command integration tests should remain useful, but ordering and Sync manifest commit guarantees should be testable through the new Module directly.

## Migration Notes

This is an internal architecture change for a pre-release CLI. No compatibility layer is required.

## Open Questions

None. The design intentionally keeps this change narrow and leaves Contract read-path deepening for later work.
