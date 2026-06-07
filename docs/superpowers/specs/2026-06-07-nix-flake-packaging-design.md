# Nix Flake Packaging Design

## Goal

Allow external Nix users to build, run, and install `driftline` directly from this repository without adding it to nixpkgs.

## Context

`driftline` is a Go CLI. The binary entrypoint is `./src/cmd/driftline`, and the repository pins Go `1.26.3` in `go.mod` and `.mise/config.toml`.

## Approach

Add a root `flake.nix` that imports nixpkgs and builds the CLI with `buildGoModule`. Filter the package source to `go.mod`, `go.sum`, and `src/` so local Nix result symlinks and documentation edits do not affect the Go build. Publish the package as both `packages.${system}.driftline` and `packages.${system}.default`, then publish matching app outputs so `nix run github:y-writings/driftline` works without an explicit package name.

The flake supports the common Linux and Darwin systems: `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`.

## Outputs

- `packages.${system}.driftline`
- `packages.${system}.default`
- `apps.${system}.driftline`
- `apps.${system}.default`

## Non-Goals

- Registering `driftline` in nixpkgs.
- Adding binary caches.
- Adding a Nix development shell.
- Preserving compatibility with any previous Nix interface, because this repository does not have one.

## Testing

Verify the flake by running `nix flake check` and `nix run . -- --help` or an equivalent command supported by the CLI.
