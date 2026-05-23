# Driftline Full Rename Design

## Goal

Make `driftline` the only product name used by the repository's command, Go module, internal package, CLI surface, Docker image entrypoint, manifest file, manifest schema, and lock file.

## Context

- The repository came from an older naming scheme and still had stale command, module, package, manifest, and lock names.
- The stale names appeared in Go imports, command paths, help output, Docker build steps, tests, and docs.
- The root lock file used by the older naming scheme should be removed rather than migrated.

## Chosen Approach

Perform a breaking rename with no compatibility shim. The canonical command, module, package, manifest, lock, and CLI option names become `driftline` or neutral source/file names. Stale names should not remain in build paths, usage text, Docker entrypoints, default file names, tests, current docs, or internal package/type names.

## Architecture

- Set the Go module path to `github.com/y-writings/driftline`.
- Use `src/cmd/driftline` for the CLI entrypoint.
- Use `src/internal/driftline` for the internal package and `src/internal/driftline/commands` for CLI command handling.
- Update package declarations and imports to use `driftline`.
- Rename source-directory options to `SourceDir` in Go and `--source-dir` in the CLI.
- Rename manifest item types to file-oriented names such as `ManifestFile` and `Manifest.File`.
- Replace user-facing reason/error text with neutral source/file wording.

## File Formats

- Use `driftline.yaml` as the default manifest path.
- Use `files` as the manifest collection key.
- Use `.driftline.lock` as the default lock path.
- Delete the stale root lock file.
- Do not implement fallback lookup for stale manifest or lock file names.

## CLI and Docker

- Use `driftline` in flag parsing metadata and usage output.
- Build `./src/cmd/driftline` in Docker.
- Write the built binary to `/out/driftline`, copy it to `/usr/local/bin/driftline`, and set `ENTRYPOINT ["driftline"]`.

## Tests and Docs

- Update tests to use `driftline.yaml`, `.driftline.lock`, `usage: driftline`, `--source-dir`, and `y-writings/driftline`.
- Update current docs so stale build, module, command, manifest, and lock names do not remain.

## Verification

- Run `go test ./...` after the rename.
- Build the Go command with `go build ./src/cmd/driftline`.
- Confirm no non-historical source, Dockerfile, test, or current docs references remain for stale command, module, manifest, lock, package, or option names.

## Out of Scope

- Backward compatibility for stale command, manifest, or lock names.
- Rewriting Git history or generated session logs.
- Publishing images or releases.
