# ADR-010: Detached signing and provenance

## Status

Accepted

## Context

Release artifacts and bundles need authenticity evidence without changing
deterministic bytes. Verification must work offline during `verify`/`plan`/
`apply`. The product module is stdlib-only (`module iacpack`), so Sigstore
client libraries cannot be imported into the runnable binary.

## Decision

1. **Detached evidence.** Signatures never rewrite `.iacbundle` / binary bytes.
   Operators pass explicit `--signature`, optional `--certificate` /
   `--rekor-proof`, a pinned `--trusted-root` snapshot, and
   `--identity-policy`.
2. **Offline verify.** `verify`/`plan`/`apply` perform only local cryptographic
   checks against the pinned root and identity policy. They never contact
   Rekor/Fulcio/OIDC. Trust-root refresh is a separate administrative step.
3. **Envelope.** Product verifies `iacpack.sig/v1` JSON envelopes containing
   artifact SHA-256, signature algorithm, public key or certificate chain, and
   optional inclusion-proof bytes that are checked structurally against the
   pinned root policy (full Rekor Merkle audit may be delegated to external
   `cosign`/`rekor-cli` when present; the binary never downloads trust material).
4. **Unsigned.** Default operator policy denies unsigned release artifacts.
   Explicit `allowUnsigned: true` is required to proceed.
5. **Provenance/SBOM.** Releases emit SPDX JSON SBOM, SLSA-compatible
   provenance JSON, and `SHA256SUMS`. Build timestamps are not injected into
   product binaries.

## Consequences

- External `cosign sign` remains the recommended producer; the CLI verifies the
  product envelope or a cosign bundle converted into it.
- Tests cover signed/unsigned/untrusted identity/tampered artifact cases with
  fixtures — no network.

## Rejected alternatives

- Embedding Sigstore Go clients — violates stdlib-only module policy.
- Online transparency lookups during apply — breaks offline install and adds
  a network trust dependency on the hot path.
- Mutating bundle bytes with attached signatures — breaks reproducibility.
