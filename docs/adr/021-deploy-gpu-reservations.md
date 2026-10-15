# ADR-021: Deploy GPU reservations

## Status

Accepted. Implements ADR-020 phase F3. Amends TZ §9.8 and §9.15 only for this shape.

## Context

Docker Compose documents GPU access as
`deploy.resources.reservations.devices` with `capabilities: [gpu]`
([GPU support](https://docs.docker.com/compose/how-tos/gpu-support)).
Service-level `devices:` (including CDI names) is a different mechanism and
stays outside the v1 manifest (TZ default-deny, ADR-020 §8).

## Decision

- Manifest `spec.services.*.resources.gpu` is an integer **1..8** or the string **`all`** (`count: all`).
- Render emits only:

```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: <gpu>
          capabilities: [gpu]
```

- Import maps that Deploy list when `driver` is omitted or `nvidia`, `count` is
  1..8 (not `all`), and `capabilities` is exactly `[gpu]`. `device_ids` and
  `options` are refused. The service stays in the draft.
- Service `devices:` remains `IMPORT.POLICY_DENIED` (warning unless
  `--import-strict`) and the service is excluded. No CDI-to-GPU translation.
- No compile-policy exception file. The schema allowlist is the capability.
  Arbitrary host devices, sockets, and privileged mode stay absent.

## Consequences

- Semantic hash includes `resources.gpu` when set.
- Forms expose the count. Default manifests without `gpu` are unchanged.
- `iacpack render` still emits `count: all`. iacpack does not create CDI specs
  and does not probe GPU hardware.
- Apply on Podman reads host CDI spec directories (`containers.conf`
  `cdi_spec_dirs`, otherwise `/etc/cdi` and `/var/run/cdi`). If no spec declares
  device `nvidia.com/gpu=all`, the compose file written for that apply omits
  the `count: all` reservation. Apply records `APPLY.GPU_UNAVAILABLE` with the
  service id and that device name, then starts the container without it.
  Docker keeps the reservation. If Podman still reports that device as
  unresolvable, apply recreates that service once. If the device is still
  unresolvable, apply drops the reservation and starts the container without it.
- Each `service.start` is
  `compose up -d --no-deps --force-recreate <service>`. One service failure
  stays on that service. Later services still start. A readiness timeout is
  likewise per-service and does not abort the rest of the plan
  ([runtime-apply-v1.md](../reference/runtime-apply-v1.md),
  [graph-v1.md](../reference/graph-v1.md)).

## Related

- [ADR-020](020-compose-import-fidelity-layers.md) §8
- [ADR-007](007-compose-docker-v1-provider.md)
