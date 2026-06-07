# File Set Path Overrides Design

## Goal

Make driftline adopt a logical file set with one `id`, so users can copy related files such as GitHub workflows or mise configs by selecting one adoption unit.

## Context

- Current Source Manifest entries model one `id` as one `source_path`.
- Related files often form one product decision: for example, `github-workflow` may include multiple files under `.github/workflows/`.
- driftline is pre-release, so the clearer model should replace the current shape directly instead of preserving `source_path` compatibility.
- The desired model treats `id` as an adoption unit, not as a single file identifier.

## Chosen Contract

Source Manifest entries use `paths` for all source-side files in an adoption unit:

```yaml
version: 1
files:
  - id: github-workflow
    paths:
      - .github/workflows/ci.yaml
      - .github/workflows/release.yaml

  - id: mise
    paths:
      - .mise/config.toml
      - .mise/config.baseline.toml
```

Target Config entries adopt the unit by `id`. By default, each source path is copied to the same target-side relative path:

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
  - id: mise
```

Target Config entries can override individual target paths with `path_overrides`:

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
    path_overrides:
      - from: .github/workflows/ci.yaml
        to: .github/workflows/project-ci.yaml
```

## Terminology

- Adoption unit: one Source Manifest `files[]` entry identified by `id`.
- Source path: one path listed in Source Manifest `paths[]`.
- Target path: the effective target-side path after applying any `path_overrides` entry.
- Path override: one Target Config rule mapping a source path `from` to a target path `to` for the same adopted `id`.

## File Formats

Source Repository `.driftline-source.yaml`:

```yaml
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

Target Repository `.driftline-target.yaml`:

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: github-workflow
    path_overrides:
      - from: .github/workflows/ci.yaml
        to: .github/workflows/project-ci.yaml
  - id: local-config
    if_not_exists: true
```

Lock file `driftline-lock.yaml` stays file-oriented because lock items represent concrete managed target files:

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

## Runtime Behavior

- `init` reads Source Manifest adoption units and writes one Target Config entry per `id`.
- `check`, `diff`, `update`, and `prune` expand each adopted `id` into one planned file per Source Manifest `paths[]` entry.
- If a source path has a matching `path_overrides[].from`, driftline writes it to that override's `to` path.
- If a source path has no override, driftline writes it to the same relative path in the target repository.
- Resolved source and target paths use normalized slash-separated relative paths, so `./foo.txt` and `foo.txt` resolve to the same source and target path.
- `if_not_exists` applies to the whole adoption unit. Source Manifest sets the default; Target Config can override it to either `true` or `false` for every expanded file for that `id`.
- `path_overrides` only moves files. It does not add files, exclude files, glob files, or rename files by template.
- A source path change inside a Source Manifest adoption unit behaves like the current default target change: the new target becomes active and the old locked target remains a prune candidate unless another active file uses that target path.

## Validation

- Source Manifest root keys remain `version`, `gitignore`, and `files`.
- Source Manifest file entries allow `id`, `paths`, and `if_not_exists`.
- Source Manifest file entries require `id` and `paths`.
- `paths` must be a non-empty sequence.
- Each `paths[]` item must pass existing relative path validation.
- A Source Manifest adoption unit must not contain duplicate normalized source paths.
- Source Manifest `id` values must remain unique.
- Target Config file entries allow `id`, `path_overrides`, and `if_not_exists`.
- Target Config file entries require `id`; `path_overrides` is optional.
- Target Config `id` values must remain unique.
- If `path_overrides` is present, it must be a non-empty sequence.
- Each `path_overrides[]` item requires `from` and `to`.
- Each `from` and `to` must pass existing relative path validation.
- Each `path_overrides[].from` must match one normalized source path in the adopted Source Manifest unit.
- A Target Config file entry must not contain duplicate normalized `path_overrides[].from` values.
- After expanding all adopted units and applying overrides, target paths must remain unique across the whole plan.
- Reserved target path checks apply after expansion and override resolution.

## Schema And Types

- Replace Source Manifest `SourceManifestFile.SourcePath string` with `Paths []string`.
- Replace Target Config `TargetConfigFile.TargetPath string` with `PathOverrides []PathOverride`.
- Add `PathOverride` with `From string` and `To string` YAML fields.
- Update `schema.json` to describe `paths` and reject `source_path`.
- Keep lock item shape as `id` plus `target_path`, allowing multiple lock entries with the same `id` when target paths differ.

## Planning Model

Plan building should normalize Source Manifest adoption units into concrete resolved files before reading source bytes. Each resolved file contains:

- `id`: adopted unit id.
- `source`: normalized source repository path from `paths[]`.
- `target`: source path or override `to` path.
- `ifNotExists`: effective unit policy.

The rest of the plan can stay file-oriented: source reads, target hashing, changes, lock writing, and prune checks operate on resolved files.

## Errors

- Unknown Target Config `id`: keep the existing error shape `unknown source file id "<id>"`, even though `id` now means adoption unit.
- Unknown override source path: return an error naming the `id` and the unmatched `from` path.
- Duplicate override `from`: return an error naming the `id` and duplicate `from` path.
- Duplicate target after expansion: keep the existing duplicate target error shape.
- Missing source file during read: keep the existing source file not found error shape.

## Tests And Docs

- Update config tests to accept Source Manifest `paths` and reject `source_path`.
- Add Source Manifest validation tests for empty `paths`, duplicate paths, and invalid paths.
- Update Target Config tests to accept `path_overrides` and reject `target_path`.
- Add Target Config validation tests for invalid overrides, duplicate `from`, and unknown `from` during planning.
- Update plan tests so one adopted `id` expands to multiple adds, updates, lock entries, and prune decisions.
- Add plan tests for default same-path targets and per-path overrides in the same adoption unit.
- Update command tests so `init` writes one Target Config entry per adoption unit without copying per-path target configuration.
- Update README examples and prose to explain adoption units, `paths`, and `path_overrides`.

## Verification

- Run `go test ./src/internal/driftline/...` during implementation.
- Run `go test ./...` before finishing.
- Review docs, schema, tests, and source for stale current examples of `source_path` and `target_path` in Target Config.
- Keep `target_path` in lock file documentation and tests, because lock entries remain concrete target-file records.

## Out Of Scope

- Compatibility parsing for Source Manifest `source_path`.
- Compatibility parsing for Target Config `target_path`.
- Per-file `if_not_exists` within an adoption unit.
- Excluding a subset of Source Manifest `paths[]` from Target Config.
- Adding extra Target Config paths not declared by Source Manifest.
- Glob expansion.
- Directory-level or template-based renames.
- Changing command names or lock file name.
