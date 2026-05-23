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
WORKDIR /workspace
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/driftline /usr/local/bin/driftline
ENTRYPOINT ["driftline"]
