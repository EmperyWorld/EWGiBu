# ADR-020: Compose import fidelity and reference layers

## Status

Accepted

## Context

Operators import existing [Docker Compose](https://docs.docker.com/compose/)
files into iacpack manifests via `import propose` (ADR-016, ADR-017). Compose
files are authored against the **Compose Specification** and executed by
**runtime Compose** (`docker compose`, or `podman compose` delegating to an
external compose provider — [Podman compose](https://docs.podman.io/en/latest/markdown/podman-compose.1.html)).

iacpack v1 targets a **strict subset** rendered as `compose-docker/v1`
(ADR-007). Import is the **inverse** of that emit subset plus external
networks/volumes (ADR-016), not full Compose Spec fidelity (KNOWN_LIMITATIONS).

Without a single normative model, implementers confuse:

- legitimate Compose syntax (layer 1–2),
- product contract and security policy (layer 3),
- and current code behavior (layer 4).

That gap produces blocking `error` diagnostics for services already excluded
from the draft, import gaps for patterns Docker documents (for example
`repo:${TAG:-default}` image lines), and operator expectation that GPU/CDI
stacks round-trip through iacpack when v1 schema and policy forbid them.

This ADR defines the **reference layers**, **fidelity boundary**, **diagnostic
severity**, and **phased closure** of import/render gaps. It amends the
interpretation of ADR-016/017 partial import; it does **not** weaken TZ default-deny
security (privileged, host namespaces, arbitrary devices, Docker socket, undeclared
host binds, secret values in manifests).

## Normative reference layers

When changing import, render, UI, or docs, apply sources in this order. Lower
layers cannot override higher-layer security rules in TZ §9–§10 and ADR-015.

| Layer | Source | Role |
| --- | --- | --- |
| **1. Compose Specification** | [Services](https://docs.docker.com/reference/compose-file/services/), [Deploy](https://docs.docker.com/reference/compose-file/deploy/), [variable interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/), [profiles](https://docs.docker.com/compose/profiles/) | How a compose file is defined |
| **2. Runtime Compose** | `docker compose config` / `up`; `podman compose` via configured provider | What providers resolve at runtime (env, profiles, GPU shapes) |
| **3. iacpack contract** | `TZ.md` §9–§10, ADR-007, ADR-016–018, ADR-020, [`compose-docker/v1` emit](../../internal/render/compose/model.go), [`KNOWN_LIMITATIONS.md`](../release/KNOWN_LIMITATIONS.md) | Manifest → emit/apply subset |
| **4. Implementation** | `internal/app/importpropose`, `internal/render/compose` | Current behavior; must converge to layers 1–3 where fidelity is promised |

**Fidelity promise:** Where ADR-016 states import is inverse of compose-docker/v1
emit, layer-1 syntax in the **emit subset** MUST map deterministically to manifest
and back to Compose YAML without silent data loss. Fields outside the subset MUST
produce visible diagnostics/refusals (ADR-016) with severities defined below.

**Non-promise:** Full Compose Spec, override files, profile activation identical
to every compose provider quirk, ambient `.env` loading at propose time, or
automatic GPU via service-level `devices:` in v1 manifest.

## Decision

### 1. Fidelity boundary (emit subset)

The authoritative emit model is [`internal/render/compose/model.go`](../../internal/render/compose/model.go)
plus TZ §9.8 service fields. Import MUST support at least:

| Compose (layer 1) | Manifest / emit (layer 3) |
| --- | --- |
| `image` / `build` | `spec.images` (pull / development build) |
| `command`, `entrypoint` | argv arrays |
| `environment` (non-secret) | literal or parameter sources |
| `ports` | ports, bind IP, host literal/parameter |
| bind `volumes` | `resources` + service mounts |
| `networks` | managed + external |
| `healthcheck` | probe + intervals (see §5) |
| `restart` | allowlist |
| `deploy.resources.limits` (`cpus`, `memory`) | `resources.cpus`, `resources.memory` |
| `mem_limit` (service) | Same as `deploy.resources.limits.memory` when limits absent ([Compose services — mem_limit](https://docs.docker.com/reference/compose-file/services/)) |

Permanent **policy deny** at import and compile (TZ §545, ADR-007): `privileged`,
service-level `devices`, `cap_add` / `cap_drop`, host `pid` / `ipc` /
`network_mode`, Docker socket binds, undeclared host binds, reading `env_file`
contents, secret literals in manifest fields.

### 2. Variable interpolation at propose

Runtime Compose resolves interpolation from shell, `--env-file`, and project
`.env` ([interpolation how-to](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/)).

At **propose**, iacpack MUST NOT read ambient `.env`, `--env-file`, or shell
environment (ADR-016, ADR-018). Propose MUST still accept Compose **syntax**:

- `${VAR}`, `${VAR:-default}`, `${VAR-default}`, `${VAR:?msg}`, `${VAR?msg}`
  in supported fields.
- **Defaults from the compose file** (`:-` / `-` forms) MAY be copied into
  parameter defaults or resolved literals where the manifest allows fixed values
  (for example non-secret literals, image ref resolution per §3).
- Required forms (`:?` / `?`) without defaults MUST become parameter gaps
  (info/warning via ADR-018), not silent empty strings.

Document in operator docs: **`docker compose config` with `.env` ≠ import
propose** unless the operator sets defaults through onboarding / manifest
parameters.

### 3. Image references

Layer-1 patterns MUST be supported at import:

1. **Whole-value** interpolation: `${IMAGE:-registry/repo:tag}`, including
   nested `${A:-${B:-registry/repo:tag}}`, → a literal string ref when peeling
   defaults reaches a literal. Info diagnostic when a default was used.
2. **Tag (or digest) segment** interpolation:
   `registry/repo:${TAG:-1.2.3}` → resolved literal ref using the same
   braced-default rules as layer 1; optional info diagnostic when resolved.
3. **No literal remainder** (`${VAR}`, `${VAR:?message}`, or nested
   `${A:-${B}}` whose innermost value is still interpolation) → 
   `spec.images.*.ref` is `{parameter: id}` and the parameter has no default.
   That object is a valid manifest, same as a `valueSource` parameter arm.
   It is not `IMPORT.EMPTY`, not a raw `${…}` string, and not
   "resolved to fixed ref". The string form is written only when a literal
   default is known (compose text, or later when that parameter default is set).

Manifest MUST NOT store raw `${…}` in `spec.images.*.ref`. A parameter ref is
not digest-pinned: propose sets `mode: development`, and release mode still
requires `repository@sha256:<64-hex>` before lock.

### 4. Resource limits

- `deploy.resources.limits.cpus` / `memory` → manifest `resources` (existing).
- Service-level `mem_limit` → manifest `resources.memory` when deploy limits
  memory is absent; if both exist, they MUST agree or propose emits
  `IMPORT.UNSUPPORTED_FIELD` / schema-level conflict diagnostic (error on that
  service only).
- Compose CPU strings MUST normalize to manifest decimal strings (for example
  `2.0` → `2`) per schema pattern.

### 5. Healthcheck: Compose CMD / CMD-SHELL vs manifest argv

Compose runs `test: ["CMD-SHELL", "script"]` in a shell
([healthcheck](https://docs.docker.com/reference/compose-file/services/#healthcheck)).
Manifest stores a **logical probe** as argv plus metadata:

- Import maps `CMD` / `CMD-SHELL` prefixes per Compose.
- For `CMD-SHELL`, manifest MAY store argv as `["/bin/sh", "-c", "<script>"]`
  **or** a dedicated `shellCommand` field if added by schema amendment; render
  MUST emit valid Compose `test` (`CMD-SHELL` or `CMD`) matching layer 1.
- Probes that embed secret interpolation MUST NOT become literal argv; use
  secret mounts + non-secret probe patterns (ADR-005, ADR-018) with warnings.

### 6. Profiles

Compose profile activation ([profiles](https://docs.docker.com/compose/profiles/))
is runtime behavior (`--profile`, `COMPOSE_PROFILES`).

- **Propose request** MUST accept optional `activeProfiles []string` (sorted,
  deterministic). Semantics align with `COMPOSE_PROFILES`:
  - Services with `profiles` omitted → always candidates for import.
  - Services with `profiles` → imported only if intersection with
    `activeProfiles` is non-empty, **or** if `activeProfiles` is empty and
    policy `import.includeUnprofiledOnly` is true (default **true** for v1:
    profile-gated services are **skipped** with info diagnostic, not error).
- When imported, services MAY carry `profiles` in rendered Compose (emit model
  already supports `profiles` on services for tasks); default `iacpack apply`
  MUST document which profiles are enabled at apply time (provider/runbook).

Until schema stores service `profiles`, skipped profile-only services MUST NOT
set `propose.ok` false.

### 7. Partial propose and Accept semantics (amends ADR-016)

ADR-016 allows partial drafts with refusals. Normative rules:

| Situation | Severity | Affects `propose.ok` |
| --- | --- | --- |
| Policy-denied service (excluded from draft) | `warning` + refusal `policy` | **No** |
| Unsupported field omitted on included service | `warning` + refusal `unsupported` | **No** |
| Secret-like / path / collision | `warning` (+ refusal) | **No** |
| Parse error in compose file (cannot read services section) | `error` | **Yes** |
| Included service missing image and build | `error` | **Yes** |
| Merge fragment (`!override` / `!reset`) with no image or build | not a diagnostic | **No** |
| Absolute host bind omitted from an included service | `warning` (error if `--import-strict`) | **No** unless strict |
| Included service with unresolved required interpolation blocking image | `error` | **Yes** |
| Empty draft (no services and no importable images) | `error` | **Yes** |

UI Accept MUST align with `propose.ok` and MUST NOT treat policy/skipped-service
warnings as blocking unless an **included** service has `error` diagnostics.

Optional future: `import.strict=true` (CLI flag / API field) elevates policy
denials on **would-be-included** services to `error` for CI gates.

### 8. GPU and devices

Layer 1 offers:

- Service `devices:` (including CDI names such as `nvidia.com/gpu=all` on some
  Podman flows).
- `deploy.resources.reservations.devices` with `capabilities: [gpu]` ([Deploy devices](https://docs.docker.com/reference/compose-file/deploy/#devices), [GPU how-to](https://docs.docker.com/compose/how-tos/gpu-support)).

Layer 3 (v1): **both are forbidden** in manifest and default compile/operator
policy (TZ §545). Import MUST refuse service-level `devices` with **warning**
and exclude the service (§7).

**Future extension (requires separate ADR + schema + conformance):** only the
**Deploy reservations devices** shape (Docker-documented GPU fields) MAY be
added to manifest; render emits `deploy.resources.reservations.devices`; **no**
automatic translation from service `devices:` CDI lists without a verified
Podman/Docker matrix ([provider issues](https://github.com/containers/podman/issues/19338)).
Until that ADR ships, GPU workloads remain operator-managed Compose outside
iacpack apply or via documented dual-track workflows in
[`compose-import-fidelity.md`](../reference/compose-import-fidelity.md).

### 9. Bind paths and volumes

- Resolve bind sources relative to the **compose file directory** (layer 1).
- Manifest stores **workspace-relative** resource paths (TZ path rules).
- When a compose file lives at workspace root and paths use `../`, import MAY
  apply the documented **rebase-to-workspace-root** heuristic (ADR-017
  implementation) with `IMPORT.PATH_INVALID` **warning** and hint to place
  compose under a subfolder matching Docker project layout.
- Missing host directories at propose time → **warning**; Prepare/stubs (ADR-018)
  are the supported remediation.
- An absolute host bind (`/mnt/...`, `/home/...`, `/run/.../*.sock`) is the same
  policy refusal as `devices` / `privileged`: **warning** by default, **error**
  only with `--import-strict`. That mount is omitted. The service stays in the
  draft, and one such file does not by itself clear `propose.ok`. A relative
  path that contains `docker.sock` stays an error.

Import MUST support long and short volume syntax where layer 1 defines them;
required `${VAR:?}` without default → parameter gap, not silent omit.

### 10. Secrets and `env_file`

Align with ADR-018: never read secret values; suggest declarations; render
Compose secrets subset when declared. Official Compose secret patterns are the
reference for documentation, not for literal env import.

### 11. Multi-file merge

Ordered multi-compose propose (ADR-017) follows **first-wins** entity ids with
`IMPORT.COLLISION` **warning** when a later file repeats a full service and
does not use a Compose merge tag. This is iacpack merge semantics, not a claim
of full Compose `include`/`extends` parity.

A service fragment whose fields use Compose `!override` or `!reset` merges into
the same service id whichever file is seen first. Those tagged fields replace
the mapped manifest fields (`command`, `volumes` → `mounts`, and the other
imported service fields). A fragment with no `image` or `build` is not
`IMPORT.EMPTY` and does not clear `propose.ok`. If the base definition is
policy-excluded (`devices`, `privileged`), the service stays excluded and the
fragment does not become its own service.

### 12. Verification

Changes to import/render fidelity MUST add:

1. Golden fixtures derived from **Docker documentation examples** where possible
   (not production vendor stacks).
2. Round-trip tests: propose → manifest → render → structurally valid Compose
   for the emit subset.
3. Where Podman is claimed: `podman compose config` (or documented equivalent)
   on rendered output for supported fields only.

## Consequences

- [`compose-import-fidelity.md`](../reference/compose-import-fidelity.md)
  becomes the operator-facing capability matrix (layers 1–3).
- KNOWN_LIMITATIONS and QUICKSTART import sections reference ADR-020.
- Implementation work is tracked in phases (below); code may lag this ADR until
  each phase lands with tests.
- ADR-016/017 remain valid; severity and profile/propose OK rules ** amend**
  their partial-import interpretation without changing Accept→Save semantics.

## Rejected alternatives

- **Full Compose Spec import** — conflicts with security, determinism, and v1
  scope (ADR-016 rejected alternative).
- **Reading `.env` during propose** — breaks ADR-018 and reproducible drafts.
- **Silent drop of policy-denied services without diagnostics** — violates
  ADR-016 fail-visible rule.
- **Mapping service `devices:` CDI GPU into manifest without schema/policy ADR**
  — would bypass TZ default-deny.
- **Blocking Accept for entire draft when excluded GPU services emit policy
  warnings** — contradicts ADR-016 partial drafts; fixed by §7 not by editing
  operator Compose files.

## Implementation phases

Phases are ordered; each requires tests and doc updates. Phases do not authorize
skipping security gates or schema validation.

| Phase | Scope | Deliverables |
| --- | --- | --- |
| **F0** | Documentation | Fidelity matrix, QUICKSTART/KNOWN_LIMITATIONS, ADR index, cursor rule |
| **F1** | Import parity (inverse emit) | Image §3, `mem_limit` §4, CPU normalize §4, healthcheck §5, volume/path §9 — **implemented** (see CHANGELOG unreleased) |
| **F2** | Propose semantics | Partial OK §7, `activeProfiles` §6, UI Accept alignment — **implemented** (see CHANGELOG unreleased) |
| **F3** | GPU extension | [ADR-021](021-deploy-gpu-reservations.md): `resources.gpu` 1..8 or `all`, Deploy reservations only — **implemented** |
| **F4** | Secrets/onboarding completion | ADR-018 alignment §10 |
| **F5** | Conformance harness | §12 round-trip started (healthcheck CMD-SHELL); provider `compose config` checks remain operator E2E |

## References

- Docker Compose file reference: [services](https://docs.docker.com/reference/compose-file/services/), [deploy](https://docs.docker.com/reference/compose-file/deploy/)
- [Variable interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/)
- [Profiles](https://docs.docker.com/compose/profiles/)
- [GPU support](https://docs.docker.com/compose/how-tos/gpu-support)
- [Multiple Compose files](https://docs.docker.com/compose/multiple-compose-files/)
- Podman: [podman compose](https://docs.podman.io/en/latest/markdown/podman-compose.1.html)

## Related ADRs

- [ADR-016](016-compose-dockerfile-propose-import.md) — propose import baseline
- [ADR-017](017-deeper-compose-import.md) — multi-file and refusals
- [ADR-018](018-secrets-parameters-onboarding.md) — secrets/parameters onboarding
- [ADR-007](007-compose-docker-v1-provider.md) — compose-docker/v1 provider
