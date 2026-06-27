# Target Repository Apply Module Design

## Status

Approved for implementation planning.

## Context

driftline's current TOML redesign makes the current Source Config and current Target manifest the only sources of truth. `driftline update` must apply a Sync plan without writing anything when conflicts exist, must update Managed files, must remove stale Managed files, and must rewrite the Target manifest only when the managed set changes.

The current `runUpdate` command owns too much Target Repository behavior. It reads `Change` status and flags, deletes files, writes files, prepares the Target manifest rewrite, commits the rewrite, and prints the report. This makes the command Module shallow: the CLI Adapter knows the ordering rules that make Target Repository writes safe.

## Goal

Introduce a Deep Module for applying a conflict-free Sync plan to a Target Repository.

The new Module must concentrate the Target Repository write sequence and safety guarantees while leaving planning and reporting behavior unchanged.

## Non-Goals

- Do not redesign Sync plan construction.
- Do not move target path ownership or conflict detection out of `BuildPlan` in this change.
- Do not change `check` or `diff` behavior.
- Do not introduce rollback for file writes or deletes.
- Do not change human-readable output.
- Do not preserve any old YAML, lock-file, `path_overrides`, `if_not_exists`, or standalone `prune` behavior.

## Terminology

- **Target Repository**: the repository where driftline places and synchronizes files from a Source Repository.
- **Target manifest**: `.driftline-target.toml`, the current record of Managed file placements.
- **Managed file**: a file that driftline keeps synchronized from the Source Repository.
- **Sync plan**: the desired set of Target Repository changes derived from current inputs.
- **Conflict**: a Sync plan item that prevents any update writes.

## Design

Add a Target Repository apply Module in the `driftline` package. The command layer calls it after building a Sync plan and after preserving the current conflict-reporting behavior.

Conceptual update flow:

```text
runUpdate
  BuildPlan
  if conflicts: printChanges and return drift
  Apply Sync plan to Target Repository
  printChanges
```

The apply Module owns the write sequence:

1. Defensively reject a Sync plan that contains conflicts.
2. Prepare a Target manifest rewrite only when the Sync plan includes Target manifest additions or removals.
3. Delete stale Managed file targets first.
4. Write added or updated Managed file targets second.
5. Commit the Target manifest rewrite last.

The command layer still performs the conflict check needed for current CLI output. The apply Module repeats that check as a safety invariant of the Target Repository write seam, so direct callers cannot accidentally write a conflicted plan.

The write sequence preserves the important ordering where a stale Managed file such as `dir` can be removed before writing a new Managed file at `dir/file`.

## Safety Guarantees

The Module provides these guarantees:

- If the Sync plan contains any conflict, no Target Repository write is attempted.
- Target manifest commit happens after all Managed file deletes and writes succeed.
- If a Managed file delete or write fails, the Target manifest is not committed.
- The Module does not attempt rollback after a successful file delete or write.
- Reporting remains derived from the Sync plan, not from apply-side output.

The deliberately limited guarantee is: the Target manifest must not move ahead of file operations. Full transactional rollback is out of scope because it would require preserving and restoring file content, symlink state, permissions, directory state, and partial filesystem failures.

## Interface Shape

Keep the apply result minimal.

Expected behavior:

- Success returns `nil`.
- Failure returns an error.
- The Module does not return operation logs.
- The Module does not write stdout or stderr.
- The command layer remains responsible for printing existing Sync plan reports.

The exact Go names can be chosen during implementation, but the Module must represent Target Repository application rather than generic file utilities.

## Existing Behavior To Preserve

- `update` prints the same changes as today.
- A conflict causes `update` to print conflicts and return drift.
- `update --force <group.file>` remains one-time behavior and is not persisted.
- File-only updates do not rewrite an unchanged Target manifest.
- Managed-to-template transitions remove the Target manifest entry and leave the target file untouched.
- Removing a Managed file deletes the target file when it is a file, and leaves target directories untouched.

## Testing Plan

Add or adjust tests so the Target Repository apply Module has its own test surface.

Required cases:

- Applying a conflict plan returns an error before writing files or committing the Target manifest.
- Applying a plan that removes stale `dir` and writes `dir/file` succeeds because deletes happen before writes.
- If a Managed file write fails, the Target manifest is not committed.
- If a plan has only Managed file content changes, the Target manifest is not rewritten.
- `runUpdate` still preserves current CLI output by printing from the Sync plan after apply.

Existing command integration tests should remain useful, but the ordering and Target manifest commit guarantees should be testable through the new Module directly.

## Migration Notes

This is an internal architecture change for a pre-release CLI. No compatibility layer is required.

## Open Questions

None. The design intentionally keeps this change narrow and leaves planning-side ownership and conflict handling for a later deepening pass.
