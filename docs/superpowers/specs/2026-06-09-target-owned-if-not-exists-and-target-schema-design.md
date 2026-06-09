# Target-Owned If-Not-Exists And Target Schema Design

## Goal

Move `if_not_exists` fully into Target Config responsibility and make both current config files editor-validatable.

`.driftline-source.yaml` must not accept `files[].if_not_exists`. `.driftline-target.yaml` remains the only place where target-side overwrite policy is configured.

## Current State

- `schema.json` describes `.driftline-source.yaml` only.
- Source Manifest file entries currently allow `id`, `paths`, and `if_not_exists`.
- Target Config file entries currently allow `id`, `path_overrides`, and `if_not_exists`.
- Runtime currently resolves `if_not_exists` from Source Manifest first, then lets Target Config override it.
- `driftline init` currently copies Source Manifest `if_not_exists` into generated Target Config entries.

## Configuration Contract

Source Manifest `.driftline-source.yaml`:

- Root properties: `version`, `gitignore`, `files`.
- File entry properties: `id`, `paths`.
- File entry required properties: `id`, `paths`.
- `files[].if_not_exists` is invalid and rejected as an unknown key.

Target Config `.driftline-target.yaml`:

- Root properties: `version`, `source`, `files`.
- `source` properties: `repository`, `ref`.
- File entry properties: `id`, `path_overrides`, `if_not_exists`.
- `path_overrides[]` properties: `from`, `to`.
- File entry required properties: `id`.
- `if_not_exists` is a target-owned boolean. Omitted means `false`.

## Schema Files

Keep `schema.json` as the canonical Source Manifest schema. It will continue to use:

```json
"$id": "https://raw.githubusercontent.com/y-writings/driftline/main/schema.json"
```

Add `target-schema.json` as the canonical Target Config schema with:

```json
"$id": "https://raw.githubusercontent.com/y-writings/driftline/main/target-schema.json"
```

Both schemas use JSON Schema draft 2020-12 and `additionalProperties: false` at every object level. Target schema mirrors parser constraints where schema can express them: `version: 1`, required `source`, required `files`, `repository` shaped as `owner/repo`, non-empty `ref`, non-empty `path_overrides` when present, and the same relative path pattern for `from` and `to` that Source Manifest uses for `paths[]`.

The schema will not try to verify that `path_overrides[].from` matches a path in the referenced Source Manifest. That is runtime validation.

## Runtime Behavior

`SourceManifestFile` no longer has an `IfNotExists` field. Source parsing rejects `if_not_exists` through strict key validation.

`TargetConfigFile.IfNotExists` becomes `bool` instead of `*bool`. Runtime resolution uses only `configured.IfNotExists`; there is no Source Manifest fallback. Existing targets for `if_not_exists: true` remain intentionally untouched and are not reported as drift, preserving the current execution semantics while moving ownership.

`TargetConfigFromSourceManifest` generates one Target Config entry per Source Manifest `id` and never sets `if_not_exists`. Users add `if_not_exists: true` in `.driftline-target.yaml` when the target repository wants that policy.

## Documentation

README lists allowed properties for both config files.

Source Manifest documentation shows `id` and `paths` only for `files[]`. It does not call out `if_not_exists` specifically; the property list is the source of truth.

Target Config documentation includes the target schema language-server directive and shows `if_not_exists: true` in a Target Config file entry.

## Tests

Update parser and runtime tests to prove the contract:

- Source Manifest strict validation succeeds without `if_not_exists`.
- Source Manifest unknown-key tests reject `files[].if_not_exists`.
- Target Config parsing accepts `if_not_exists: true` and `if_not_exists: false` as booleans.
- Plan and update fixtures put `if_not_exists` on Target Config entries, not Source Manifest entries.
- Init tests use Source Manifest fixtures without `if_not_exists` and assert generated Target Config output does not contain `if_not_exists`.
- Schema tests keep `schema.json` synchronized with `allowedSourceManifestKeys()` and `target-schema.json` synchronized with `allowedTargetConfigKeys()`.

## Non-Goals

- Do not add compatibility parsing or migration for Source Manifest `if_not_exists`.
- Do not rename `schema.json` to `source-schema.json`.
- Do not add per-path `if_not_exists` within an adoption unit.
- Do not update historical design plans except where current README, schemas, implementation, or tests require it.
