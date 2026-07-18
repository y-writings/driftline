# Driftline Repository Metadata Layout Design

<!-- markdownlint-disable MD013 -->

## Status

This document is the canonical design for driftline repository metadata paths and artifact names.

It supersedes the path and artifact naming decisions in `2026-06-27-toml-managed-template-sync-design.md`. That document remains canonical for Managed/Template behavior, configuration shape, planning, and command semantics where it does not conflict with this document.

This is an intentionally breaking change. The old root-level paths are not compatibility requirements.

## Goal

Move driftline metadata out of the repository root, reserve room for future metadata, and name the two current artifacts after their distinct responsibilities.

## Decision

Driftline metadata lives under a repository-root `.driftline/` directory:

```text
.driftline/
|-- contract.toml
`-- sync.toml
```

- `.driftline/contract.toml` is the repository's outbound Contract.
- `.driftline/sync.toml` is the repository's inbound Sync manifest.
- Either file may exist without the other.
- A repository that provides files to one repository and synchronizes files from another may contain both files.

The names are singular because the current model has one Contract per providing repository and one inbound relationship to one provider and ref per Sync manifest.

## Contract

`.driftline/contract.toml` replaces `.driftline-source.toml`.

The Contract declares:

- Group IDs and File IDs,
- local source paths,
- `managed` or `template` mode for each file.

The repository authors the Contract and driftline reads it. Driftline does not rewrite it as part of Target Repository synchronization.

"Contract" means the file interface exposed by the selected repository ref. It does not promise compatibility between refs, semantic versioning, package publication, or a bilateral agreement.

The Contract uses the Contract TOML shape defined by the Managed/Template sync design. This layout change does not independently change that schema or its version.

## Sync Manifest

`.driftline/sync.toml` replaces `.driftline-target.toml`.

The Sync manifest declares:

- one providing repository and ref,
- local target paths for currently Managed files.

It is human-editable and driftline-updatable. Driftline may rewrite it when the current Managed set changes.

The Sync manifest is current desired mapping, not history, generated lock state, or an import receipt. It does not retain Template files after initial placement because those files become target-owned.

"Sync" means explicit, one-way reconciliation from the providing repository to the local repository. It does not mean bidirectional synchronization, background synchronization, or a new `sync` command.

The Sync manifest uses the Sync manifest TOML shape defined by the Managed/Template sync design. This layout change does not independently change that schema or its version.

## Dual-Role Repositories

A repository may contain both artifacts:

```text
.driftline/
|-- contract.toml
`-- sync.toml
```

The artifacts describe independent relationships:

- `contract.toml` declares files this repository provides outward.
- `sync.toml` declares the Managed relationship this repository receives inward.

Their coexistence does not create self-sync, circular sync, or bidirectional sync. Commands acting on the Sync manifest must not rewrite the Contract.

## Reserved Namespace

The complete `.driftline/` subtree is reserved for driftline metadata.

- A Managed or Template source path must not equal `.driftline` or start with `.driftline/`.
- A Managed or Template target path must not equal `.driftline` or start with `.driftline/`.
- Driftline must reject these paths during configuration validation, before planning writes.
- The rejection must identify the exact path and explain that `.driftline/` is reserved metadata.

When driftline needs to create `.driftline/sync.toml`, it creates `.driftline/` if the directory is absent. It must fail without writing when `.driftline` is not a real directory or when the Sync manifest path is a symlink or another unsupported file type.

The reserved subtree is the stable extension point for future driftline metadata. This design does not assign names or behavior to future files.

## Command Integration

- `driftline init <owner/repo>` reads `.driftline/contract.toml` from the providing repository.
- `init` creates `.driftline/` when needed and writes `.driftline/sync.toml` in the local repository.
- `check`, `diff`, and `update` read `.driftline/sync.toml`.
- `update` may rewrite `.driftline/sync.toml` under the existing Sync manifest rules.
- No command infers a role from the presence of only one artifact when the other exact path is required.

Diagnostics must use the exact path and artifact role when either could be ambiguous. Preferred forms include:

```text
Contract not found: .driftline/contract.toml
Sync manifest already exists: .driftline/sync.toml
reserved driftline metadata path: .driftline/example.toml
```

Documentation may continue to use Source Repository and Target Repository for the endpoints of a synchronization relationship. It must call the artifacts the Contract and Sync manifest rather than using endpoint-based artifact names.

## Migration Policy

This repository is pre-release. Implement the new layout directly.

- Do not read `.driftline-source.toml`.
- Do not read or write `.driftline-target.toml`.
- Do not add fallback lookup, deprecated aliases, dual-path writes, migration commands, or compatibility warnings for the old paths.
- Old root-level files are unrelated files: their presence neither satisfies a required new path nor changes command behavior.
- Rewrite implementation constants, messages, tests, fixtures, examples, and current documentation to the new paths and artifact names.

## Rationale

The artifacts have intentionally asymmetric responsibilities:

- the Contract declares both Managed and Template files,
- the Sync manifest records only the ongoing Managed relationship.

`source.toml` and `target.toml` name relationship endpoints but obscure that asymmetry. In a dual-role repository, `source.toml` can also be mistaken for the place to edit the provider and ref stored in the inbound manifest.

`export.toml` and `import.toml` make local direction clear, but `import` commonly suggests a one-time transfer followed by local ownership. That implication conflicts with Managed files, which driftline may later update or delete. Both words also imply operations more strongly than persistent declarations.

`contract.toml` and `sync.toml` name the persisted responsibilities. Their risks are controlled by defining Contract as ref-scoped rather than compatibility-guaranteeing, and Sync as explicit and one-way rather than bidirectional or background behavior.

## Testing Requirements

Tests must cover:

- parsing the Contract from `.driftline/contract.toml`,
- creating `.driftline/` and `.driftline/sync.toml` during `init`,
- reading and rewriting `.driftline/sync.toml` during Target Repository commands,
- allowing both artifacts in one repository without rewriting the Contract,
- rejecting Managed and Template source paths under `.driftline/`,
- rejecting Managed and Template target paths under `.driftline/`,
- failing safely when `.driftline` is not a real directory or the Sync manifest path is a symlink or unsupported file type,
- verifying that old root-level files are not read and do not satisfy either required new path,
- using the new paths and artifact terms in command help, success output, conflicts, and errors.

## Out of Scope

- Multiple provider relationships in one Sync manifest.
- Bidirectional or background synchronization.
- A `sync` command.
- Automatic migration from the old root-level files.
- Defining additional files under `.driftline/`.
