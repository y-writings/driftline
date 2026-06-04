# AGENTS.md

## Project Status

`driftline` is a pre-release CLI tool. It has not shipped a stable public interface, so there is no compatibility contract for existing command names, flags, configuration fields, output formats, lock-file formats, internal APIs, or repository layout.

## Breaking Changes

Breaking changes are allowed in this repository. Prefer the simplest correct design for the current product direction over preserving old behavior.

When changing behavior, do not add backward-compatibility code unless the user explicitly asks for it or there is a concrete shipped/persisted-data reason. Avoid compatibility shims, deprecated aliases, legacy config readers, dual output formats, migration layers, and old-name wrappers that exist only to preserve unreleased behavior.

If an old interface conflicts with a clearer new interface, replace it instead of supporting both. Update the implementation, tests, schema, and documentation to the new behavior directly.

## Safety Boundary

In this file, "breaking change" means breaking CLI/API/config/schema/output compatibility. It does not authorize destructive shell commands, deleting user data, discarding unrelated worktree changes, or bypassing normal safety checks.

## When Unsure

If backward compatibility seems necessary, state the concrete reason briefly. If the reason is only "someone might rely on this," assume they do not unless the user confirms otherwise.
