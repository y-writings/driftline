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
  - id: example
    source_path: templates/example.txt
  - id: local-config
    source_path: templates/config.local
    if_not_exists: true
```

Source Manifest file entries do not define `target_path`; target paths belong to the Target Config.

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
  - id: example
  - id: local-config
    if_not_exists: true
```

When a Target Config file entry omits `target_path`, driftline writes to the same relative path as the Source Manifest `source_path`. Add `target_path` only when the Target Repository wants a different destination path:

```yaml
files:
  - id: example
    target_path: example.txt
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
  - id: example
    target_path: example.txt
```

## GitHub Authentication

Public repositories work without configuration. Set `GITHUB_TOKEN` for private repositories or higher rate limits.
