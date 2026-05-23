# Dockerfile and Go Toolchain Design

## Goal

Add Docker support for both development and runtime use, and align the repository's Go toolchain settings with the latest stable Go version.

## Context

- The repository currently has a Go module at `go.mod` using `go 1.22`.
- mise is configured in `.mise/config.toml` with `go = "1.22.12"`.
- No Dockerfile currently exists.
- The main command builds from `./src/cmd/driftline`.

## Chosen Approach

Use a multi-stage root `Dockerfile` with separate `dev`, `builder`, and runtime stages.

The `dev` target will use the official Go image, install only basic development dependencies such as `git` and `ca-certificates`, and set `/workspace` as the working directory. This keeps local tool development straightforward without turning the runtime image into a heavy development environment.

The `builder` target will compile `./src/cmd/driftline` with `CGO_ENABLED=0` into a Linux binary under `/out/driftline`.

The final runtime target will use `debian:bookworm-slim`, install `ca-certificates`, copy in only the built binary, and set it as the entrypoint.

## Toolchain

Update `.mise/config.toml` to the latest stable Go version. Set `go.mod` to the corresponding Go language version and add a `toolchain` directive for the exact latest patch version so Go command behavior is explicit for contributors who use Go's toolchain management.

## Verification

- Run `go mod tidy` if the local Go toolchain supports the selected version.
- Run `go test ./...`.
- Build the Docker development target.
- Build the Docker runtime target.

## Out of Scope

- Publishing images.
- Adding CI workflow changes.
- Adding Docker Compose or devcontainer configuration.
