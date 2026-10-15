# ADR-001: Go single-binary architecture

## Status

Accepted

## Context

`iacpack` must ship a reproducible CLI used by developers, CI, and a later
local API embedding path. The product needs strong static typing, portable
cross-compilation, and a dependency boundary that keeps domain logic free of
CLI/HTTP adapters.

## Decision

Implement the core product in Go as a single local module (`iacpack`) with one
primary command binary `cmd/iacpack`. Domain and application packages live under
`internal/`; adapters under `internal/adapters/`. Runnable product code uses the
Go standard library only and must not import remote module paths (host-style
import paths). The Web UI remains a separate TypeScript package and may later
be embedded as static assets.

## Consequences

- One toolchain and test harness for compiler and runtime.
- Architecture import rules can be enforced in Go tests.
- Distribution of prebuilt binaries is optional; the repository remains
  source-first and never commits compiled artifacts.

## Rejected alternatives

- Polyglot core (Go + Python/Ruby) — splits contracts and weakens determinism.
- Plugin RPC architecture for v1 — adds process complexity without provider
  multiplicity in v1.
- Pure library without a CLI — fails the developer and CI UX requirement.
