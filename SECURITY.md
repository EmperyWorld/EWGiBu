# Security policy

## Supported versions

| Version | Support |
|---------|---------|
| `1.1.x` | Current stable (v1.0 CLI/runtime + v1.1 authoring UI) |
| `1.0.x` | Security fixes for CLI/runtime-only consumers |
| `0.1.0-rc.*` | Historical RC line; migrate to `1.1.x` |
| `< 0.1.0-rc` | Unsupported |

Product threat model: [`docs/release/THREAT_MODEL.md`](./docs/release/THREAT_MODEL.md).  
Invariants: `TZ.md` and `.cursor/rules/30-security-reproducibility.mdc`.

## Reporting a vulnerability

Report suspected vulnerabilities privately to the repository maintainers.
Do not open a public issue with exploit details, secret samples, or production
credentials.

Include:

- affected commit or version string from `iacpack version --output json`;
- impact summary;
- minimal reproduction that does not require real secrets;
- whether the issue affects packaging purity, path confinement, secret leakage,
  archive handling, or runtime privilege boundaries.

## Non-goals for reporters

Do not request help bypassing access controls of third-party systems. This
project packages and deploys workloads the operator already controls.

## Disclosure preference

Maintainers will acknowledge reports when possible, assess impact against the
product threat model, and prefer coordinated disclosure after a fix is
available in source form.
