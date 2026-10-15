# ADR-003: Pure compiler and explicit inputs

## Status

Accepted

## Context

Packaging must be reproducible and safe on untrusted manifests. Hidden reads,
network calls, or project code execution during compile would break
determinism and security invariants.

## Decision

Split the product into a pure compiler pipeline
(`load → validate → resolve → compile → plan → render → verify → package`)
and a separate runtime engine. Compiler commands accept only explicitly
declared inputs, lockfile data, non-secret parameters, and policy. They must
not execute project code, launch containers, or perform outbound network I/O.

## Consequences

- Offline `build` is achievable; image fetch is an explicit separate command.
- Adapters for filesystem/network/process are injectable and absent from
  pure paths.
- Tests can prove no ambient environment dependence.

## Rejected alternatives

- Implicit workspace walking for “convenience” — non-deterministic and unsafe.
- Registry pulls inside `build` — couples packaging to network availability.
- Host shell hooks during compile — arbitrary code execution risk.

## Amendment (package builds)

`spec.builds` is an explicit authoring exception to the no-execution rule,
and only for `iacpack build`:

- `validate`, `lock`, `plan`, `inspect`, and `apply` still do not run the
  package manager or project scripts.
- `iacpack build` runs one declared manager (`pnpm`, `npm`, `yarn`, or `bun`)
  with `install: frozen` and `run <script>` in a temporary copy of `context`.
  The script name comes from `package.json`; iacpack does not accept an
  author shell string. Frozen install may use the network.
- The workspace is not modified. Output is copied into `payload/build-<id>/`
  only when it is non-empty and contains the entry file (default
  `index.html`). Apply mounts that payload and does not rebuild.
