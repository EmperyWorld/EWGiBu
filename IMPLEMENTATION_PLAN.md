# IaC Packager — Cursor Implementation Plan

**Audience:** Cursor coding agents and human reviewers  
**Authoritative product specification:** [`TZ.md`](./TZ.md)  
**Plan language:** English  
**Execution model:** complete phases in order; do not combine phases unless all gates of each phase remain independently reviewable  

## 1. How to use this plan

Each phase below contains a self-contained prompt. Give one prompt at a time to a Cursor agent and execute phases in numeric order. The agent must finish the implementation, run the stated checks, update documentation, and provide a handoff before the next phase starts.

The agent must not treat this file as permission to implement future phases early. A phase may create a narrow interface needed by its own behavior, but it must not add speculative providers, plugins, compatibility layers, or empty framework code.

Before every phase:

1. Read `TZ.md`, this plan, applicable `.cursor/rules/*.mdc`, existing ADRs, and the current repository tree.
2. Run the current baseline checks before editing.
3. Compare the actual repository state with the phase prerequisites.
4. If a prerequisite is missing, implement only the smallest prerequisite repair and document it; do not silently redesign contracts.
5. Preserve all accepted behavior and golden fixtures unless the phase explicitly changes a contract.

After every phase, the handoff must list:

- user-visible behavior added;
- files/contracts/schema versions changed;
- security and determinism implications;
- tests added;
- exact commands run and their results;
- checks not run and the concrete reason;
- remaining limitations that are explicitly assigned to later phases.

## 2. Non-negotiable invariants

These requirements apply to every phase:

1. Source workspaces are read-only except for explicit authoring commands such as `init`, `lock`, migration, and UI Save.
2. `validate`, `inspect`, preview, `build`, and `verify` execute no project code, launch no containers, and perform no network access.
3. Build output depends only on declared inputs, lockfile, explicit non-secret parameters, policy, renderer, and tool version.
4. No secret value may enter a manifest, lockfile, bundle, plan, diagnostics, logs, docs, Compose environment, process arguments, or persisted journal.
5. Every filesystem path is typed and confined to its declared root; traversal, escaping links, and normalization collisions fail closed.
6. CLI, API, and UI call the same application services and return the same diagnostics.
7. Unsupported versions, fields, capabilities, platforms, and provider options fail explicitly.
8. Generated ordering and serialization are deterministic.
9. File writes use staging and atomic replacement. Directory builds require a new target. Successful existing output is never modified on failure.
10. Persistent volumes and pre-existing resources are never deleted by default.
11. Shell command strings are not used for subprocess execution; use argument arrays.
12. Tests and validation may not be weakened to make a phase pass.
13. Every new CLI JSON result must add/update a checked-in `schemas/cli/v1`
    schema and validation/golden tests in the same phase.

## 3. Target technology and repository shape

### Backend/runtime

- Go 1.27.1 baseline, pinned through `go.mod` toolchain configuration.
- Go module path: `iacpack` (local name only; no host/URL module path).
- Initial product version: `0.1.0-dev`.
- Stdlib CLI router (no third-party CLI framework in runnable product code).
- `gopkg.in/yaml.v3` plus strict AST checks.
- JSON Schema Draft 2020-12.
- RFC 8785 canonical JSON.
- `go-containerregistry` for OCI operations.
- bbolt or an ADR-approved equivalent for the durable runtime journal.
- Go `net/http`, OpenAPI 3.1, and generated API bindings.

### Frontend

- Node.js 24 LTS and pnpm 10, exact versions pinned when the UI is introduced.
- TypeScript strict mode, React 19, Vite, `@xyflow/react`.
- Generated OpenAPI client.
- Vitest, Testing Library, and Playwright.

### Intended layout

```text
cmd/iacpack/
internal/
  app/
  diagnostic/
  manifest/
  domain/
  inputs/
  lock/
  policy/
  render/compose/
  bundle/
  runtime/
  adapters/
api/openapi/
schemas/
web/
docs/adr/
docs/reference/
examples/
testdata/
e2e/
```

## 4. Phase order

```text
P00 → P01 → P02 → P03 → P04 → P05 → P06 → P07
    → P08 → P09 → P10 → P11 → P12 → P13 → P14 → P15 → P16
    → P17 → P18 → P19
```

Numeric order is authoritative. The compiler/API work could technically be parallelized, but this plan deliberately keeps one linear, reviewable sequence.

### Gate ownership

- `make check`, `make test-race`, and available unit/security targets are local-agent gates.
- Docker/Compose version matrix, cross-OS builds, and full E2E are CI gates.
- Signing identity, dedicated performance hardware, release tag, and publish are human-controlled release gates.
- A phase may add a named target when its subsystem appears, but must never report an unavailable CI/human gate as passed.

## 5. Phase summary

| Phase | Outcome | Primary gate |
|---|---|---|
| P00 | Reproducible repository, architecture boundaries, CI | Fresh copied workspace passes baseline |
| P01 | `init` and strict `validate` walking slice | YAML/JSON fixtures and stable diagnostics |
| P02 | Complete v1alpha1 manifest/domain contract | Full schema corpus |
| P03 | Safe resource resolution and canonical lockfile | Path/security/reproducibility corpus |
| P04 | Deterministic lifecycle DAG and graph inspection | Cycle/order/task tests |
| P05 | Compose, docs, `.env.example`, directory output | Golden output and Compose validation |
| P06 | Deterministic `.iacbundle`, verification, semantic diff | Byte-identical archive tests |
| P07 | Runtime planning, operator policy, preflight, state foundation | Mutation-free plan/doctor |
| P08 | Docker apply, health, tasks, journal | Reference app deploys twice safely |
| P09 | Resume/update/destroy and Linux distribution | Failure injection and volume retention |
| P10 | Offline images, signing, SBOM, release workflow | Offline install and signed release rehearsal |
| P11 | Loopback OpenAPI service | CLI/API equivalence |
| P12 | First usable Web UI | Two-service form workflow |
| P13 | Visual dependency designer | Semantic graph round-trip |
| P14 | Disk synchronization and accessibility | Conflict safety and WCAG gate |
| P15 | Security/performance/release hardening | All v1/v1.1 acceptance criteria |
| P16 | Reviewable Compose/Dockerfile import drafts | Propose→Accept→Save; subset goldens |
| P17 | Deeper Compose import + multi-file onboarding | Discover widen; multi-propose; refusals UX |
| P18 | Secrets/parameters onboarding from import | Stubs only; no values; `.env.example` |
| P19 | GitHub-oriented release templates | Explicit template write; no auto-publish |

---

## P00 — Contracts and reproducible repository bootstrap

### Deliverables

- Local Git repository if one does not exist; no remote is created.
- Go module and `cmd/iacpack` binary with `version`.
- Architecture package boundaries and dependency checks.
- MIT `LICENSE`, `README.md`, `CONTRIBUTING.md`, `SECURITY.md`.
- ADRs 001–007 and 014 from `TZ.md`.
- Pinned toolchain, reproducible developer commands, CI, fixtures conventions.
- Project Cursor rules from `.cursor/rules/`.

### Copy/paste prompt

```text
Implement Phase P00: repository bootstrap for the IaC Packager product.

Authoritative inputs:
- Read TZ.md completely.
- Read IMPLEMENTATION_PLAN.md and all .cursor/rules/*.mdc.
- The repository may contain only documentation and may not yet be a Git repository.

User-visible outcome:
A clean, reproducible Go repository builds an `iacpack` CLI. `iacpack version`
prints a stable text form and `iacpack version --output json` prints a versioned
JSON object. No product compiler behavior is required yet.

Required work:
1. Initialize a local Git repository only if absent. Do not create a remote,
   publish anything, or commit unless the user separately requests it.
2. Create module `iacpack` (no host/URL in the module path), set initial product
   version `0.1.0-dev`, and pin Go 1.27.1 in module/toolchain configuration. If
   that toolchain cannot be acquired, report the exact blocker rather than
   downgrade. Runnable product code must not import remote module paths.
3. Create the intended package tree, but create packages only when they contain
   useful code in this phase. Do not add empty future-provider stubs.
4. Add a stdlib CLI router, a build/version package, and `cmd/iacpack`.
5. Implement deterministic version output containing product version, commit,
   and dirty/build metadata only when supplied through build flags. The default
   development output must not inject the current time.
6. Add Makefile or equivalent commands for format, lint, test, race, security,
   build, and check. Establish named targets from TZ.md: check, test-race,
   test-security, test-integration, test-e2e, test-ui, test-release, test-all;
   future-subsystem targets may initially explain that the subsystem is absent.
   Pin staticcheck/govulncheck and all tool versions through repository tooling.
7. Add GitHub Actions CI with third-party actions pinned to full commit SHAs,
   least permissions, dependency caching, Go build, vet, staticcheck, tests,
   race tests, and govulncheck. Release publishing is out of scope.
8. Add MIT LICENSE, README quick developer setup, CONTRIBUTING, SECURITY,
   .gitignore, .editorconfig, and testdata conventions.
9. Create concise ADRs for: Go single binary; JSON Schema contract; pure
   compiler; typed paths; secret references; deterministic bundle; Docker
   Compose as the sole v1 provider; provider extension/conformance process.
   ADRs must state context, decision, consequences, and rejected alternatives.
10. Add an architecture dependency test or documented enforceable mechanism so
    domain packages cannot import CLI, HTTP, filesystem, process, or Docker
    adapters.
11. Check in `schemas/cli/v1/version.schema.json` and a common machine-envelope
    schema matching TZ.md §12.4. Version JSON fields are exactly product,
    version, goVersion, commit, and dirty inside `result`; no timestamp.

Non-goals:
- Do not implement manifest parsing, Docker integration, API, UI, plugins,
  providers, generated installers, or release publishing.
- Do not add framework dependencies for future phases.

Required tests:
- Version command text and JSON golden tests.
- JSON output is one valid document, validates against the checked-in schema,
  and contains no timestamp.
- Architecture dependency rule has a failing fixture or a focused unit test.
- Copy the repository contents (including uncommitted files) to a fresh temporary
  directory and prove the documented bootstrap/check commands pass there. Do
  not call this a Git clone until a human has created commits.

Acceptance commands:
- gofmt/check command
- go vet ./...
- staticcheck ./...
- go test ./...
- go test -race ./...
- govulncheck ./...
- go build ./cmd/iacpack
- built binary: `iacpack version`
- built binary: `iacpack version --output json`

Apply all global invariants from IMPLEMENTATION_PLAN.md. Do not weaken a check
because a tool is missing: install/pin it through documented tooling or report
the exact unavailable check. End with the required phase handoff.
```

---

## P01 — Strict loader, diagnostics, `init`, and minimal validation slice

### Deliverables

- Safe YAML/JSON loader with source locations.
- Initial JSON Schema and minimal typed model.
- Stable diagnostics and exit-code plumbing.
- `iacpack init`, `validate`, `schema export`.
- Minimal image-based service fixture.

### Copy/paste prompt

```text
Implement Phase P01: the first manifest-to-validation walking slice.

Prerequisites:
- P00 checks pass.
- Read TZ.md sections 7–12 and existing ADRs/rules before editing.

User-visible outcome:
`iacpack init --output iacpack.yaml` creates a valid minimal manifest.
`iacpack validate --file iacpack.yaml` validates YAML or equivalent JSON.
`--output json` returns stable machine-readable diagnostics. No artifacts or
containers are created.

Implement:
1. Add JSON Schema Draft 2020-12 at
   schemas/manifest/v1alpha1/schema.json for the minimal contract:
   apiVersion, kind, metadata name/version, target profile, release mode,
   one digest-pinned image, and one service using that image.
2. Implement a strict loader that accepts YAML 1.2-compatible scalar semantics
   and JSON, exactly one document, string map keys, maximum 2 MiB and depth 100.
   Reject duplicate keys, merge keys, aliases, anchors, custom tags, and extra
   documents before typed decoding.
3. Preserve source file, line, column, and JSON Pointer mappings.
4. Reject unknown fields. Reject unsupported apiVersion/kind/profile explicitly.
5. Introduce immutable DTO/domain conversion; parser DTOs must not leak into
   domain/application packages.
6. Implement the diagnostic model from TZ.md with deterministic ordering and
   namespaced stable codes. Aggregate independent schema errors.
7. Implement `init`, `validate`, and `schema export`. `init` must refuse to
   overwrite by default and use atomic creation. `validate` is read-only and
   network/process free.
8. Implement text output on stderr and one versioned JSON result on stdout.
   Respect exit codes 0, 2, 3, 4, and 70 and disable color in JSON/non-TTY mode.
9. Check in schemas/cli/v1/validate.schema.json and diagnostics.schema.json.
   All JSON success/failure results use the common envelope from P00 and validate
   against these schemas; do not invent an undocumented top-level shape.
10. Add examples/minimal and positive/negative testdata. YAML and equivalent JSON
   must produce the same canonical typed value.

Required diagnostic fixtures:
- malformed YAML/JSON;
- duplicate key;
- alias/anchor/merge key;
- unknown field;
- missing required field;
- bad identifier and SemVer;
- unsupported version/profile;
- mutable image tag in release mode;
- over-limit input.

Non-goals:
- No paths/resources, lockfile, Compose rendering, build command, graph, Docker,
  API, UI, discovery, environment interpolation, or secret values.

Acceptance:
- Current repository checks all pass.
- `go test -race ./...`.
- Run init, validate its YAML, convert fixture to equivalent JSON, validate it.
- Compare canonical test representation for YAML and JSON.
- Confirm source files are byte-identical before and after validate.
- Confirm validate opens no network and starts no subprocess via injected
  adapters/tests.

Keep generated output deterministic and apply every global invariant. Update
README and manifest reference for only the implemented subset. End with the
required phase handoff.
```

---

## P02 — Complete v1alpha1 manifest and semantic validation

### Deliverables

- Complete schema/model for parameters, secrets, images/builds, resources, networks, volumes, services, healthchecks, dependencies, tasks, package, and `x-ui`.
- Typed value/path domains.
- Versioning and capability validation.
- Full schema corpus and generated reference.

### Copy/paste prompt

```text
Implement Phase P02: complete the iacpack.io/v1alpha1 authoring contract.

Prerequisites:
- P01 is green and its CLI/output behavior must remain compatible.
- Read TZ.md sections 9, 10, 12, and 18 in full.

Outcome:
The checked-in JSON Schema, Go boundary models, domain models, generated schema
reference, examples, and validation diagnostics describe the complete v1alpha1
manifest. Validation remains pure; no local resource traversal, registry access,
Docker calls, templates, or artifact generation occur yet.

Required contract:
0. Before code, write ADR-012 for x-ui/semantic hashing and ADR-015 for
   compile/operator policy layering. Keep them consistent with TZ.md §9.15.
1. Implement metadata, target profile, development/release mode, typed build and
   deploy parameters, secret declarations, OCI ref/build image union, declared
   resources, networks, volumes, services, ports, mounts, non-secret
   environment sources, secret file mounts, healthchecks, readiness
   dependencies, lifecycle tasks, package options, and x-ui.
2. Use discriminated objects such as `{literal: ...}` and `{parameter: ...}`.
   Do not accept `${VAR}` interpolation or ambient environment values.
3. Secrets are declarations only and v1 delivery is file-only. Reject literal
   values and secret use in non-secret fields.
4. Unknown fields fail except the precisely documented x-ui extension shape.
5. Enforce identifier, SemVer, duration, port, protocol, mode, container path,
   command argv, restart-policy, and resource-limit constraints.
6. Release mode requires digest-pinned image refs. Build images are
   development-only and explicitly marked as non-reproducible runtime builds.
7. Model WorkspacePath, BundlePath, ContainerPath, ManagedHostPath, and
   ExternalHostPath as distinct domain types. This phase validates lexical
   syntax; filesystem containment is P03.
8. Validate references: images, parameters, secrets, volumes, networks,
   resources, dependencies, and task targets.
9. Validate statically resolvable host port conflicts using the wildcard overlap
   rules in TZ.md. Defer unresolved deploy-parameter values to P07 plan; equal
   parameter references are statically equal. Equal container ports remain valid.
10. `healthy` dependencies require a target healthcheck.
11. x-ui data must be excluded from deployment semantic hashing. Add tests that
    position-only edits preserve the semantic model hash.
12. Add `iacpack inspect manifest` to print canonical public semantics without
    secret values, in text and JSON.
13. Generate or maintain a human manifest reference from the same contract and
    add complete valid development and release examples.
14. Add schemas/policy/v1/schema.json and the immutable default compile policy.
    It controls registry allowlists and supported development/build-network/
    external-reference capabilities, resource ceilings, and package quotas.
    High-risk requirements are explicit and recorded; it cannot unlock schema-
    unsupported privileged capabilities. Implement pure policy evaluation.
15. Implement every default, union arm, range, duration/retry shape, restart
    allowlist, build-context field, external reference, package-option rule, and
    exact x-ui shape from TZ.md §9.13–9.15. Do not leave prose for agents to guess.

Testing:
- Positive fixture for every field/union arm.
- Negative fixture for every constraint and invalid cross-reference.
- Equivalent YAML/JSON property tests.
- Unknown-field tests at every object level.
- Parameter boundary and tagged-source tests.
- Secret canaries proving diagnostics/inspect output do not expose values.
- Schema/Go model drift test.
- Fuzz loader plus typed conversion; malformed input must never panic.

Non-goals:
- Do not resolve files/globs, contact registries, build graphs, render Compose,
  package bundles, execute Docker, add API/UI, or support Podman/k3s/host hooks.

Acceptance:
- All Go checks and race tests pass.
- Every example validates.
- Schema corpus runs in deterministic order.
- `inspect manifest` is stable across repeated runs and YAML key reordering.
- Existing P01 JSON diagnostics remain backward compatible.

Apply all global invariants. If TZ.md and the prior schema conflict, preserve the
documented specification and update the schema/tests together; do not invent a
silent compatibility behavior. End with the required phase handoff.
```

---

## P03 — Safe input resolution, explicit parameters, OCI locking, and lockfile

### Deliverables

- Workspace confinement and deterministic glob expansion.
- File hashing/copy plan.
- Explicit build parameter values.
- OCI digest resolution only through explicit lock operation.
- RFC 8785 `iacpack.lock.json`.

### Copy/paste prompt

```text
Implement Phase P03: safe declared-input resolution and the canonical lockfile.

Prerequisites:
- P02 full schema/domain tests pass.
- Read TZ.md sections 7, 9.5–9.7, 10, 11, 12, and 18.

Outcome:
`iacpack lock` resolves declared workspace inputs and optionally OCI image tags,
then atomically writes canonical `iacpack.lock.json`. Ordinary `validate` now
performs full input and compile-policy validation; `validate --syntax-only`
explicitly performs only parser/schema checks. `inspect inputs` reports the same
deterministic resolution without changing the workspace. `build` is not
implemented in this phase.

Required behavior:
1. Establish manifest directory as the workspace root unless an explicit
   permitted root is supplied by the application layer.
2. Resolve only declared resources and build-context include/exclude patterns.
   Define and document doublestar glob behavior. Sort matches by normalized
   UTF-8 bundle path.
3. Reject absolute paths, `..`, backslashes, drive/UNC prefixes, NUL, output
   recursion, case-folding collisions, Unicode-normalization collisions,
   symlinks, hardlinks, devices, sockets, FIFOs, and any canonical escape.
4. Empty matches fail unless `allowEmpty: true`. Source files need read access
   only. Never rewrite source content.
5. Normalize recorded modes to the contract; preserve executability only when
   explicitly declared.
6. Resolve build parameter precedence exactly as TZ.md: `--set`, explicit
   values file, non-secret default. Never read process environment or `.env`.
   Reject secret-like fields in values files and never accept secrets on CLI.
7. Implement canonical RFC 8785 lock JSON with manifest/schema/provider/policy/
   compiler digests, build values, sorted input path/size/mode/SHA-256 records,
   and resolved OCI digests. Exclude timestamp, host, user, cwd, temp paths, and
   secrets.
8. `lock` performs no network by default. Mutable OCI refs require explicit
   `--update-images`; only then may the registry adapter access the network.
   Make registry authentication adapter-based and ensure credentials cannot
   enter diagnostics or lockfiles.
9. Use staging, restrictive temporary permissions, fsync where supported, and
   atomic replacement. Refuse overwrite only where the command contract says so.
10. Add `--locked` validation and drift diagnostics. Any file bytes/mode/list,
    image digest, schema, policy, build value, or relevant compiler contract
    drift must fail with a precise code.
11. Implement `inspect inputs` in text/JSON, excluding host absolute paths from
    stable machine output unless a debug-only local field is explicitly marked.
12. Apply the P02 compile policy during validate/lock and record its canonical
    digest and required capabilities. An explicit policy may select only
    schema/provider-supported capabilities; runtime operator policy remains an
    independent stricter acceptance boundary.
13. Implement deterministic `init --discover` and `inspect --discover` as
    reviewable suggestions. Recognize only documented Dockerfile and public
    well-known config names; never read/propose `.env`, key, credential, secret-
    like, VCS, editor, or output files. Discovery never changes normal build input.

Mandatory tests:
- traversal, escaping/cyclic links, path collisions, odd Unicode, case variants,
  output-inside-input, permission failure, changing file during hash, and empty
  glob cases;
- deterministic expansion under randomized directory creation order;
- canonical lock golden and build-parameter precedence;
- no ambient environment influence;
- full validate rejects missing/escaping inputs; `--syntax-only` is clearly
  marked insufficient for a release gate;
- deterministic discovery and sensitive-file exclusion;
- fake registry tests for explicit/no-network behavior and credential redaction;
- changing declared input changes lock; undeclared files do not;
- atomic-write failure preserves prior valid lockfile;
- fuzz path/glob normalization and canonical JSON.

Non-goals:
- No Compose output, DAG, bundle, Docker execution, arbitrary discovery, API,
  UI, secret values, or local image builds.

Acceptance:
- All baseline, race, fuzz smoke, and security corpus checks pass.
- Generate a lock twice from copied workspaces with different absolute paths,
  locale, timezone, and creation order; bytes must match.
- Prove validate/inspect are read-only and network/process free.

Apply all global invariants and end with the required phase handoff.
```

---

## P04 — Dependency graph, lifecycle tasks, planning, and inspection

### Deliverables

- Immutable IR.
- Deterministic action DAG.
- Cycle and readiness validation.
- Text/JSON/Mermaid inspection.
- Lifecycle task semantics.

### Copy/paste prompt

```text
Implement Phase P04: compile a validated locked project into immutable IR and a
deterministic lifecycle action graph.

Prerequisites:
- P03 passes with canonical lockfiles.
- Read TZ.md sections 8, 9.8–9.12, 12, and 15.1–15.3.

Outcome:
`iacpack inspect graph` produces equivalent text, versioned JSON, and Mermaid
views of the same deterministic graph. Compilation and inspection are pure and
execute no tasks or provider operations.

Required work:
0. Write ADR-009 for container tasks/no host hooks and freeze the lifecycle
   phase/edge matrix before implementation.
1. Define immutable IR separate from authoring DTOs. Convert only after schema,
   reference, parameter, input, lock, provider-capability, and policy validation.
2. Model action kinds for verify/preflight, resource provisioning, image
   availability, preStart tasks, service start, readiness, migration tasks,
   postDeploy tasks, and finalize. Runtime execution comes later.
3. Edge direction is prerequisite -> dependent. Use stable IDs and lexical
   tie-breaking for simultaneously runnable actions.
4. Enforce that the readiness/task execution graph is acyclic. Return the
   actual minimal/reproducible cycle path in diagnostics.
5. Distinguish `started`, `healthy`, and `completedSuccessfully`. `healthy`
   requires a healthcheck; completedSuccessfully targets a one-shot task.
6. Implement task phase, argv command, working directory, dependencies, timeout,
   bounded retries/backoff, and rerun policy (`always`, `oncePerDeployment`,
   `manual`). Default failure policy is stop.
7. Do not interpret ordinary network communication as an execution dependency.
8. Compute a semantic deployment hash excluding x-ui but including all runtime
   behavior. The hash and graph must be stable under map insertion order.
9. Implement inspect text/JSON/Mermaid from the same graph DTO. Mermaid labels
   must be escaped safely and output must be deterministic.
10. Generate graph diagnostics without source execution, Docker, network, or
    filesystem writes.
11. Enforce the exact partial-order matrix from TZ.md §9.11. Reject backward
    phase dependencies and test each allowed/forbidden pair.
12. Define `schemas/deployment/v1/schema.json` for the redacted compiled model
    consumed by renderers/runtime. It contains no authoring source locations,
    local absolute paths, x-ui, or secret values.

Required tests:
- linear, diamond, disconnected, fan-in/out, service/task mixed, and large DAGs;
- self-loop, two-node, and long cycles with exact cycle diagnostics;
- missing healthcheck/invalid condition/phase-order violations;
- stable ordering under randomized manifest map order;
- x-ui-only changes leave graph/hash unchanged;
- semantic changes alter the hash;
- Mermaid injection/escaping tests;
- property tests comparing topological order against edge constraints;
- performance fixture for 500 services/tasks.

Non-goals:
- No Compose rendering, artifact writes, bundle, provider calls, rollback,
  continue-on-error, host hooks, API, or UI.

Acceptance:
- All baseline and race checks pass.
- Golden text/JSON/Mermaid outputs are reviewed and stable.
- Repeated graph compilation produces identical bytes and hashes.
- Validate/inspect/graph paths still launch no subprocess and use no network.

Apply all global invariants. Document graph semantics and task idempotency
limits. End with the required phase handoff.
```

---

## P05 — Compose renderer, documentation, `.env.example`, and directory build

### Deliverables

- Typed Docker Compose renderer.
- Virtual artifact tree and safe new-target directory output.
- README, Mermaid, `.env.example`.
- `iacpack build --format directory`.

### Copy/paste prompt

```text
Implement Phase P05: compile the locked IR into a deterministic directory
artifact for the compose-docker/v1 profile.

Prerequisites:
- P04 graph/IR is green.
- Read TZ.md sections 12, 13, and 16.

Outcome:
`iacpack build --file iacpack.yaml --lock iacpack.lock.json --format directory
--output dist/project` creates a deterministic inspectable artifact tree with
typed Compose preview when defaults permit, payload files, generated README,
Mermaid graph, `.env.example`, canonical manifest, deployment document, and
lockfile. It never executes Docker or project code.

Required work:
1. Introduce a virtual artifact tree with normalized paths, modes, content
   hashes, and collision checks. Render fully in memory/stream-safe staging
   before materialization.
2. Implement Compose YAML through typed structures and a deterministic
   serializer. Do not concatenate YAML strings and do not rely on ambient
   Compose interpolation.
3. Map supported services, images/build declarations, argv commands, non-secret
   environment, ports, networks, named volumes, declared resources, healthchecks,
   and dependencies to the documented Compose subset.
4. All host-side paths point only into the managed artifact/deployment tree.
   Container paths remain absolute and are never rewritten.
5. If deploy parameters lack defaults, omit `compose.preview.yaml` and emit a
   documented preview-unavailable record rather than inventing values.
6. Generate README with project metadata, requirements, ports, parameters,
   secret file IDs (never values), services, tasks, update/destroy safety, and
   recovery references. Generate Mermaid from P04 graph.
7. Generate `.env.example` only for non-secret deploy parameters using safe
   defaults/placeholders. Never create `.env`.
8. Copy only lock-recorded declared files and verify each hash while copying.
   Fail on drift or time-of-check/time-of-use changes.
9. Add generated-file headers where valid and clearly mark all output as derived
   and non-authoritative. Source workspace remains untouched.
10. Build to a sibling staging directory, verify the complete tree, and rename
    it to a target that MUST NOT already exist. Directory overwrite/replace is
    unsupported in v1; an existing target fails without modification.
11. Serialize and validate deployment.json against the P04 schema. Apply package
    option defaults/compatibility from TZ.md §9.14; README and `.env.example`
    entries are conditional, not silently unconditional.
12. Include canonical `policy.compile.json` and verify its digest matches the
    lockfile. Compiler commands accept explicit `--policy`; no flag uses the
    immutable default.

Required tests:
- golden artifact trees for minimal and multi-service examples;
- YAML/JSON source equivalence;
- deterministic map/list order and line endings;
- parameterized and no-preview cases;
- secret canary absence across every output byte;
- undeclared-file exclusion and lock drift during copy;
- output path recursion/collision and failed replacement;
- recording/unit renderer validation locally and `docker compose config --quiet`
  on minimum/latest supported versions as a required CI matrix gate.

Non-goals:
- No archive bundle, signatures, Docker apply, shell installer, API, UI, Jinja,
  arbitrary source rewriting, or secret values.

Acceptance:
- Build the reference examples twice in different absolute directories and
  compare normalized tree bytes/modes/hashes.
- Run all Go/race/static/security checks.
- Compose golden files pass internal validation and CI Compose validation.

Apply all global invariants and end with the required phase handoff.
```

---

## P06 — Deterministic bundle, verification, semantic diff, and migration shell

### Deliverables

- `.iacbundle` format v1.
- Safe verifier.
- `build --format bundle`, `verify`, `inspect bundle`, semantic `diff`.
- Explicit schema migration command behavior.

### Copy/paste prompt

```text
Implement Phase P06: deterministic bundle creation and hostile-bundle
verification.

Prerequisites:
- P05 directory artifacts are deterministic.
- Read TZ.md sections 11–14 and security sections 18.1–18.3.

Outcome:
`iacpack build --format bundle` creates a byte-reproducible gzip-compressed tar
with `.iacbundle` extension. `verify` and `inspect bundle` safely parse it
without extracting unsafe content. `diff` compares two explicit semantic
sources. No provider mutation occurs.

Required behavior:
1. Implement bundle format v1 layout and schema. Include bundle.json,
   manifest.canonical.json, deployment.json, lockfile, policy.compile.json, optional preview,
   payload, docs, and config/.env.example.
2. bundle.json records project identity, versions/capabilities, required runtime,
   and path/size/SHA-256 for every other entry; it does not hash itself.
3. Normalize entry order, UTF-8 paths, timestamps, UID/GID, names, modes, gzip
   level/header, generated LF text, and all metadata exactly as TZ.md.
4. Write to a same-filesystem staging file, flush, close, re-open, verify, and
   atomically rename. Existing successful output survives any failure.
5. The verifier must reject absolute/traversal/backslash/NUL paths, duplicates,
   type confusion, unsupported types, invalid modes, links, devices, sparse
   entries, checksum mismatch, manifest mismatch, schema mismatch, excessive
   entry count, individual/expanded size, and compression ratio. Check quotas
   before extraction/materialization.
6. Inspection must stream where practical and must not materialize payload.
7. `verify --output json` returns a versioned result and stable diagnostics.
8. Implement semantic `diff --from ... --to ...` for explicit manifest, lock,
   or bundle sources. Recorded deployment state is added in P09. Report service/
   image/parameter/resource/graph/storage changes and classify destructive/
   irreversible risk. Formatting and x-ui-only changes are non-deployment changes.
9. Implement `manifest migrate` command plumbing. With only v1alpha1 available,
   support explicit same-version canonical output and reject unsupported source/
   target pairs; never silently rewrite in place.
10. Add format reference and compatibility policy.
11. Implement public `render compose <bundle>` with explicit deploy values. It
    writes only to stdout or explicit new output and uses the verified
    deployment document, never authoring YAML.
12. Enforce the exact USTAR/gzip constants, Unicode NFC rules, path limits, and
    default/hard quotas from TZ.md. Apply package-option conditional entries.
13. Check in and validate CLI JSON schemas for build, verify, inspect, render,
    diff, and migrate results.

Required tests:
- byte-identical bundles under changed cwd, timezone, locale, umask, UID,
  temporary path, file creation order, and manifest map order;
- malicious archive corpus: traversal, absolute paths, duplicate entries,
  Unicode/case collisions, links, devices, archive bombs, truncated streams,
  corrupt gzip/tar, forged hashes, and unsupported versions;
- fuzz tar/gzip verifier and bundle schema decoder;
- failed write/verification preserves prior output;
- semantic diff golden tests including x-ui-only and formatting-only changes;
- secret canary scan over compressed and expanded logical content.

Non-goals:
- No signatures yet, OCI image embedding, Docker calls, apply/runtime journal,
  API, UI, or generated installer.

Acceptance:
- Build the same bundle in at least two isolated workspaces and compare SHA-256.
- Verify all valid fixtures and reject every malicious fixture with no panic.
- All checks, race tests, fuzz smoke targets, and documented size limits pass.

Apply all global invariants and end with the required phase handoff.
```

---

## P07 — Runtime planning, operator policy, preflight, and state foundations

### Deliverables

- Bundle-to-deployment resolution.
- Policy evaluation.
- `plan`, `doctor`, state schema, lock, process/provider interfaces.
- No mutating apply yet.

### Copy/paste prompt

```text
Implement Phase P07: mutation-free runtime planning, provider preflight, operator policy,
and durable state foundations.

Prerequisites:
- P06 bundle verifier is hardened.
- Read TZ.md sections 14–18, especially the runtime and default policy.

Outcome:
`iacpack plan bundle.iacbundle` verifies the bundle, resolves explicit deploy
parameters and secret file references, applies policy, and emits a deterministic
redacted action plan. `iacpack doctor` performs read-only Docker/host preflight.
No managed resource is created or changed in this phase.

Required work:
0. Write ADR-008 for journaled apply/compensation and freeze project/instance
   identity, provider slug, state path, lock order, and state migration model.
1. Separate authoring compiler trust boundary from runtime. Runtime consumes
   verified deployment.json and bundle metadata, never reparses authoring YAML.
2. Implement deploy parameter validation and secret-file references. Accept
   `--secret-file id=path`; never accept a value argument or environment secret.
   Require regular no-follow files <=1 MiB and validate metadata/readability
   without reading secret bytes into diagnostics.
3. Verify the embedded compile-policy digest and evaluate a local operator
   policy that can only be stricter. Enforce all default denies from TZ.md.
   Unsupported schema capabilities cannot be unlocked by policy. Accept it only
   through explicit `--operator-policy`; no flag uses the immutable operator default.
4. Build a deterministic redacted plan with action IDs, dependencies, risk,
   reversibility, commit barrier, expected ownership labels, and required
   confirmations.
5. Implement `doctor` checks for supported Linux platform, Docker Engine,
   Compose plugin/version, daemon reachability, permissions, disk capacity,
   architecture, registry/offline requirements, and obvious host-port use.
   Explain the port race limitation.
6. Sanitize provider environment, especially COMPOSE_* and `.env` behavior.
   Also clear DOCKER_HOST/DOCKER_CONTEXT and select only the default local Unix
   socket or an explicit local `--docker-socket`; reject TCP/SSH contexts.
   Registry auth uses only explicit `--docker-config`.
7. Define typed ProcessRunner and Docker Compose provider interfaces using argv
   arrays. Add a recording fake; do not implement mutation methods yet.
8. Implement XDG state path calculation, strict permissions, per-project/
   instance operation lock, versioned state DTO, and state migration interface.
   `plan` must not create persistent state.
9. Define journal action states and crash semantics in ADR; journal writes begin
   in P08.
10. Emit stable JSON/text plan and RFC-compatible diagnostics without secrets.
11. Implement the exact `--instance` default/grammar and provider project slug
    from TZ.md. For apply-like flows, verify bundle metadata before deriving the
    project state path or acquiring its lock.
12. Re-run resolved host-port overlap validation after deploy parameters and add
    the read-only host availability check with an explicit race warning.

Required tests:
- parameter precedence/constraints and missing secret references;
- secret path diagnostics never include contents;
- every default-policy deny, stricter operator policy, and attempted policy
  weakening;
- deterministic plan under reordered inputs;
- fake doctor matrix: missing binary/plugin, bad version, daemon denied, low
  space, occupied port, wrong OS/arch, and success;
- COMPOSE_*/DOCKER_HOST/DOCKER_CONTEXT contamination has no effect;
- concurrent lock behavior using temporary state roots;
- process argv injection tests; no shell invocation;
- plan/doctor make no mutations.

Non-goals:
- No `apply`, image pull, Compose up, task execution, journal mutation, resume,
  destroy, installer, API, or UI.

Acceptance:
- All existing checks and race tests pass.
- Plan JSON contains no absolute secret source path unless explicitly classified
  as local/private output; it never contains secret bytes.
- Recording fake proves zero mutation calls for plan and doctor.

Apply all global invariants and end with the required phase handoff.
```

---

## P08 — Docker apply, health checks, tasks, and durable journal

### Deliverables

- Mutating Docker Compose provider.
- Journaled `apply`.
- Health/readiness waits and container tasks.
- Re-apply idempotency.

### Copy/paste prompt

```text
Implement Phase P08: journaled Docker Compose deployment for verified bundles.

Prerequisites:
- P07 plan, policy, doctor, state, locks, and recording provider are green.
- Read TZ.md sections 15, 16, 18, 20, and 21.

Outcome:
On supported Linux hosts, `iacpack apply bundle.iacbundle` verifies, plans,
confirms, materializes private runtime files, provisions resources, starts
services, waits for readiness, executes declared container tasks, and records a
durable successful state. Re-applying the same bundle is safe and does not
duplicate resources.

Required work:
1. Implement the apply stages in the exact TZ.md order. Acquire the instance
   lock before state mutation.
2. Create a durable journal transaction before each action and record intent,
   start, result, owned resource IDs, reversibility, and redacted failure data.
   Crash between transitions must leave a detectable state.
3. Materialize final Compose from typed deployment data and explicit parameters,
   not shell interpolation. Use a private managed root and restrictive modes.
4. Materialize secret files as 0440 under a 0700 directory, mount read-only, and
   never log/hash their content or place it in structured state. The designated
   secret payload directory is the only persisted exception; follow TZ.md
   source type/size, atomic rotation, resume lifetime, and destroy cleanup rules.
5. Invoke `docker compose` with explicit project name/file and a sanitized
   environment. Use argv arrays. Assign ownership labels to every managed
   resource.
6. Pull/load/build only capabilities approved by the verified plan and policy.
   This image-availability action occurs before any preStart task. Release images
   use digests. Development builds require `--allow-development-build --yes`,
   locked context verification, default network none, and optional expected
   image-ID verification.
7. Start services in plan order; implement bounded readiness waits with
   cancellation, useful progress, and redacted provider diagnostics.
8. Execute preStart, migration, and postDeploy one-shot containers with timeout,
   retry/backoff, dependencies, secrets, and rerun policy. Record task result by
   deployment semantic hash. Never execute `manual` automatically.
9. Mark the irreversible commit barrier before the first migration/irreversible
   task. Do not claim atomic rollback.
10. On failure, stop and perform only documented compensating cleanup for
    reversible resources created by the current run. Never delete volumes,
    pre-existing resources, or host files.
11. On success, atomically set active bundle/deployment hash and health summary.
12. Implement text/JSON `status` for active state and last run.

Required tests:
- fake-provider exact action order for success and every stage failure;
- fault injection before/after every journal transition;
- cancellation/signals during each action;
- secret canaries absent from structured state, logs, journal, process args,
  Compose, and errors; only the designated 0440 secret payload may contain them;
- same-bundle second apply is no-op except `always` tasks;
- oncePerDeployment, x-ui-only repack, and manual task behavior;
- pre-existing/foreign resources and volumes survive all failures;
- health timeout/retry/provider-error cases;
- real Docker E2E reference app in the required provisioned CI job.

Non-goals:
- Full resume/reconcile, cross-bundle update, destroy, thin installer, offline
  image layout, signatures, API, or UI.

Acceptance:
- Fake-provider suite and race tests pass.
- Reference app reaches healthy state, applies twice, and reports status.
- Simulated failures return exit 7 when mutation occurred and exit 6 before it.
- Logs are private and contain actionable recovery information without secrets.

Apply all global invariants. Update runtime/recovery docs honestly about partial
failure and compensation limits. End with the required phase handoff.
```

---

## P09 — Resume, update, destroy, and Linux distribution

### Deliverables

- Reconciliation and resume.
- Safe update and destroy.
- Thin Linux installer distribution.
- Failure/recovery E2E matrix.

### Copy/paste prompt

```text
Implement Phase P09: operational lifecycle completion and Linux distributions.

Prerequisites:
- P08 apply/journal behavior passes fake and real-provider tests.
- Read TZ.md sections 13.3–13.4 and 15.2–15.5.

Outcome:
Operators can inspect/recover interrupted runs, update to a new verified bundle,
destroy managed resources safely, and use a platform distribution containing
the bundle, matching iacpack runtime binary, checksums, and a thin install.sh.

Required work:
1. Implement `resume` by loading journal/state, probing actions marked unknown,
   reconciling owned provider resources, and either safely continuing or
   returning an explicit operator-action diagnostic. Never blindly rerun an
   action with unknown irreversible outcome.
2. Implement cross-bundle update: verify and semantic-diff old/new state,
   display risk, require confirmation for destructive/irreversible changes,
   apply the new plan, and mark it active only after success.
   Update is `apply <new-bundle>` targeting the same `(project, --instance)`;
   no ambiguous separate update command is introduced.
3. Preserve persistent volumes and stable project identity across updates.
   Digest-pinned image changes must be visible in plan/status.
4. Implement `destroy`: remove only resources with matching ownership labels.
   Preserve volumes by default. `--delete-volumes` may delete only managed
   volumes declared `retention: delete`; retained volumes are never deleted by
   iacpack. Require `--delete-volumes --yes` in non-interactive mode and an
   explicit interactive confirmation otherwise. Refuse ambiguous/foreign ownership.
5. Implement stale state/resource diagnostics and a safe dry-run plan for
   resume/update/destroy.
6. Implement Linux amd64/arm64 distribution layout with bundle,
   `bin/iacpack`, SHA-256 checksums, README, and thin `install.sh`.
   Distribution format requires manifest `package.includeRuntime: true`; other
   formats reject/omit runtime exactly as TZ.md §9.14 specifies.
7. `install.sh` contains no orchestration logic: safe shell mode/quoting, locate
   runtime, verify checksum, and exec `iacpack apply` with original arguments.
   It must not invoke sudo, curl, eval, or source arbitrary files.
8. Ensure distribution/runtime version compatibility is verified before apply.
9. Add bounded private log rotation and state backup before metadata migration.
10. Implement the real versioned state migrator with verified backup and
    metadata-only rollback. Add recorded deployment state as a semantic `diff`
    source.
11. Implement `task run <project> <task> --instance ...` for active manual tasks
    through the same policy, lock, journal, timeout, secret, and confirmation
    path. It must reject non-manual tasks unless a future contract says otherwise.
12. Complete `status` with active bundle/deployment semantic hash, health,
    last run, pending/unknown recovery, and owned resource IDs.

Required tests:
- crash at every journal boundary followed by resume;
- unknown reversible versus irreversible action outcomes;
- update success/failure before and after commit barrier;
- same/different bundle task rerun behavior;
- destroy dry-run, normal destroy, deletion of `retention: delete`, refusal to
  delete `retention: retain`, foreign labels, missing resources, and interruption;
- manual task success/failure/interruption and non-manual rejection;
- state migration success, corrupt backup, and metadata rollback;
- volume data survives apply failure, update failure, and default destroy;
- installer ShellCheck, quoting, spaces/non-ASCII paths, checksum mismatch,
  missing runtime, exit-code forwarding, and fake-provider install;
- real Docker E2E: install, interrupt, resume, update, destroy, retained volume.

Non-goals:
- No Windows/PowerShell, Podman, Kubernetes, remote Docker, host hooks, OCI
  image embedding, signing, API, or UI.

Acceptance:
- All baseline, race, ShellCheck, fake-provider, and Docker E2E gates pass.
- Every partial failure gives an exact next command.
- No destructive operation can delete data with a single accidental flag.

Apply all global invariants and end with the required phase handoff.
```

---

## P10 — Offline OCI images, signatures, SBOM, provenance, and release pipeline

### Deliverables

- Offline `.iacbundle`.
- Signature verification policy.
- SBOM/provenance/checksums.
- Reproducible release workflow.

### Copy/paste prompt

```text
Implement Phase P10: supply-chain and offline distribution completion.

Prerequisites:
- P09 operational lifecycle and distributions are green.
- Read TZ.md sections 13.3, 18.4, 22, and v1.0 acceptance criteria.

Outcome:
Release engineers can create online or truly offline distributions. Operators
can verify checksums/signatures/policy before mutation. CI produces reproducible
binaries, schemas, SBOMs, provenance, signatures, and release archives from a
signed tag with least privilege.

Required work:
0. Write ADR-010 for signing/provenance and ADR-013 for offline OCI layout before
   implementation.
1. Implement the named network-enabled command
   `iacpack images fetch --file ... --lock ... --output image-cache/`. Use
   go-containerregistry to fetch only locked digests into a verified deterministic
   OCI cache. `build --embed-images --image-cache ...` remains network-free,
   validates every blob/descriptor against the lock, and embeds the layout.
2. Offline apply verifies all OCI descriptors, imports images without network,
   and proves no registry request occurs. Online behavior remains unchanged.
3. Extend bundle quotas for OCI content without weakening archive-bomb defense.
   Deduplicate blobs by digest deterministically.
4. Add detached Sigstore/cosign signing and offline verification integration.
   Signing is explicit and external to deterministic bundle bytes. Verification
   consumes explicit signature/certificate/proof, a pinned local trusted-root
   snapshot, and a local identity policy; verify/plan/apply make no transparency
   network calls. Unsigned behavior follows operator policy. A signature
   authenticates provenance, not runtime safety.
5. Generate SPDX or CycloneDX SBOM for binaries and shipped web/runtime
   dependencies; add SLSA-compatible provenance and SHA-256 checksum files.
6. Add release automation for supported CLI platforms and Linux runtime
   distributions. Pin actions to full SHAs; default workflow permissions are
   read-only and only the release job receives minimal contents write.
7. Never propose `curl | sh`. Document download, checksum, signature, and
   identity verification before execution.
8. Add dependency/license/vulnerability/secret scanning. Critical/high findings
   block release absent a documented owner/expiry exception.
9. Verify source archive/binary builds are reproducible within documented
   toolchain constraints. Never inject build timestamp into product artifacts.
10. Add a release rehearsal command/workflow that does not publish.

Required tests:
- multi-platform manifest and multi-architecture OCI layout fixtures;
- corrupt/missing/swapped OCI blob and descriptor attacks;
- offline network-denial E2E;
- signed, unsigned, untrusted identity, invalid signing-time certificate,
  invalid Rekor inclusion proof, stale/untrusted root snapshot, and tampered
  artifact verification;
- deterministic online/offline bundle builds;
- release workflow permission assertions and artifact checksum verification;
- SBOM/provenance schema validation.

Non-goals:
- No GitHub publishing from the user-facing application, auto-update daemon,
  Podman, Kubernetes, Windows installer, API, or UI.

Acceptance:
- Clean offline host installs the reference project with network denied.
- Release rehearsal creates the documented artifact matrix and all verification
  commands pass.
- v1.0 acceptance criteria in TZ.md are traceable to automated tests or an
  explicit documented manual gate.

Apply all global invariants and end with the required phase handoff.
```

---

## P11 — Loopback local API and generated client contract

### Deliverables

- OpenAPI 3.1 source.
- Loopback Go server.
- Generated server/client bindings.
- API/CLI equivalence and workspace confinement.

### Copy/paste prompt

```text
Implement Phase P11: the local authoring API used by the future Web UI.

Prerequisites:
- P10 is complete. Runtime code exists, but deployment endpoints must not be exposed.
- Read TZ.md section 17 and all API/security invariants.

Outcome:
`iacpack serve --workspace <dir>` serves a loopback-only API and placeholder
static page. The API loads/saves the configured manifest, validates, inspects
the graph/inputs, previews Compose, and runs compiler builds through the exact
same application services as the CLI.

Required work:
0. Write ADR-011 for loopback API, workspace confinement, session/CSRF model,
   ETag saves, and transport boundaries.
1. Author OpenAPI 3.1 for health, schema, get/put manifest, validate, graph,
   Compose/artifact preview, canonical export, Save As, build start/status/
   cancel/events, manifest events, and inputs endpoints listed in TZ.md. Define
   create-when-missing and conflict responses. OpenAPI is the transport source.
2. Pin exact Node 24 LTS, pnpm 10, and OpenAPI generator versions. Generate Go
   bindings and the TypeScript client under `web/src/api/generated/`; add drift
   checks and a minimal package that typechecks it. Do not duplicate DTOs.
3. Bind only loopback by default. Reject non-loopback binding unless a future
   explicit dangerous mode/ADR exists; do not implement remote mode now.
4. Implement the TZ.md session bootstrap: HttpOnly SameSite=Strict cookie plus
   nonce-protected CSRF meta token, required as `X-Iacpack-CSRF`; keep all tokens
   out of URLs/logs. Use same-origin policy and disable CORS.
5. Fix workspace root at server startup. No request may select arbitrary host
   paths. Fix compile policy at startup through `serve --policy`; requests cannot
   select a policy path. Apply canonical containment and request/body/file/output quotas.
6. Implement manifest GET with ETag/revision and atomic PUT requiring If-Match,
   or If-None-Match:* to create the configured missing manifest. Implement
   workspace-contained no-overwrite Save As and canonical export. Return
   conflict instead of overwriting external changes.
7. Use RFC 9457 Problem Details with the same structured diagnostics/codes as
   CLI. CLI and API must produce byte-equivalent canonical preview/results for
   identical inputs and explicit values.
8. Build jobs use queue size 2, cancellation DELETE, deterministic IDs scoped to
   the process, and SSE with 1,000-event buffer/resync behavior. They may write
   only under an explicit workspace
   output root and must not deploy/execute hooks.
9. Serve embedded static assets through a narrow adapter; a real UI is P12.
10. Add 4 MiB request limits, security headers, method/content-type checks,
    timeouts, and graceful
    shutdown. Browser responses must never include secret values or source
    secret paths.
11. Define versioned manifest-event DTOs (`changed`, `invalid`, `deleted`,
    `permissionDenied`, `resyncRequired`). Filesystem watcher ships in P14.
    but transport/schema compatibility is fixed now.

Required tests:
- OpenAPI validation and generated-code drift;
- CLI/API diagnostics and preview equivalence fixtures;
- traversal, encoded traversal, symlink race, oversized body/file, invalid
  content type, method, token, and CORS/origin cases;
- stale ETag conflict and atomic-save failure;
- bounded queue/cancellation DELETE/SSE reconnect and overflow-resync behavior;
- loopback bind assertion and no deployment route;
- race tests with concurrent reads/saves/builds.

Non-goals:
- No deployment/apply/status/destroy API, authentication service, remote hosted
  mode, WebSocket requirement, graph canvas, or secrets in the browser.

Acceptance:
- All Go/static/race/security checks pass.
- OpenAPI-generated client builds.
- An integration test proves API cannot read/write outside configured workspace.
- API and CLI preview bytes and diagnostics match.

Apply all global invariants and end with the required phase handoff.
```

---

## P12 — First frontend vertical slice

### Deliverables

- Strict React application.
- Manifest load/edit/validate/save/export.
- Forms, diagnostics, preview, artifact tree.
- Accessible two-service workflow.

### Copy/paste prompt

```text
Implement Phase P12: the first usable local Web UI vertical slice.

Prerequisites:
- P11 OpenAPI and generated TypeScript client are green.
- Read TZ.md sections 17 and 19.4.

Outcome:
From `iacpack serve`, a user can load or create a manifest, edit every
schema-known authoring object through forms, validate, preview Compose/artifact
tree, explicitly Save/Save As with revision protection, cancel a build, and
export. A graph canvas is not yet required.

Required work:
1. Use the P11-pinned Node/pnpm versions and generated client; commit pnpm lockfile.
2. Create strict TypeScript React/Vite app. Enable the strictest practical
   compiler/linter rules; no `any`, unsafe assertions, or duplicated API DTOs.
3. Use only the generated OpenAPI client. The browser must not parse local files,
   access arbitrary paths, compile Compose, or reimplement server validation.
4. Implement explicit UI states: initial/loading/loaded, locally dirty,
   validating, invalid, preview stale/loading/current, saving/saved, conflict,
   build running/succeeded/failed, and disconnected.
5. Keep manifest changes in typed domain commands/reducer, not scattered
   component state. Preserve invalid user input long enough to show a useful
   error without corrupting the last valid server model.
6. Implement forms for metadata/target/mode, parameters, secret declarations
   (never values), image refs/development builds, resources/globs, services,
   argv/environment/ports/health/resources/restart, networks, volumes, mounts,
   dependencies, lifecycle tasks/retry/rerun, package options, and basic x-ui.
   Schema-known fields without a specialized control use a safe typed fallback
   and remain unchanged; strict-schema unknown fields remain errors.
7. Debounce preview requests, cancel/ignore obsolete responses, and never show a
   stale preview as current.
8. Display structured diagnostics with severity, source path, field linkage,
   hint, and error summary. Do not expose stack traces.
9. Implement explicit Save using ETag/If-Match; do not auto-save. Export is a
   server-generated canonical file/download. Implement create-if-missing and
   workspace-contained no-overwrite Save As.
10. Meet baseline accessibility: semantic controls, labels, descriptions,
    keyboard order, visible focus, error association, live regions, and no
    color-only meaning.
11. Embed production assets into the Go binary reproducibly; asset generation
    must not inject build times or machine paths.
12. Add visible build cancellation wired to the P11 DELETE endpoint and show
    cancelled separately from failed.

Required tests:
- reducer/domain command unit tests;
- form validation and diagnostic linkage;
- stale/cancelled preview race;
- save success, conflict, server validation, network failure, and retry;
- schema-known but specialized-form-unmodeled field round-trip;
- create, Save As no-overwrite, export, artifact tree, and build cancellation;
- two-service user journey in Playwright;
- keyboard-only workflow and automated accessibility scan;
- deterministic production build/drift check for embedded assets.

Non-goals:
- No React Flow canvas, automatic disk synchronization UI, merge editor,
  deployment controls, secret values, remote hosting, or frontend compiler.

Acceptance:
- pnpm lint, typecheck, unit tests, production build, and Playwright pass.
- Go checks remain green with embedded assets.
- A user completes the documented two-service workflow without editing YAML.

Apply all global invariants and end with the required phase handoff.
```

---

## P13 — Visual dependency and resource designer

### Deliverables

- React Flow canvas.
- Typed nodes/edges and property panel.
- DAG-safe editing.
- `x-ui` positions and semantic round-trip.

### Copy/paste prompt

```text
Implement Phase P13: the visual dependency designer.

Prerequisites:
- P12 forms/preview/save workflow passes.
- P04 graph semantics and API graph endpoint are authoritative.
- Read TZ.md sections 9.11–9.12, 17.1, and v1.1 acceptance criteria.

Outcome:
Users can view and edit service/task dependencies on an accessible React Flow
canvas while using the same property forms and server validation. UI layout is
stored under x-ui and never changes deployment output.

Required work:
1. Define an explicit mapping between manifest services/tasks/resources and
   visual nodes/edges. Dependency edges are distinct from informational network,
   volume, and resource links.
2. Treat server graph/diagnostics as authoritative. Client-side cycle checks may
   improve UX but may not replace validation.
3. Implement selection, pan/zoom, add, connect, disconnect, delete, multi-select,
   fit view, and deterministic optional auto-layout.
4. Reject or clearly revert self-dependencies, duplicate edges, invalid target
   conditions, phase-invalid task edges, and cycles. Preserve the user's context
   and announce the reason accessibly.
5. Use a selection-driven property panel backed by the P12 typed commands.
   One user action must produce exactly one semantic manifest mutation.
6. Store node positions and presentation labels only in x-ui. Do not mix them
   into services or graph semantics. Position-only edits must not trigger a
   changed Compose/bundle semantic hash.
7. Implement presentation categories such as app/database/proxy/cache as UI
   labels only; they cannot change provider behavior.
8. Debounce/cancel graph preview requests and visibly distinguish pending,
   invalid, and current server graph states.
9. Provide keyboard alternatives for every graph operation, meaningful focus
   management, textual dependency list/table, and screen-reader announcements.
10. Preserve unsupported manifest fields and semantic identity on import/export.

Required tests:
- service/task node and dependency mapping;
- add/remove edge updates the correct manifest field exactly once;
- self, duplicate, missing, cycle, health-condition, and phase-invalid edges;
- x-ui-only movement preserves deployment hash and Compose preview;
- import -> layout -> export semantic round-trip;
- deterministic auto-layout from identical graph;
- keyboard graph editing and property panel focus;
- Playwright large/diamond/disconnected/task graph journeys.

Non-goals:
- No deployment controls, arbitrary free-form Compose nodes, provider-specific
  Podman/k3s nodes, collaborative editing, or auto-save.

Acceptance:
- Frontend and backend gates pass.
- Every visual semantic operation has a matching server-validated manifest edit.
- Position-only changes do not alter infrastructure artifacts.

Apply all global invariants and end with the required phase handoff.
```

---

## P14 — External file synchronization, conflict safety, and accessibility completion

### Deliverables

- File watcher and SSE notifications.
- ETag conflict workflow.
- Invalid external edit safety.
- WCAG 2.2 AA completion.

### Copy/paste prompt

```text
Implement Phase P14: safe bidirectional disk awareness and Web UI accessibility
completion.

Prerequisites:
- P13 visual designer is stable.
- P11 ETag saves exist.
- Read TZ.md sections 17.2–17.3, 19.4, and v1.1 acceptance criteria.

Outcome:
Valid external manifest edits are detected and offered to the UI without page
reload. Stale UI saves never overwrite disk changes. Invalid external YAML stays
on disk while the UI preserves the last valid visual model and shows diagnostics.
The complete authoring UI meets WCAG 2.2 AA.

Required work:
1. Add a workspace-confined file watcher adapter. Normalize noisy platform
   events, debounce safely, detect replacement/rename, and avoid self-save loops.
2. Compute content revision from exact file bytes. Send revision/change events
   over SSE with reconnect/last-event semantics and bounded buffering.
3. On a clean UI, allow explicit reload of a valid external edit. On a dirty UI,
   enter a conflict state with choices to review, discard local changes, or
   export local work. Never silently merge or overwrite.
4. If the external file is invalid, keep it untouched, retain the last valid
   visual model, display current disk diagnostics, and disable unsafe Save until
   the user makes an explicit recovery choice.
5. PUT remains atomic and requires If-Match. A watcher event is not permission
   to write.
6. Clearly state that canonical Save may normalize YAML formatting/comments.
   Preserve deployment semantics and x-ui according to the schema.
7. Handle file deletion, recreation, permissions loss, editor atomic-renames,
   rapid event bursts, server reconnect, and manifest root replacement.
8. Complete a WCAG 2.2 AA audit: keyboard behavior, focus order/restoration,
   headings/landmarks, forms/errors, graph textual alternative, contrast,
   reduced motion, zoom/reflow, status announcements, and pointer alternatives.
9. Add user-facing unsaved/saving/saved/external-change/conflict/invalid-disk/
   disconnected status that cannot be conveyed by color alone.
10. Update synchronization and recovery documentation with exact data-loss
    guarantees and limitations.

Required tests:
- watcher normalization and self-event suppression;
- valid external edit, dirty conflict, stale save, invalid YAML, delete/recreate,
  permissions failure, rapid events, reconnect, and two UI sessions;
- no save loop and no silent overwrite;
- last valid model remains usable/read-only during invalid disk state;
- Playwright conflict/recovery journeys;
- automated accessibility tests plus documented manual keyboard/screen-reader/
  zoom/contrast checklist.

Non-goals:
- No CRDT, collaborative merge, cloud sync, comment-preserving YAML guarantee,
  deploy controls, or remote server mode.

Acceptance:
- All Go/UI/race/Playwright gates pass.
- Every v1.1 acceptance criterion in TZ.md has an automated or explicit manual
  verification.
- Simulated concurrent edits cannot lose data silently.

Apply all global invariants and end with the required phase handoff.
```

---

## P15 — Product hardening and release candidate

### Deliverables

- Full traceability matrix.
- Security/performance/reliability hardening.
- Release candidate and reproducibility evidence.
- Complete docs and no hidden deferred blockers.

### Copy/paste prompt

```text
Implement Phase P15: harden the completed product and produce a release
candidate. This is a verification/fix phase, not permission for new features.

Prerequisites:
- P10 runtime/distribution and P14 UI acceptance gates pass.
- Read all of TZ.md, all ADRs, release/security docs, and previous handoffs.

Outcome:
Every v1.0 and v1.1 requirement is traced to implementation and evidence. The
release candidate is reproducible, signed, documented, security-reviewed, load-
tested, failure-tested, and usable from a clean environment.

Required work:
1. Build a requirements traceability matrix mapping every numbered acceptance
   criterion and normative MUST to code, automated test, or documented manual
   verification. Fix gaps; do not mark unverified behavior complete.
2. Run and expand parser/path/archive fuzzing, race tests, fault injection,
   malicious corpus, secret canaries, and provider failure matrix. Add regression
   tests for every defect found.
3. Verify performance targets from TZ.md on the documented 4-vCPU/8-GiB runner.
   Record methodology and raw machine-readable results. Optimize measured
   bottlenecks without weakening safety.
4. Run reproducibility builds across isolated Linux/macOS/Windows CLI builders
   where supported and Linux runtime architectures. Compare hashes and document
   any platform-specific artifact boundary explicitly.
5. Run complete E2E: online/offline build, verify, plan, install, health,
   re-apply, interruption, resume, update, failed migration, status, destroy,
   retained volume, and explicit volume deletion.
6. Perform threat-model review for manifest, workspace, archives, OCI, signatures,
   Docker socket, API, browser, file watcher, secrets, logs, journal, installer,
   and CI supply chain. Resolve all blocker/critical findings.
7. Verify >=90% Go statement/line coverage for core packages and complete the
   explicit decision/scenario matrix for path/archive/journal/cleanup outcomes.
   Do not claim a branch metric unsupported by standard Go tooling. Coverage
   does not replace scenario tests.
8. Validate all schemas/OpenAPI/SBOM/provenance, generated-code drift, dependency
   licenses, vulnerability policy, pinned CI actions, and least permissions.
9. Exercise clean-room Quickstart using only released docs/artifacts. Fix every
   hidden assumption. Verify docs never recommend curl-pipe-shell or unsafe
   secret handling.
10. Prepare changelog, support matrix, known limitations, upgrade/migration,
    recovery, security, reproducibility, artifact format, CLI/API/manifest
    references, and release rehearsal evidence.
11. Remove dead code, stale TODOs, disabled/flaky tests, placeholder docs, and
    accidental future-provider abstractions only when removal is behavior-safe.
12. Do not publish. Produce the release candidate locally and provide commands
    a human can use to verify and then publish through the approved workflow.

Mandatory final gates are executed by their owner (local agent, provisioned CI,
or human release operator) and linked as evidence; unavailable external gates
must be marked pending, never passed:
- formatting, lint, vet, staticcheck, race, unit/integration/E2E, fuzz smoke,
  govulncheck, frontend lint/type/unit/build/Playwright, ShellCheck;
- schema/OpenAPI/generated drift;
- secret/license/dependency/vulnerability scans;
- reproducibility, signature, SBOM, provenance, offline network-denial;
- performance and accessibility checks;
- all traceability rows resolved.

Non-goals:
- No Podman, Kubernetes/k3s, Windows installer, remote/hosted UI, arbitrary host
  hooks, executable plugins, telemetry, or application-side GitHub publishing.

End with a release-candidate handoff containing artifact paths and SHA-256,
signature identities, SBOM/provenance paths, exact checks and results, known
limitations, and the single approved human-controlled publish procedure.
```

---

## P16 — Reviewable Compose/Dockerfile import drafts

### Deliverables

- ADR-016 and TZ amendments for propose → Accept → Save.
- Shared `import propose` app service (Compose primary, Dockerfile secondary).
- CLI `iacpack import propose` with `--output json` schema + goldens.
- `POST /api/v1/import/propose` and UI Import draft panel.
- Synthetic fixtures under `testdata/import/`; negative corpus for policy denials.

### Copy/paste prompt

```text
Implement Phase P16: reviewable Compose/Dockerfile → draft Project import.

Prerequisites:
- P15 RC gates pass. Read TZ.md §10/§17, ADR-016, render/compose emit subset.

Outcome:
Operators can propose a draft Project from a workspace-relative Compose file
(and optional Dockerfiles), review diagnostics, Accept into memory, and Save
explicitly. No silent disk writes. Subset is inverse of compose-docker/v1 emit
plus external networks/volumes.

Required work:
1. App service parse/map Compose subset; Dockerfile stubs; deterministic ids.
2. Diagnostics for unsupported/policy-denied/env_file/interpolation/secret-like.
3. CLI + schema + goldens; OpenAPI + generated client; UI propose/Accept/Save.
4. Playwright happy path and negative refusal visibility.
5. Quickstart + known limitations (no full Compose fidelity; no ambient .env).

Non-goals:
- No silent Save; no apply/deploy from import; no profiles/overrides merge;
  no reading .env; no rewriting source Compose/Dockerfiles; no discover name
  set widening without a separate ADR.
```

Apply all global invariants and end with the required phase handoff.
```

---

## P17 — Deeper Compose import and multi-file onboarding

### Deliverables

- ADR-017 implemented: widened discover patterns; multi-Compose propose;
  structured `refusals[]`; UI multi-select + grouped refusals.
- Discover/CLI/API/UI contracts, schemas, goldens, and Playwright coverage.
- Docs: Quickstart multi-file import; known limitations updated.

### Copy/paste prompt

```text
Implement Phase P17: deeper Compose import and multi-file onboarding (ADR-017).

Prerequisites:
- P16 import propose→Accept→Save works. Read TZ.md §10/§17, ADR-016, ADR-017.

Outcome:
Operators can discover Compose-like files beyond compose.yaml/docker-compose.yml,
propose from multiple workspace-relative Compose paths (deterministic merge with
collision diagnostics), and review structured refusals in CLI/API/UI. Accept and
Save semantics remain as ADR-016. No ambient .env; no silent disk writes from
discover or propose.

Required work:
1. Extend discover well-known patterns (depth ≤ 3): existing names plus
   compose.*.yml|yaml, docker-compose.*.yml|yaml, stack*.yml|yaml. Keep skips for
   hidden/output/secret-like. Update inspect/init discover goldens and API.
2. Extend import propose to accept multiple compose paths (ordered). First-wins
   on id collision with diagnostic + suggested rename; merge networks/volumes/
   images/services deterministically (sorted ids).
3. Add result.refusals[] {path, pointer, code, category} alongside diagnostics.
   Categories: policy | unsupported | secret | path | collision.
4. UI: multi-select discover hits + path chips; refusal groups; keep Propose /
   Accept / Cancel / explicit Save. Playwright: multi-file happy path; collision;
   discover lists stack-like name from fixture.
5. Synthetic fixtures under testdata/import/multi/; no vendor trees checked in.
6. Update Quickstart and KNOWN_LIMITATIONS for widened discover and multi-propose.

Non-goals:
- No profiles/overrides as apply semantics; no .env value load; no auto-Save;
  no apply/deploy from import; no rewriting source Compose/Dockerfiles.
```

Apply all global invariants and end with the required phase handoff.
```

---

## P18 — Secrets and parameters onboarding from import

### Deliverables

- ADR-018: parameter gap panel; secret declaration stubs from env_file /
  secret-like keys / Compose secrets; `.env.example` placeholders only.
- Shared app service used by CLI, API, UI; schemas/goldens; Playwright.
- Secret canaries prove values never enter draft, diagnostics, or examples.

### Copy/paste prompt

```text
Implement Phase P18: secrets and parameters onboarding from import (ADR-018).

Prerequisites:
- P17 multi-file import works. Read ADR-005, ADR-016, ADR-018, TZ secret rules.

Outcome:
After propose/Accept, operators see unresolved parameters and suggested secret
declarations (ids + optional mount targets) without any secret values. They can
apply stubs to the in-memory Project explicitly and Save. Onboarding may write
or refresh .env.example placeholders only — never a populated .env.

Required work:
1. Extend propose (or add import onboard-propose) with parameterGaps[] and
   secretSuggestions[] derived without reading env_file/.env contents.
2. CLI: document flags; --output json schema + goldens; refuse if canary secret
   values would appear in stdout/diagnostics.
3. API: extend POST /api/v1/import/propose (or sibling endpoint) with the same
   payload; regenerate client; drift tests.
4. UI: post-import / Forms panel to review gaps, toggle secret stubs, set
   non-secret parameter defaults only; Apply to memory; Save explicit.
5. Optional authoring helper to generate/update .env.example from declared
   parameters/secrets (placeholders); package/build still must not embed values.
6. Regression: secret canaries in fixtures; Playwright shows stubs not values.

Non-goals:
- No reading or importing env_file/.env values; no browser secret storage;
  no apply/deploy secret injection UI; no weakening ADR-005.
```

Apply all global invariants and end with the required phase handoff.
```

---

## P19 — GitHub-oriented release templates (no auto-publish)

### Deliverables

- ADR-019: `iacpack release init-templates` (exact name as implemented) writes
  opted-in workflow/gitignore/docs templates with pinned actions.
- UI propose→confirm write; examples/release-templates goldens; docs checklist.
- Explicit non-goal enforcement: no gh/push/remote/network publish from app.

### Copy/paste prompt

```text
Implement Phase P19: GitHub-oriented release templates without auto-publish
(ADR-019).

Prerequisites:
- P18 onboarding works. Read TZ §5 non-goals (no auto GitHub publish), ADR-019,
  P15 release docs.

Outcome:
Operators can explicitly generate reviewable CI/release helper files into the
workspace (workflow with pinned actions for validate/lock/build/verify, ignore
snippets, operator checklist). No push, no gh, no remote creation, no publish
job enabled by default.

Required work:
1. Authoring command writes documented paths only; refuse overwrite unless
   explicit force flag with safe semantics; deterministic template bytes from
   examples/release-templates/.
2. Default workflow: checkout + setup-go/node as needed + make check / iacpack
   validate/lock/build/verify; pin action versions/SHAs; no secrets in logs;
   no deploy/publish steps.
3. CLI --output json schema + goldens; API POST for template propose/apply write
   (CSRF; workspace-relative only); UI "Release templates…" preview→confirm.
4. Docs: Quickstart section + docs/iacpack-release.md checklist pointing to
   human-controlled publish (P15); update KNOWN_LIMITATIONS.
5. Tests: write into temp workspace; overwrite refusal; force path; prove no
   network calls; drift on template goldens.

Non-goals:
- No GitHub OAuth; no gh CLI integration; no auto tag/release/push;
  no hosted UI; no telemetry; no unpinned floating action tags.
```

Apply all global invariants and end with the required phase handoff.
```

## 6. Post-v1 decision gates

The following are separate initiatives, not unfinished v1 work:

### Podman

Requires a provider capability ADR, rootless/UID/SELinux model, supported Compose implementation, independent golden fixtures, and conformance CI. Docker outputs must remain unchanged.

### Kubernetes/k3s

Requires a separate provider profile and generator for Deployments/StatefulSets, Services, PVCs, probes, Jobs/init containers, RBAC, namespaces, and k3d/k3s tests. It must not be implemented as an `engine` switch over Compose.

### Windows distribution

Requires a PowerShell 7/runtime ADR, Docker Desktop support matrix, Windows path/process/permission semantics, Authenticode/signing decision, Windows CI, and parity E2E.

### Host hooks and plugins

Require a capability security model, sandbox/signature/trust policy, explicit operator opt-in, versioned plugin API, and a new threat model. They are prohibited until that work is approved.

### Hosted UI

Requires authentication, authorization, tenant isolation, encrypted storage, CSRF/CSP/session design, audit logs, quotas, abuse prevention, privacy policy, deployment architecture, and independent security review.

### Compose import fidelity (ADR-020)

Post-P19 documentation and implementation track **F0–F5** in
[ADR-020](docs/adr/020-compose-import-fidelity-layers.md) and
[compose-import-fidelity.md](docs/reference/compose-import-fidelity.md).
This is not full Compose Spec parity; it closes inverse-emit gaps (image tag
interpolation, `mem_limit`, healthcheck CMD-SHELL, partial `propose.ok`,
`activeProfiles`) without weakening TZ default-deny or reading `.env` at propose.
GPU in manifest requires ADR-020 **F3** (Deploy reservations devices only).

## 7. Definition of a successful implementation

Implementation is complete only when:

- the repository satisfies `TZ.md` rather than merely compiling;
- each phase has an evidence-based handoff;
- schemas, CLI, API, UI, docs, examples, and tests agree;
- deterministic builds are demonstrated, not assumed;
- failure/recovery and destructive operations are tested;
- no secret or undeclared input leaks into output;
- unsupported behavior fails explicitly;
- a clean-room operator can verify and deploy the reference product without hidden knowledge.

## 8. After P19 — v1.1.0 readiness

**Feature delivery:** P00–P19 complete. Product version **`1.1.0`**.

### 8.1. Evidence closed on the preparation host (2026-09-29)

| Gate | Result |
|------|--------|
| `make check` | pass |
| Integration + Go E2E | pass |
| UI / Playwright | pass |
| Linux reproduce-build | pass |
| FULL=1 validate perf | pass (p95 6 ms; see `PERFORMANCE.md`) |
| Threat model reconfirm | done — no blocker/critical |
| WCAG automated + checklist | signed in `ui-a11y-checklist.md` |

### 8.2. Still operator/CI before *public signed* tag

| Gate | Action |
|------|--------|
| Container compose-config / live apply | **Closed** 2026-09-29 via `IACPACK_DOCKER_APPLY=1 make docker-e2e` (`engine=podman` stand-in). Re-run if engine changes. |
| Cross-OS reproduce | Confirm GitHub Actions job `reproduce-multi-os` green |
| Signed publish | Human only — `docs/release/PUBLISH.md` |

### 8.3. Intentional v1 limitations / post-v1

Unchanged: see `KNOWN_LIMITATIONS.md` and §6 (Podman, k8s, Windows installer,
hosted UI, auto GitHub publish, etc.).

