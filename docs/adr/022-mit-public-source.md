# ADR-022: MIT license and public source snapshot

- Status: accepted
- Date: 2026-10-16

## Decision

At the project owner's request, EWGiBu / iacpack first-party code is licensed
under MIT. Preserve the existing contributor copyright and upstream notices.
Update package metadata, contribution terms, product requirements and release
SBOM generation consistently. Earlier releases retain their delivered licenses.

Publish current source as one initial commit in EmperyWorld/EWGiBu, without
old history, local credentials, agent configuration, caches or release binaries.
Local secret files must be omitted or replaced with explicit placeholders in
public examples. This is source publication, not a signed release/tag; the
human binary-release procedure and acceptance gates remain in effect.
