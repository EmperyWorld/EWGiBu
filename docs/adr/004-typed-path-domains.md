# ADR-004: Typed path domains

## Status

Accepted

## Context

Path traversal, symlink escapes, normalization collisions, and absolute path
injection are primary abuse cases for packaging tools.

## Decision

Represent paths as typed domain values bound to a declared root (workspace,
resource root, bundle root, state root). Core logic never accepts unclassified
raw strings for filesystem operations. Validation rejects traversal, escaping
links, duplicate normalized paths, and unsupported file types at the boundary.

## Consequences

- Call sites must convert and validate paths once at adapters.
- Security defects get regression fixtures under `testdata/malicious/`.
- Arbitrary Dockerfile/string rewriting is out of scope; ownership is
  declaration-based.

## Rejected alternatives

- String concatenation with ad-hoc `Clean` calls — historically bypassed.
- Allowing absolute host paths in manifests — breaks portability and safety.
- Silent path rewriting — hides authoring errors.
