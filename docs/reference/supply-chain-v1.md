# Supply chain and offline images (v1)

## Offline OCI cache

```text
iacpack images fetch --file iacpack.yaml --lock iacpack.lock.json --output image-cache/
iacpack build --format bundle --embed-images --image-cache image-cache/ --file … --output out.iacbundle
```

`images fetch` is the only network-enabled packaging step. `build --embed-images`
is network-free and verifies every blob against lock digests before embedding
`images/oci-layout/`.

Runnable code is stdlib-only: fetch uses a `Fetcher` adapter (Docker CLI in
provisioned environments; fixture fetcher in tests). See ADR-013.

## Detached signatures

Signatures never rewrite artifact bytes. Offline verification:

```text
iacpack verify out.iacbundle \
  --signature out.iacbundle.sig.json \
  --trusted-root trusted-root.json \
  --identity-policy identity.json \
  --operator-policy operator.json
```

Default operator policy denies unsigned release artifacts (`allowUnsigned: false`).
`verify`/`plan`/`apply` never contact Rekor/Fulcio. See ADR-010.

## Release rehearsal

```text
make test-release
```

Produces binaries, `SHA256SUMS`, SPDX SBOM, and SLSA-compatible provenance under
`dist/rehearsal/` without publishing. Installation docs must never recommend
`curl | sh`; always verify checksums (and signatures) before execution.
