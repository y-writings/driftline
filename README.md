# driftline

`driftline` synchronizes files from a GitHub Source Repository into a Target Repository using small TOML manifests.

The Source Repository defines file identity and mode. The Target Repository defines placement for managed files.

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

## Source Config

The Source Repository owns `.driftline-source.toml` at its repository root.

```toml
version = 2

[files.github-workflow]
ci = { path = ".github/workflows/ci.yaml", mode = "managed" }
release = { path = ".github/workflows/release.yaml", mode = "template" }

[files.mise]
config = { path = ".mise/config.toml", mode = "template" }
```

Source config rules:

- `version = 2` is required.
- `[files.<group>]` groups related files.
- Each file is keyed by stable file ID inside its group.
- Each file entry has `path` and `mode`.
- `mode` is `managed` or `template`.

## Target Config

Create a Target Config from a GitHub Source Repository:

```sh
driftline init y-writings/source-repo
```

This creates `.driftline-target.toml` in the Target Repository.

```toml
version = 2

[source]
repository = "y-writings/source-repo"
ref = "main"

[files.github-workflow]
ci = ".github/workflows/project-ci.yaml"
```

Target config rules:

- `version = 2` is required.
- `[source]` contains `repository` and `ref`.
- `[files.<group>]` contains managed files only.
- Each file value is the target repository path for that managed file.
- Template files are not recorded after initial placement.

Use `--ref` with `init` to preserve a branch, tag, or commit-ish by name:

```sh
driftline init y-writings/source-repo --ref main
```

## File Modes

Managed files stay synchronized with the source. `driftline update` adds missing managed entries to `.driftline-target.toml`, updates changed files, removes entries that are no longer managed, and deletes files that were removed from the managed source set.

Template files are initial placement aids. `driftline init` writes a template file only when the target path is missing. Later updates do not record, update, or delete template files.

## Commands

```sh
driftline check
driftline diff
driftline update
```

Use `--target-dir` to operate on another Target Repository path.

If a newly managed file would overwrite an existing target-owned file, driftline reports a conflict and does not write files or config. To overwrite one file once, pass its file key:

```sh
driftline update --force github-workflow.ci
```

Force is not persisted in `.driftline-target.toml`.

## GitHub Authentication

Public repositories work without configuration. Set `GITHUB_TOKEN` for private repositories or higher rate limits.
