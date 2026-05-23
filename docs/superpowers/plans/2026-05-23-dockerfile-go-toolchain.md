# Dockerfile and Go Toolchain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a multi-stage Dockerfile for development and runtime use, and align Go toolchain settings to Go 1.26.3.

**Architecture:** Keep Docker support in a root `Dockerfile` with named `dev`, `builder`, and final runtime stages. Keep toolchain configuration explicit in `go.mod` and `.mise/config.toml` so local, mise, and Docker usage agree.

**Tech Stack:** Go 1.26.3, Docker, Debian bookworm, mise.

---

## File Structure

- Create: `Dockerfile` - multi-stage image definition with `dev`, `builder`, and runtime stages.
- Modify: `go.mod` - update `go` directive to `1.26` and add `toolchain go1.26.3`.
- Modify: `.mise/config.toml` - update Go tool version to `1.26.3`.
- Existing test target: `go test ./...`.
- Existing build target: `./src/cmd/driftline`.

## Task 1: Update Go Toolchain Settings

**Files:**
- Modify: `go.mod`
- Modify: `.mise/config.toml`

- [ ] **Step 1: Update `go.mod`**

Change `go.mod` to:

```go.mod
module github.com/y-writings/driftline

go 1.26

toolchain go1.26.3

require gopkg.in/yaml.v3 v3.0.1
```

- [ ] **Step 2: Update mise Go version**

Change `.mise/config.toml` to:

```toml
[tools]
go = "1.26.3"
```

- [ ] **Step 3: Run Go module check**

Run: `go mod tidy`

Expected: command exits successfully and leaves dependency declarations consistent.

## Task 2: Add Multi-Stage Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Add root Dockerfile**

Create `Dockerfile` with:

```dockerfile
# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.3

FROM golang:${GO_VERSION}-bookworm AS base
WORKDIR /workspace

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
    && rm -rf /var/lib/apt/lists/*

FROM base AS dev
CMD ["bash"]

FROM base AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/driftline ./src/cmd/driftline

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/driftline /usr/local/bin/driftline
ENTRYPOINT ["driftline"]
```

## Task 3: Verify Changes

**Files:**
- Verify: `go.mod`
- Verify: `.mise/config.toml`
- Verify: `Dockerfile`

- [ ] **Step 1: Run Go tests**

Run: `go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Build development image target**

Run: `docker build --target dev -t driftline-dev .`

Expected: image builds successfully.

- [ ] **Step 3: Build runtime image target**

Run: `docker build -t driftline .`

Expected: image builds successfully.

- [ ] **Step 4: Inspect working tree**

Run: `git status --short`

Expected: only intended files are modified or added: `Dockerfile`, `go.mod`, `.mise/config.toml`, and docs under `docs/superpowers/`.

## Self-Review

- Spec coverage: Dockerfile, development target, runtime target, Go toolchain, mise config, and verification commands are covered.
- Placeholder scan: no placeholder tasks remain.
- Type consistency: file paths and command names match the repository layout.
