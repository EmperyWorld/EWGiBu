# Contributing

## Scope

Work follows `TZ.md` and the active phase in `IMPLEMENTATION_PLAN.md`. Do not
implement future-phase features early or weaken security/reproducibility gates.

## Source-only rule

Commit source, schemas, docs, and tests only. Never commit binaries, populated
`.env` files, credentials, private keys, or generated release archives.

## Developer checks

```bash
make check
make test-race
make test-security
make build
```

Subsystem targets such as `test-ui` may report that the subsystem is not
present yet; that is expected until the owning phase lands.

## Contract changes

Changes to schemas, CLI JSON envelopes, diagnostics, or exit codes are public
contract changes. Update schemas, golden tests, docs, and an ADR in the same
change. Silent compatibility shims are forbidden.

## License and disclaimer

By contributing you agree that your contributions are licensed under the
MIT License. The project is provided AS IS; contributors do not assume
liability for downstream use. See `LICENSE` and `NOTICE`.

## No unsolicited publishing

Do not create remotes, open pull requests, or push commits unless a maintainer
explicitly asks for that action.
