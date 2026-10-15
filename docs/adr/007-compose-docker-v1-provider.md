# ADR-007: Docker Compose as the sole v1 provider

## Status

Accepted

## Context

v1 needs one complete, testable deployment target. Supporting multiple
orchestrators immediately would dilute security review and conformance.

## Decision

Ship a single provider profile `compose-docker/v1` that renders and applies
Docker Compose projects with a default-deny security policy (no Docker socket
mounts, no privileged escalation, no undeclared host binds). Other providers
require the extension process in ADR-014.

## Consequences

- Render/apply code can specialize for Compose semantics.
- Acceptance fixtures target Compose + Docker Engine; operator E2E may use
  Podman as a docker-compatible CLI/socket stand-in (not a separate provider
  profile). Apply semantics for per-service up/ready, force-recreate, mount
  RO checks, and YAML 1.1 bool quoting:
  [runtime-apply-v1.md](../reference/runtime-apply-v1.md).
- Manifest `target.profile` rejects unknown providers explicitly.

## Rejected alternatives

- Kubernetes-first v1 — larger surface, weaker local UX for packing apps.
- Multiple providers in parallel for v1 — splits the conformance budget.
- Generating opaque shell installers as the primary runtime — unsafe and
  non-declarative.
