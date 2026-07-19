# Contract Gitignore Section Design

<!-- markdownlint-disable MD013 -->

## Status

This document is the canonical design for Contract-managed sections in a Target Repository's root `.gitignore`.

It supersedes only the decision in `2026-06-27-toml-managed-template-sync-design.md` that removes root `gitignore` behavior. That document remains canonical for Managed and Template behavior, Contract and Sync manifest fundamentals, planning, and command semantics where this document does not explicitly override it. `2026-07-18-driftline-metadata-layout-design.md` remains canonical for metadata paths and artifact names.

This is a new desired-state feature. It does not restore the append-only root `gitignore` behavior from the obsolete YAML design and must not add compatibility parsing for that design.

## Goal

Allow a Source Repository Contract to declare raw `.gitignore` lines that driftline maintains inside one clearly marked generated section in the Target Repository's root `.gitignore`.

The generated section must:

- identify the Source Repository Contract that produced it,
- state that it is generated and must not be edited,
- replace all content inside its markers during reconciliation,
- preserve all bytes outside its markers,
- remain independent of matching lines outside the generated section.

## Ownership Model

The Gitignore section introduces partial-file ownership:

- driftline owns the complete region from the recognized start marker through the recognized end marker and its line terminator when present,
- the Target Repository owns every byte outside that region,
- matching `.gitignore` lines outside the region do not satisfy or conflict with the Contract,
- edits inside the region are drift and are replaced wholesale,
- no lock file, historical state, or Sync manifest field records this ownership.

The current Contract and a structurally valid marker pair are the only desired-state and ownership evidence.

This responsibility is distinct from both existing file modes:

- a Managed file is wholly source-controlled,
- a Template file becomes wholly target-owned after initial placement,
- a Gitignore section remains source-controlled only within its markers.

## Contract Shape

The Contract remains `version = 2` and gains an optional `[gitignore]` table:

```toml
version = 2

[gitignore]
entries = [
  ".env",
  "/dist/",
  "!/dist/.gitkeep",
]

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
```

Rules:

- `[gitignore]` is optional.
- When `[gitignore]` is present, `entries` is required.
- `entries` must be an array of strings.
- `entries = []` explicitly requests removal of an existing generated section.
- An absent `[gitignore]` table also requests removal of an existing generated section.
- Unknown fields in `[gitignore]` are invalid under the existing strict Contract parsing rule.
- The Sync manifest schema and version do not change.
- The Sync manifest has no Gitignore opt-out, entry override, target path, or historical state.

## Entry Semantics And Validation

Each entry is one raw `.gitignore` line, not a repository path.

Driftline must preserve:

- authored array order,
- duplicate entries,
- empty entries,
- whitespace-only entries,
- leading and trailing whitespace,
- comments,
- negation patterns,
- glob syntax and escaping.

Driftline must not trim, sort, deduplicate, normalize, or semantically validate entries. Git remains responsible for interpreting `.gitignore` syntax. This feature must not add a runtime dependency on a Git command for entry validation.

An entry is invalid when:

- it contains CR or LF and therefore represents more than one line,
- it exactly matches the generated end marker,
- it matches the generated start marker grammar for any syntactically valid `owner/repository`.

The generated warning line is ordinary owned content rather than a structural marker. A Contract entry equal to the warning line is allowed, although it results in a duplicate warning line inside the generated section.

## Generated Format

The generated section has this exact logical form:

```gitignore
# start driftline from y-writings/hogehoge/.driftline/contract.toml
# DO NOT EDIT: this section is managed automatically by driftline.
.env
/dist/
# end driftline
```

The start marker is:

```text
# start driftline from <owner/repository>/.driftline/contract.toml
```

The `<owner/repository>` value comes from the current Sync manifest. The marker deliberately omits the ref and resolved commit so routine ref resolution does not create provenance-only drift.

The warning and end marker are fixed:

```text
# DO NOT EDIT: this section is managed automatically by driftline.
# end driftline
```

The generated section consists of:

1. the start marker,
2. the warning line,
3. every Contract entry in authored order,
4. the end marker,
5. a final line terminator.

There is no extra blank line between the warning and the first entry or between the final entry and the end marker unless the Contract explicitly includes an empty entry at that position.

## Marker Recognition

Marker recognition is line-oriented and exact after excluding the line terminator.

At the byte level, LF delimits lines. A CR immediately before LF is part of a CRLF delimiter. A lone CR is line content rather than a delimiter. Marker comparison excludes LF and its immediately preceding CR. A final line without LF is also eligible for marker comparison, including every byte through end of file.

A start marker is recognized only when the entire line consists of the fixed prefix, a repository accepted by the existing `owner/repository` validation, and the fixed `/.driftline/contract.toml` suffix. An end marker is recognized only when the entire line is `# end driftline`.

Recognition must not:

- trim indentation,
- trim trailing whitespace,
- ignore case,
- match marker substrings inside another line,
- require the repository in the start marker to equal the current Sync source.

Recognizing any valid repository lets a source change update the existing section's provenance instead of leaving a stale section and appending another one.

The warning line and all other non-marker content between the markers are not validated. Reconciliation replaces them wholesale. A recognized start or end marker inserted into the owned content makes the structure invalid rather than repairable.

A `.gitignore` has a valid driftline structure when it contains either:

- no recognized start or end markers, or
- exactly one start marker followed by exactly one end marker.

The following are structural errors:

- a start marker without an end marker,
- an end marker without a start marker,
- an end marker before the start marker,
- nested start markers,
- multiple complete sections,
- any other count or ordering containing more than one recognized start or end marker.

A structural error fails planning without modifying `.gitignore`, Managed files, or the Sync manifest. It is not a Managed conflict, and `--force` cannot bypass it. Non-matching marker-like comments remain target-owned ordinary lines.

## Desired-State Transformation

The transformation operates on bytes so content outside the owned section can be preserved even when it is not valid UTF-8. Contract strings are encoded as UTF-8 when constructing generated lines.

### Non-Empty Entries

When `[gitignore].entries` is non-empty:

- If `.gitignore` is missing, create it with only the generated section.
- If `.gitignore` contains no markers, append the generated section at the end.
- If `.gitignore` contains one valid section, replace that section in place.
- If the existing desired bytes already match, produce no change and do not rewrite the file.

When appending to a non-empty file, preserve its existing bytes and add only the line terminators needed to ensure at least one empty separator line before the start marker. An empty separator line contains zero bytes between line delimiters; a whitespace-only line does not count. Do not remove existing trailing blank lines. An empty file receives no leading blank line.

### Empty Or Absent Configuration

When `[gitignore]` is absent or `entries = []`:

- If a regular `.gitignore` contains one valid section, remove the complete section from the start marker through the end marker and its line terminator when present.
- Preserve every byte outside that span.
- Preserve the separator blank line before the section because it is outside the markers.
- Keep `.gitignore` even when section removal leaves a zero-byte file.
- If a regular `.gitignore` contains no recognized markers, produce no change.
- If `.gitignore` is missing, produce no change.
- If `.gitignore` is a non-regular path, do not follow or modify it and produce no change.

## Line Endings

Driftline preserves all line endings outside the generated section.

Generated lines use:

- the start marker's line ending when replacing an existing section,
- the first line ending found in the file when appending a new section,
- LF when the file has no line ending or is newly created.

This rule applies even when the file mixes LF and CRLF. Driftline does not normalize mixed line endings. Every generated section ends with its selected line ending.

## Managed And Template Interaction

A Contract may declare both `[gitignore]` and a Template file whose source path is `.gitignore`.

- `init` places the Template under existing Template rules when `.gitignore` is missing.
- `init` does not add the generated section.
- Later `check`, `diff`, and `update` preserve the template-created content outside the generated section.
- Later Template changes remain ignored under existing Template semantics.

While a `[gitignore]` table is present, including when `entries = []`, no desired Managed file may resolve to the root `.gitignore` target. A stale Sync manifest entry scheduled for removal is not a desired Managed file and does not trigger this prohibition.

- A Managed Contract entry whose default target is `.gitignore` is a Contract validation error.
- A Sync manifest override that maps any Managed File key to `.gitignore` is a planning error.
- These errors are invalid ownership configurations, not forceable target conflicts.

When `entries` is non-empty, `.gitignore` must be a file path, so no Managed or Template source or resolved target may equal a descendant path such as `.gitignore/rules`. These parent-child path-shape collisions are validation or planning errors before any write. The explicitly allowed Template coexistence applies to the exact `.gitignore` path only.

### Gitignore Section To Managed

When a Contract removes `[gitignore]` and makes `.gitignore` a Managed target in the same reconciliation, Managed whole-file ownership takes precedence. The planner does not create a separate generated-section removal change.

Existing Managed behavior applies:

- a newly Managed `.gitignore` that already exists is a conflict,
- `--force <group.file>` may perform the existing one-time overwrite,
- a Managed entry already mapped to `.gitignore` updates the complete file normally,
- the resulting Managed source bytes replace any old generated section.

### Managed To Gitignore Section

When a currently Managed `.gitignore` is removed from the Contract and a non-empty `[gitignore]` is added in the same reconciliation, preserve existing Managed removal semantics:

1. delete the former Managed target,
2. create a new `.gitignore` containing only the generated section.

The former Managed content does not become target-owned implicitly.

When a currently Managed `.gitignore` changes to Template while a non-empty `[gitignore]` is added, existing Managed-to-Template semantics leave the file in place. The generated section is then added while preserving that content outside the markers.

The complete transition precedence is:

| Previous ownership   | Current declaration                                                | Result                                                                                                                   |
| -------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| Gitignore section    | desired Managed `.gitignore` and no `[gitignore]` table            | Apply Managed whole-file behavior only.                                                                                  |
| Managed `.gitignore` | removed Managed entry plus non-empty `[gitignore]`                 | Delete the former Managed file, then create a generated-only `.gitignore`.                                               |
| Managed `.gitignore` | removed Managed entry plus absent or empty Gitignore configuration | Delete the former Managed file only.                                                                                     |
| Managed `.gitignore` | Template `.gitignore` plus non-empty `[gitignore]`                 | Leave the file under Managed-to-Template semantics, then add the section.                                                |
| Managed `.gitignore` | Template `.gitignore` plus absent or empty Gitignore configuration | Leave the file untouched under Managed-to-Template semantics; do not scan it for section removal during this transition. |

The planner resolves this transition before generic section removal so one reconciliation never schedules contradictory writes for `.gitignore`.

## Command Semantics

### init

`init` parses and validates `[gitignore]`, including entry validation, direct path-shape collisions, and the prohibition on simultaneous Managed ownership. It does not inspect `.gitignore` marker structure or filesystem type and does not create, update, or remove a generated section.

Template `.gitignore` placement continues to run under existing Initial adoption rules.

### check

`check` includes a Gitignore section change in the same plan used by `update`. A non-nil change is drift.

Expected output forms are:

```text
add gitignore: generated section is missing
update gitignore: generated section differs
remove gitignore: generated section is no longer declared
```

The exact `add` reason also applies when the complete `.gitignore` file is missing. `remove` refers to the generated section, not deletion of the file.

### diff

`diff` renders a unified diff between the current complete `.gitignore` bytes and the complete desired bytes. The unchanged target-owned content appears only as ordinary diff context. A missing `.gitignore` is diffed as a new file.

Structural, validation, and filesystem inspection errors fail `diff` instead of emitting a partial generated-section diff. Apply-time stale-state validation does not run during `diff`.

The existing byte-oriented `git diff --no-index` path renders the complete current and desired files. Driftline does not decode, replace, or discard invalid UTF-8 bytes. If Git classifies the content as binary, its normal binary-difference output is used and the Gitignore change still counts as drift.

### update

`update` applies the Gitignore section change from the shared plan. It does not independently recalculate marker contents in the command layer.

An invalid marker structure fails before any apply mutation. The Gitignore section has no force behavior and never uses the Managed conflict instructions.

## Planning Architecture

The Contract model gains a dedicated optional Gitignore configuration type. The plan gains a dedicated `GitIgnoreSectionChange` rather than representing the section as a synthetic Managed file.

`GitIgnoreSectionChange` carries the information needed for reporting, diffing, stale validation, and application:

- `add`, `update`, or `remove` status,
- reason,
- fixed `.gitignore` target path,
- whether the target was missing at plan time,
- expected original bytes when the target was regular,
- complete desired bytes.

Marker parsing and desired-byte transformation must be pure logic separated from filesystem inspection and writing. The planner is responsible for:

1. validating Managed and Template exact-path and parent-child coexistence,
2. inspecting the root `.gitignore` without following symlinks,
3. parsing marker structure,
4. deriving complete desired bytes,
5. returning no change when bytes already match,
6. attaching a dedicated change to the complete plan.

`HasDrift` and command reporting must consider the dedicated change alongside existing Managed changes. The dedicated type must not expose Managed source paths, force flags, or Managed conflict semantics.

## Filesystem Safety And Apply Ordering

For a non-empty Gitignore configuration, the target may be missing or a regular file. A symlink, broken symlink, directory, or other non-regular path is an unsupported target error. Driftline must not follow or replace it.

For an absent or empty Gitignore configuration, a missing or non-regular `.gitignore` is left untouched. A regular file is read so a valid generated section can be removed; an unreadable regular file is an error because driftline cannot determine whether removal is required.

When the plan contains a non-nil `GitIgnoreSectionChange`, apply must revalidate the Gitignore target against its plan-time state before any plan mutation:

- expected missing but now present is stale,
- expected regular but now missing or non-regular is stale,
- expected regular with different bytes is stale.

A stale target aborts apply before Managed files or the Sync manifest are changed. The user reruns the command to build a plan from current bytes. This is stale-state protection, not automatic merging or a forceable conflict. When the desired Gitignore bytes already match and the plan therefore contains no Gitignore change, Gitignore stale-state validation does not run and unrelated Managed reconciliation is not coupled to later target-owned edits.

When a write is required:

- prepare a temporary file in the Target Repository root,
- write the complete desired bytes,
- preserve the existing file permission bits when replacing a regular file,
- use `0644` subject to the process umask when creating a new file,
- atomically rename the prepared file to `.gitignore`,
- remove the temporary file on failure.

The apply sequence extends the existing commit-last design:

1. reject conflict or invalid plans,
2. prepare any required Sync manifest rewrite,
3. revalidate the Gitignore target and prepare its temporary file,
4. delete stale Managed targets,
5. write Managed additions and updates,
6. commit the prepared Gitignore replacement,
7. commit the Sync manifest last.

This ordering permits the Managed-to-Gitignore transition to delete the former Managed file before installing the generated-only replacement. A Gitignore write failure prevents the Sync manifest commit. Existing no-rollback behavior remains: file mutations completed before a later error are not reverted.

Atomic replacement prevents partial-file writes. Stale revalidation narrows, but does not claim to eliminate, the filesystem race between the final comparison and rename.

## Error Categories

The feature introduces no new forceable conflict class.

Errors are classified as:

- Contract validation errors for missing `entries`, invalid entry values, unknown fields, or direct Managed coexistence,
- planning errors for Sync manifest target coexistence, malformed marker structure, unreadable regular files, or unsupported target types when a non-empty section is required,
- stale-state errors when apply-time target state differs from the plan,
- filesystem errors while preparing or committing the atomic replacement.

Diagnostics must identify `.gitignore` and the specific cause. Marker errors should identify the observed structural problem and instruct the user to repair the markers manually. They must not suggest `--force`.

## Testing Requirements

Contract tests must cover:

- parsing valid `[gitignore]` entries,
- preserving version 2,
- requiring `entries` when the table exists,
- accepting an explicit empty array,
- rejecting unknown fields,
- rejecting CR and LF inside an entry,
- rejecting recognized start and end marker entries,
- preserving all other raw entry strings,
- allowing Template `.gitignore` coexistence,
- rejecting direct Managed `.gitignore` coexistence.

Pure transformation tests must cover:

- creating a generated-only file,
- appending after target-owned content,
- replacing a block in place,
- removing a block while retaining the file,
- retaining the separator blank line after removal,
- ignoring duplicate lines outside the block,
- preserving entry order, duplicates, comments, empty lines, and whitespace,
- updating provenance after a Source Repository change,
- repairing edited warning or entry content that does not introduce a recognized marker,
- producing no change for matching bytes,
- LF, CRLF, and mixed line endings,
- a block without a final line ending,
- malformed, reversed, nested, and duplicate markers,
- non-matching marker-like comments.

Planner and apply tests must cover:

- missing and regular targets,
- symlinks, broken symlinks, directories, and other non-regular targets,
- absent or empty configuration leaving non-regular targets untouched,
- unreadable regular targets,
- Managed target overrides to `.gitignore`,
- Template coexistence,
- Gitignore-to-Managed ownership transfer,
- Managed-to-Gitignore deletion and generated-only recreation,
- Managed-to-Template plus Gitignore section placement,
- stale content,
- missing-to-created and regular-to-missing races,
- permission preservation,
- atomic write preparation and commit failures,
- preventing Sync manifest commit after a Gitignore write failure,
- preserving existing no-rollback semantics.

Command integration tests must cover:

- `init` validating but not applying the section,
- `init` retaining normal Template `.gitignore` placement,
- `check` add, update, remove, and synced output,
- `diff` full-file unified diffs,
- `update` section creation, replacement, and removal,
- malformed markers preventing all writes,
- `--force` not bypassing Gitignore section errors.

## Documentation Changes

Current documentation and examples must:

- document `[gitignore].entries`,
- explain marker-scoped ownership,
- show the exact generated format,
- state that `init` validates but does not apply the section,
- distinguish the feature from Managed and Template files,
- remove or supersede guidance that `.gitignore` must always be modeled as a normal Managed or Template file,
- avoid presenting obsolete append-only YAML behavior as current or compatible.

## Out Of Scope

- Applying the generated section during `init`.
- Target-side opt-out or entry overrides.
- A configurable target path other than the repository-root `.gitignore`.
- Multiple generated sections or multiple Source Repositories.
- Automatic repair of malformed or duplicate markers.
- Force overwrite for Gitignore section errors.
- Semantic validation of Git ignore patterns.
- Sorting or deduplicating entries.
- A generic partial-file operation framework.
- Legacy YAML parsing or migration behavior.
- Historical ownership state or a new metadata file.
