# Source Target Config Naming Design

## Goal

Make driftline configuration names distinguish Source Repository manifests from Target Repository configs at the file-name and path-field levels.

## Context

- Source Repository and Target Repository configuration currently both use `driftline.yaml`, which makes it unclear which side owns a file.
- Source Manifest file entries currently use `source`, while Target Config file entries use `target`; these short names are easy to confuse when reading examples or tests together.
- Recent work already separated Source Manifest responsibility from Target Config responsibility by removing target paths from Source Manifest entries.
- The project is pre-release, so this change should replace the old names directly instead of adding compatibility readers or migration shims.

## Chosen Approach

Use symmetric dotfile names and explicit path field names:

- Source Repository manifest: `.driftline-source.yaml`
- Target Repository config: `.driftline-target.yaml`
- Lock file: `driftline-lock.yaml`
- Source Manifest file path field: `source_path`
- Target Config file path field: `target_path`
- Lock file path field: `target_path`

Do not support the old `driftline.yaml`, `source`, or `target` names after the rename.

## File Formats

Source Repository `.driftline-source.yaml`:

```yaml
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

Target Repository `.driftline-target.yaml`:

```yaml
version: 1
source:
  repository: y-writings/source-repo
  ref: main
files:
  - id: example
  - id: local-config
    if_not_exists: true
  - id: explicit-place
    target_path: config/example.txt
```

Lock file `driftline-lock.yaml`:

```yaml
version: 1
repository: y-writings/source-repo
ref: main
commit: 0123456789abcdef0123456789abcdef01234567
files:
  - id: example
    target_path: example.txt
```

## Runtime Behavior

- `init` reads `.driftline-source.yaml` from the Source Repository and writes `.driftline-target.yaml` into the Target Repository.
- `check`, `diff`, `update`, and `prune` read `.driftline-target.yaml` from the Target Repository.
- If a Target Config entry omits `target_path`, driftline writes to the same relative path as the Source Manifest entry's `source_path`.
- `target_path` overrides the default target-side path when present.
- `if_not_exists` keeps its current behavior and naming.
- `source.repository` and `source.ref` stay unchanged because they describe the source repository reference, not file paths.
- `--target-dir` stays unchanged because it identifies the target repository directory, not an individual target file path.

## Validation

- Source Manifest validation allows root keys `version`, `gitignore`, and `files`; file entries allow `id`, `source_path`, and `if_not_exists`.
- Source Manifest file entries require `id` and `source_path`.
- Target Config validation allows root keys `version`, `source`, and `files`; source entries allow `repository` and `ref`; file entries allow `id`, `target_path`, and `if_not_exists`.
- Lock file validation allows root keys `version`, `repository`, `ref`, `commit`, and `files`; lock file entries allow `id` and `target_path`.
- Existing relative path validation continues to apply to `source_path` and `target_path`.
- Old keys `source` and `target` are rejected as unknown keys in their old path-field positions.

## Tests and Docs

- Update `README.md` examples and prose to use `.driftline-source.yaml`, `.driftline-target.yaml`, `source_path`, and `target_path`.
- Update `schema.json` so it describes `.driftline-source.yaml` and requires `source_path`.
- Update config tests to reject old path keys and accept the new ones.
- Update plan tests to verify omitted `target_path` defaults to `source_path`, explicit `target_path` overrides it, duplicate/reserved target checks still work, and locks use `target_path`.
- Update command tests to verify `init` reads `.driftline-source.yaml`, writes `.driftline-target.yaml`, and generated lock files use `target_path`.
- Search current source, tests, schema, and docs for stale current references to `driftline.yaml`, `source` as a file path field, and `target` as a path field.

## Verification

- Run `go test ./src/internal/driftline/...` after implementation.
- Run `go test ./...` before finishing.
- Review the diff for stale current examples of old file names or old path field names.

## Out of Scope

- Compatibility lookup for `driftline.yaml`.
- Compatibility parsing for path fields named `source` or `target`.
- Renaming `source.repository`, `source.ref`, command names, or `--target-dir`.
- Changing the lock file name.
