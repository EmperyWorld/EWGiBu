# ADR-006: Canonical lockfile and deterministic bundle

## Status

Accepted

## Context

Two builds from identical declared inputs must produce byte-identical
artifacts for verification, caching, and supply-chain review.

## Decision

Use a canonical lockfile (`iacpack.lock.json`, RFC 8785 JSON) and emit
`.iacbundle` as deterministic `tar.gz` with fixed metadata: Unicode NFC paths,
sorted entries, Unix epoch timestamps, UID/GID 0, normalized modes, LF text,
gzip level 9 with zeroed header fields. Build identity depends only on
declared inputs, lockfile, parameters, policy, renderer, and tool version.

## Consequences

- Ambient environment (cwd, time, locale, hostname) must not affect bytes.
- Archive and lock golden tests become release gates.
- Non-deterministic provider features are rejected or explicitly excluded.

## Rejected alternatives

- “Mostly reproducible” archives with current timestamps — unverifiable.
- Unordered JSON maps — unstable digests.
- Including VCS metadata automatically — couples artifacts to git state.
