# driftline

`driftline` copies files from a GitHub Source Repository into a Target Repository and records the consumed Git commit in `driftline-lock.yaml`.

## Install

```sh
go build ./src/cmd/driftline
```

## Source Manifest

The Source Repository owns `.driftline-source.yaml` at its repository root.
Editors that support JSON Schema can validate it with the canonical schema.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/y-writings/driftline/main/schema.json
version: 1
gitignore:
  - .cache/tool
files:
  - id: github-workflow
    paths:
      - .github/workflows/ci.yaml
      - .github/workflows/release.yaml
  - id: local-config
    paths:
      - templates/config.local
    if_not_exists: true
```

Source Manifest file entries define adoption units. Each `id` can expose one or more source-side `paths`; target paths belong to the Target Config.

## Target Config

Create a Target Config from a GitHub repository:

```sh
driftline init y-writings/source-repo
```

This creates `.driftline-target.yaml` in the Target Repository.

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
  - id: local-config
    if_not_exists: true
```

When a Target Config file entry has no `path_overrides`, driftline writes each source path to the same relative path in the Target Repository. Add `path_overrides` only for source paths that need a different target-side path:

```yaml
files:
  - id: github-workflow
    path_overrides:
      - from: .github/workflows/ci.yaml
        to: .github/workflows/project-ci.yaml
```

Use `--ref` with `init` to pin the configured branch, tag, or commit-ish by name:

```sh
driftline init y-writings/source-repo --ref main
```

## Commands

```sh
driftline check
driftline diff
driftline update
driftline prune
```

Use `--target-dir` to operate on another Target Repository path.

## Lock File

`driftline update` writes `driftline-lock.yaml` with the resolved Git commit and managed target files.

```yaml
version: 1
repository: y-writings/source-repo
ref: main
commit: 0123456789abcdef0123456789abcdef01234567
files:
  - id: github-workflow
    target_path: .github/workflows/project-ci.yaml
  - id: github-workflow
    target_path: .github/workflows/release.yaml
```

## GitHub Authentication

Public repositories work without configuration. Set `GITHUB_TOKEN` for private repositories or higher rate limits.
