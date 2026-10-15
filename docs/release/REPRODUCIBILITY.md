# Reproducibility guide

## Guarantees

- Build output depends only on declared inputs, lockfile, explicit non-secret
  parameters, compile policy, renderer, and tool version (`TZ.md` invariants).
- Archives normalize path, time, owner, group, mode, compression metadata.
- Generated collections are sorted; YAML/JSON round-trip to the same canonical model.

## Local verification

```bash
scripts/p15/reproduce-build.sh
```

Builds the CLI twice with `-trimpath` and compares SHA-256. Directory/bundle
rebuilds from `examples/release` are compared in CLI tests (`p06`).

## Platform boundaries

| Artifact | Reproducible claim |
|----------|--------------------|
| Linux amd64/arm64 CLI | Target for byte-identical with same Go toolchain |
| macOS/Windows CLI | Built separately; path separators in docs only — compare within OS |
| Bundle `.iacbundle` | Byte-identical across hosts given same inputs/tool |
| Directory build | Content-identical file digests; host metadata may differ outside tree |

Document any platform-specific difference explicitly in release notes before
claiming cross-OS identity.

## Non-determinism prohibited in product paths

cwd, hostname, username, locale, timezone, umask, wall clock, randomness,
filesystem enumeration order, ambient environment (except explicit flags).
