# driftline

<!-- markdownlint-disable MD013 -->

`driftline` synchronizes files from a GitHub Source Repository into a Target Repository using small TOML metadata files.

The Source Repository's Contract defines file identity, source paths, and mode. The Target Repository's Sync manifest defines placement for Managed files.

## Install

Run with Nix:

```sh
nix run github:y-writings/driftline
```

Build with Nix:

```sh
nix build github:y-writings/driftline#driftline
```

Install into a Nix profile:

```sh
nix profile install github:y-writings/driftline#driftline
```

Build from source with Go:

```sh
go build ./src/cmd/driftline
```

## Contract

The Source Repository owns `.driftline/contract.toml`. The Contract declares stable file identities, source paths, whether each file uses Managed or Template mode, and optional root `.gitignore` entries.

```toml
version = 2

[gitignore]
entries = [
  ".env",
  "/dist/",
]

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
```

Contract rules:

- The Contract is parsed as TOML 1.1.
- `version = 2` is required.
- `[files.<group>]` groups related files.
- Each file is keyed by stable file ID inside its group.
- Each file entry has `path` and `mode`.
- `mode` is `managed` or `template`.

## Sync Manifest

Create a Sync manifest from a GitHub Source Repository:

```sh
driftline init y-writings/source-repo
```

This creates `.driftline/sync.toml` in the Target Repository. The Sync manifest records the provider repository and ref plus local target paths for currently Managed files only.

```toml
version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
```

Sync manifest rules:

- The Sync manifest is parsed as TOML 1.1.
- `version = 2` is required.
- `[source]` contains `repository` and `ref`.
- `[files.<group>]` contains Managed files only.
- Each file value is the Target Repository path for that Managed file.
- Template files are not recorded after initial placement.

A repository may contain both metadata files:

```text
.driftline/
|-- contract.toml
`-- sync.toml
```

They are independent. The Contract declares files the repository provides outward, while the Sync manifest records files it receives inward. Commands that use the Sync manifest do not rewrite the Contract.

The complete `.driftline/` subtree is reserved for driftline metadata. A Managed or Template source or target path cannot equal `.driftline` or be inside `.driftline/`.

Use `--ref` with `init` to preserve a branch, tag, or commit-ish by name:

```sh
driftline init y-writings/source-repo --ref main
```

Use `--force` with `init` to adopt existing regular files at Managed file target paths into the Sync manifest:

```sh
driftline init y-writings/source-repo --force
```

`init --force` does not overwrite file content. It records those paths in the Sync manifest; later `check` reports drift and `update` synchronizes the now-Managed files if their content differs from the Source Repository.

## File Modes

Managed files stay synchronized with the source. `driftline update` adds missing Managed entries to the Sync manifest, updates changed files, removes entries that are no longer Managed, and deletes target files only when their Managed file entry is removed from the Contract. If a file changes from Managed to Template, `update` removes the Sync manifest entry and leaves the target file untouched.

Template files are initial placement aids. `driftline init` writes a template file only when the target path is missing. Later updates do not record, update, or delete template files.

## Gitignore Section

The optional Contract `[gitignore]` table gives the Source Repository ownership of one marker-delimited region in the Target Repository's root `.gitignore`. The example Contract generates this exact block:

```gitignore
# start driftline from y-writings/source-repo/.driftline/contract.toml
# DO NOT EDIT: this section is managed automatically by driftline.
.env
/dist/
# end driftline
```

The start marker records only the Source Repository that provided the Contract, not its ref or resolved commit. Each `entries` value is a raw line: driftline preserves authored order, duplicates, empty lines, and whitespace. Matching or duplicate lines outside the block remain target-owned and are ignored. Reconciliation replaces the complete marked region while preserving every byte outside it.

An absent `[gitignore]` table or `entries = []` removes the generated block but keeps `.gitignore` and all bytes outside the markers. Malformed, out-of-order, or multiple recognized markers are errors that require manual repair and are not bypassed by `--force`.

`init` validates `[gitignore]` but does not inspect existing markers or apply the generated block. A Template file whose path is exactly `.gitignore` may coexist and seed a missing target during `init`; `check`, `diff`, and `update` later reconcile the generated region. A Managed file cannot resolve to root `.gitignore` while `[gitignore]` is present. The Sync manifest provides no target-side opt-out, target-path override, or entry override for this region.

Packaged builds support Linux and Darwin, where Gitignore section updates use atomic replacement. Windows supports Contract parsing, `check`, and `diff` through safe no-follow reads, but a Gitignore section `update` fails before mutation because driftline has no documented atomic replacement primitive there. Other unsupported platforms may reject Gitignore planning or apply depending on safe-read and atomic-replacement support.

## Commands

```sh
driftline check
driftline diff
driftline update
```

Use `--target-dir` to operate on another Target Repository path.

If a newly Managed file would overwrite an existing target-owned file, driftline reports a conflict and does not write files or the Sync manifest. To overwrite one file once, pass its File key:

```sh
driftline update --force github-workflow.ci
```

Force is not persisted in the Sync manifest.

## GitHub Authentication

Public repositories work without configuration. Set `GITHUB_TOKEN` for private repositories or higher rate limits.
