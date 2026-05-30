# driftline

`driftline` copies files from a GitHub Source Repository into a Target Repository and records the consumed Git commit in `driftline-lock.yaml`.

## Install

```sh
go build ./src/cmd/driftline
```

## Source Manifest

The Source Repository owns `driftline.yaml` at its repository root.

```yaml
version: 1
gitignore:
  - .cache/tool
files:
  - id: example
    source: templates/example.txt
    target: example.txt
  - id: local-config
    source: templates/config.local
    target: config.local
    if_not_exists: true
```

## Target Config

Create a Target Config from a GitHub repository:

```sh
driftline init y-writings/source-repo
```

This creates `driftline.yaml` in the Target Repository.

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: example
    target: example.txt
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

`driftline update` writes `driftline-lock.yaml` with the resolved Git commit and file hashes.

```yaml
version: 1
repository: y-writings/source-repo
ref: main
commit: 0123456789abcdef0123456789abcdef01234567
files:
  - id: example
    target: example.txt
    source_sha256: ...
    target_sha256: ...
```

## GitHub Authentication

Public repositories work without configuration. Set `GITHUB_TOKEN` for private repositories or higher rate limits.
