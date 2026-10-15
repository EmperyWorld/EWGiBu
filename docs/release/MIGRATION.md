# Migration and versioning

## Manifest

- Current schema: `iacpack.io/v1alpha1` (`schemas/` + `docs/reference/manifest-v1alpha1.md`).
- Explicit upgrade: `iacpack manifest migrate --file <path>` (atomic write + backup
  of metadata only). Review the diff; do not treat migration as infrastructure rollback.

## Bundle format

Readers must accept the declared format version or exit
`BUNDLE.UNSUPPORTED_VERSION` (ADR-006 / bundle reference).

## Support window

Stable v1 schemas remain readable for at least two minor product releases after
a replacement appears (`TZ.md` §19.3).

## Lockfile

Regenerate with `iacpack lock` after manifest or tool upgrades. Drift between
manifest and lock fails validate `--locked` / build.
