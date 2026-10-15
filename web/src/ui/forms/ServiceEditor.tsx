import type { Command } from "../../domain/commands";
import { allowedPrerequisiteIds } from "../../domain/deps";
import { argvText, type Healthcheck, type Port, type Project, type Service } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import { IdChecklist } from "./IdChecklist";
import { EnvMapEditor } from "./EnvMapEditor";
import { MountListEditor } from "./MountListEditor";
import { bytesToMib, mibToBytes } from "./memory";
import {
  hostFromParameter,
  hostIsParameter,
  hostLiteralText,
  parseHostLiteral,
} from "./portHost";
import { UnmodeledFields } from "./UnmodeledFields";
import { KNOWN_SERVICE, mergeUnmodeled } from "./unmodeled";

type Props = {
  project: Project;
  serviceId: string;
  service: Service;
  dispatch: (cmd: Command) => void;
  showCategory?: boolean;
};

function splitArgv(text: string): string[] | undefined {
  const parts = text
    .split(/\s+/)
    .map((s) => s.trim())
    .filter(Boolean);
  return parts.length ? parts : undefined;
}

function patch(dispatch: Props["dispatch"], id: string, svc: Service, next: Partial<Service>) {
  dispatch({ type: "upsertService", id, value: { ...svc, ...next } });
}

export function ServiceEditor({ project, serviceId, service: svc, dispatch, showCategory }: Props) {
  const { t } = useLocale();
  const ports = svc.ports ?? [];
  const mounts = svc.mounts ?? [];
  const hc = svc.healthcheck;
  const xuiNode = project.spec["x-ui"]?.nodes?.[serviceId];
  const volumeIds = Object.keys(project.spec.volumes ?? {}).sort();
  const resourceIds = Object.keys(project.spec.resources ?? {}).sort();
  const networkIds = Object.keys(project.spec.networks ?? {}).sort();
  const secretIds = Object.keys(project.spec.secrets ?? {}).sort();
  const dependTargets = [
    ...new Set([
      ...allowedPrerequisiteIds(project, serviceId),
      ...(svc.dependsOn ?? []).map((d) => d.target),
    ]),
  ].sort();
  const imageIds = Object.keys(project.spec.images).sort();
  const parameterIds = Object.entries(project.spec.parameters ?? {})
    .filter(([, p]) => p.phase === "deploy")
    .map(([id]) => id);
  const deployParams = [...parameterIds].sort();

  const setPort = (index: number, port: Port) => {
    const next = [...ports];
    next[index] = port;
    patch(dispatch, serviceId, svc, { ports: next });
  };

  const addPort = () => {
    patch(dispatch, serviceId, svc, {
      ports: [...ports, { container: 80, host: 8080 + ports.length, bind: "127.0.0.1", protocol: "tcp" }],
    });
  };

  const removePort = (index: number) => {
    const next = ports.filter((_, i) => i !== index);
    patch(dispatch, serviceId, svc, { ports: next.length ? next : undefined });
  };


  const secretRefs = new Set((svc.secrets ?? []).map((s) => s.ref));
  const setSecretRefs = (refs: string[]) => {
    const byRef = new Map((svc.secrets ?? []).map((s) => [s.ref, s] as const));
    const next = refs.map((ref) => byRef.get(ref) ?? { ref, target: `/run/secrets/${ref}` });
    patch(dispatch, serviceId, svc, { secrets: next.length ? next : undefined });
  };

  const setSecretTarget = (ref: string, target: string) => {
    const next = (svc.secrets ?? []).map((s) => (s.ref === ref ? { ...s, target } : s));
    patch(dispatch, serviceId, svc, { secrets: next });
  };

  return (
    <div
      className="grid2 service-editor"
      data-testid={`service-${serviceId}`}
      data-field-path={`/spec/services/${serviceId}`}
    >
      <label>
        {t("svc.image")}
        <select
          data-testid={`service-image-${serviceId}`}
          value={svc.image}
          onChange={(e) => patch(dispatch, serviceId, svc, { image: e.target.value })}
        >
          {imageIds.length === 0 ? <option value="">{t("stack.empty")}</option> : null}
          {imageIds.map((img) => (
            <option key={img} value={img}>
              {img}
            </option>
          ))}
        </select>
      </label>
      <label>
        {t("svc.user")}
        <input
          data-testid={`service-user-${serviceId}`}
          data-field-path={`/spec/services/${serviceId}/user`}
          value={svc.user ?? ""}
          placeholder="1000 or 1000:1001"
          onChange={(e) => patch(dispatch, serviceId, svc, { user: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("svc.environmentFrom")}
        <select
          data-testid={`service-envfrom-${serviceId}`}
          data-field-path={`/spec/services/${serviceId}/environmentFrom`}
          value={svc.environmentFrom ?? ""}
          onChange={(e) =>
            patch(dispatch, serviceId, svc, { environmentFrom: e.target.value || undefined })
          }
        >
          <option value="">{t("stack.empty")}</option>
          {Object.keys(project.spec.services)
            .filter((id) => id !== serviceId)
            .concat(svc.environmentFrom && !project.spec.services[svc.environmentFrom] ? [svc.environmentFrom] : [])
            .sort()
            .map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
        </select>
      </label>
      <label>
        {t("svc.restart")}
        <select
          value={svc.restart ?? "no"}
          onChange={(e) =>
            patch(dispatch, serviceId, svc, { restart: e.target.value as Service["restart"] })
          }
        >
          <option value="no">no</option>
          <option value="always">always</option>
          <option value="on-failure">on-failure</option>
          <option value="unless-stopped">unless-stopped</option>
        </select>
      </label>
      <label className="span2">
        {t("svc.profiles")}
        <input
          data-testid={`service-profiles-${serviceId}`}
          value={(svc.profiles ?? []).join(", ")}
          placeholder="edge, debug"
          onChange={(e) => {
            const profiles = e.target.value
              .split(/[,\s]+/)
              .map((s) => s.trim())
              .filter(Boolean);
            patch(dispatch, serviceId, svc, { profiles: profiles.length ? profiles : undefined });
          }}
        />
      </label>
      {showCategory ? (
        <label className="span2">
          {t("svc.category")}
          <select
            value={xuiNode?.category === "other" ? "custom" : (xuiNode?.category ?? "custom")}
            onChange={(e) => dispatch({ type: "setNodeCategory", id: serviceId, category: e.target.value })}
          >
            <option value="app">app</option>
            <option value="database">database</option>
            <option value="proxy">proxy</option>
            <option value="cache">cache</option>
            <option value="custom">custom</option>
          </select>
        </label>
      ) : null}
      <label className="span2">
        {t("svc.command")}
        <input
          data-testid={`service-command-${serviceId}`}
          value={argvText(svc.command)}
          onChange={(e) => patch(dispatch, serviceId, svc, { command: splitArgv(e.target.value) })}
        />
      </label>
      <label className="span2">
        {t("svc.entrypoint")}
        <input
          data-testid={`service-entrypoint-${serviceId}`}
          value={argvText(svc.entrypoint)}
          onChange={(e) => patch(dispatch, serviceId, svc, { entrypoint: splitArgv(e.target.value) })}
        />
      </label>

      <div className="span2 service-list" data-field-path={`/spec/services/${serviceId}/ports`}>
        <div className="section-head">
          <strong>{t("svc.ports")}</strong>
          <button type="button" data-testid={`service-add-port-${serviceId}`} onClick={addPort}>
            {t("svc.addPort")}
          </button>
        </div>
        {ports.length === 0 ? <p className="muted">{t("svc.none")}</p> : null}
        {ports.map((port, index) => (
          <div
            className="grid2 service-row"
            key={`port-${String(index)}`}
            data-testid={`service-port-row-${serviceId}-${String(index)}`}
          >
            <label>
              {t("svc.hostPort")}
              <select
                aria-label={t("svc.hostPortSource")}
                value={hostIsParameter(port.host) ? "parameter" : "literal"}
                onChange={(e) => {
                  if (e.target.value === "parameter") {
                    setPort(index, {
                      ...port,
                      host: hostFromParameter(deployParams[0] ?? ""),
                    });
                  } else {
                    setPort(index, {
                      ...port,
                      host: parseHostLiteral(hostLiteralText(port.host)) ?? 8080,
                    });
                  }
                }}
              >
                <option value="literal">{t("env.literal")}</option>
                <option value="parameter" disabled={deployParams.length === 0}>
                  {t("env.parameter")}
                </option>
              </select>
            </label>
            {hostIsParameter(port.host) ? (
              <label>
                {t("env.parameter")}
                <select
                  data-testid={index === 0 ? `service-port-${serviceId}` : undefined}
                  value={port.host.parameter}
                  onChange={(e) =>
                    setPort(index, { ...port, host: hostFromParameter(e.target.value) })
                  }
                >
                  {deployParams.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              <label>
                {t("svc.hostPort")}
                <input
                  data-testid={index === 0 ? `service-port-${serviceId}` : undefined}
                  type="number"
                  value={hostLiteralText(port.host)}
                  onChange={(e) =>
                    setPort(index, {
                      ...port,
                      host: parseHostLiteral(e.target.value),
                    })
                  }
                />
              </label>
            )}
            <label>
              {t("svc.containerPort")}
              <input
                type="number"
                value={port.container}
                onChange={(e) => setPort(index, { ...port, container: Number(e.target.value) || 80 })}
              />
            </label>
            <label>
              {t("svc.bind")}
              <input
                value={port.bind ?? "127.0.0.1"}
                onChange={(e) => setPort(index, { ...port, bind: e.target.value })}
              />
            </label>
            <label>
              {t("svc.protocol")}
              <select
                value={port.protocol ?? "tcp"}
                onChange={(e) =>
                  setPort(index, { ...port, protocol: e.target.value as Port["protocol"] })
                }
              >
                <option value="tcp">tcp</option>
                <option value="udp">udp</option>
              </select>
            </label>
            <button type="button" className="linkish" onClick={() => removePort(index)}>
              {t("svc.removePort")}
            </button>
          </div>
        ))}
      </div>

      <IdChecklist
        label={t("svc.depends")}
        testId={`service-depends-${serviceId}`}
        ids={dependTargets}
        selected={(svc.dependsOn ?? []).map((d) => d.target)}
        emptyHint={t("stack.noPrereqs")}
        onChange={(targets) => {
          const prev = new Map((svc.dependsOn ?? []).map((d) => [d.target, d] as const));
          patch(dispatch, serviceId, svc, {
            dependsOn: targets.length
              ? targets.map((target) => prev.get(target) ?? { target, condition: "started" as const })
              : undefined,
          });
        }}
      />
      {(svc.dependsOn?.length ?? 0) > 0 ? (
        <div className="span2 service-list">
          {(svc.dependsOn ?? []).map((d) => (
            <label key={d.target}>
              {t("svc.dependsCondition")} ({d.target})
              <select
                value={d.condition ?? "started"}
                onChange={(e) => {
                  const condition = e.target.value as
                    | "started"
                    | "healthy"
                    | "completedSuccessfully";
                  patch(dispatch, serviceId, svc, {
                    dependsOn: (svc.dependsOn ?? []).map((x) =>
                      x.target === d.target ? { ...x, condition } : x,
                    ),
                  });
                }}
              >
                <option value="started">started</option>
                <option value="healthy">healthy</option>
                <option value="completedSuccessfully">completedSuccessfully</option>
              </select>
            </label>
          ))}
        </div>
      ) : null}

      <IdChecklist
        label={t("svc.networks")}
        testId={`service-networks-${serviceId}`}
        ids={networkIds}
        selected={svc.networks ?? []}
        emptyHint={t("stack.noNetworks")}
        onChange={(networks) =>
          patch(dispatch, serviceId, svc, { networks: networks.length ? networks : undefined })
        }
      />

      <IdChecklist
        label={t("svc.secrets")}
        testId={`service-secrets-${serviceId}`}
        ids={secretIds}
        selected={[...secretRefs].sort()}
        emptyHint={t("stack.noSecrets")}
        onChange={setSecretRefs}
      />
      {(svc.secrets?.length ?? 0) > 0 ? (
        <div className="span2 service-list">
          {(svc.secrets ?? []).map((s) => (
            <label key={s.ref}>
              {t("svc.secretTarget")} ({s.ref})
              <input value={s.target} onChange={(e) => setSecretTarget(s.ref, e.target.value)} />
            </label>
          ))}
        </div>
      ) : null}

      <EnvMapEditor
        label={t("svc.env")}
        hint={t("svc.env.hint")}
        testId={`service-env-${serviceId}`}
        value={svc.environment}
        parameterIds={parameterIds}
        onChange={(environment) => patch(dispatch, serviceId, svc, { environment })}
      />

      <MountListEditor
        mounts={mounts}
        volumeIds={volumeIds}
        resourceIds={resourceIds}
        testIdPrefix={`service-${serviceId}`}
        onChange={(next) => patch(dispatch, serviceId, svc, { mounts: next })}
      />

      <label className="span2">
        {t("svc.health")}
        <input
          data-testid={`service-health-${serviceId}`}
          value={(hc?.command ?? []).join(" ")}
          onChange={(e) => {
            const command = splitArgv(e.target.value);
            const healthcheck: Healthcheck | undefined = command
              ? {
                  command,
                  interval: hc?.interval ?? "10s",
                  timeout: hc?.timeout ?? "3s",
                  retries: hc?.retries ?? 3,
                  startPeriod: hc?.startPeriod,
                }
              : undefined;
            patch(dispatch, serviceId, svc, { healthcheck });
          }}
        />
      </label>
      <label>
        {t("svc.healthInterval")}
        <input
          value={hc?.interval ?? ""}
          disabled={!hc}
          onChange={(e) =>
            hc && patch(dispatch, serviceId, svc, { healthcheck: { ...hc, interval: e.target.value } })
          }
        />
      </label>
      <label>
        {t("svc.healthTimeout")}
        <input
          value={hc?.timeout ?? ""}
          disabled={!hc}
          onChange={(e) =>
            hc && patch(dispatch, serviceId, svc, { healthcheck: { ...hc, timeout: e.target.value } })
          }
        />
      </label>
      <label>
        {t("svc.healthRetries")}
        <input
          type="number"
          value={hc?.retries ?? ""}
          disabled={!hc}
          onChange={(e) =>
            hc &&
            patch(dispatch, serviceId, svc, {
              healthcheck: { ...hc, retries: Number(e.target.value) || 1 },
            })
          }
        />
      </label>
      <label>
        {t("svc.healthStartPeriod")}
        <input
          value={hc?.startPeriod ?? ""}
          disabled={!hc}
          onChange={(e) =>
            hc &&
            patch(dispatch, serviceId, svc, {
              healthcheck: { ...hc, startPeriod: e.target.value || undefined },
            })
          }
        />
      </label>
      <label>
        {t("svc.memory")}
        <input
          data-testid={`service-memory-${serviceId}`}
          type="number"
          min={1}
          step={1}
          value={bytesToMib(svc.resources?.memory)}
          onChange={(e) =>
            patch(dispatch, serviceId, svc, {
              resources: {
                ...svc.resources,
                memory: mibToBytes(e.target.value),
              },
            })
          }
        />
        <span className="muted">{t("svc.memory.unit")}</span>
      </label>
      <label>
        {t("svc.cpus")}
        <input
          value={svc.resources?.cpus ?? ""}
          onChange={(e) =>
            patch(dispatch, serviceId, svc, {
              resources: {
                ...svc.resources,
            cpus: e.target.value || undefined,
          },
        })
      }
    />
  </label>
      <label>
        {t("svc.gpu")}
        <select
          data-testid={`service-gpu-${serviceId}`}
          value={svc.resources?.gpu === undefined || svc.resources.gpu === null ? "" : String(svc.resources.gpu)}
          onChange={(e) => {
            const raw = e.target.value;
            let gpu: number | "all" | "devices-only" | undefined;
            if (raw === "") gpu = undefined;
            else if (raw === "all" || raw === "devices-only") gpu = raw;
            else gpu = Number(raw);
            const resources = { ...svc.resources, gpu };
            if (gpu !== "devices-only") delete resources.ldLibraryPath;
            patch(dispatch, serviceId, svc, { resources });
          }}
        >
          <option value="">{t("stack.empty")}</option>
          {[1, 2, 3, 4, 5, 6, 7, 8].map((n) => (
            <option key={n} value={String(n)}>
              {n}
            </option>
          ))}
          <option value="all">all</option>
          <option value="devices-only">devices-only</option>
        </select>
        <span className="muted">{t("svc.gpu.hint")}</span>
      </label>
      {svc.resources?.gpu === "devices-only" ? (
        <label className="span2">
          {t("svc.ldLibraryPath")}
          <input
            data-testid={`service-ld-${serviceId}`}
            value={svc.resources.ldLibraryPath ?? ""}
            placeholder="/usr/local/cuda/compat"
            onChange={(e) =>
              patch(dispatch, serviceId, svc, {
                resources: { ...svc.resources, gpu: "devices-only", ldLibraryPath: e.target.value || undefined },
              })
            }
          />
        </label>
      ) : null}
      <UnmodeledFields
        testId={`service-unmodeled-${serviceId}`}
        value={svc as Record<string, unknown>}
        known={KNOWN_SERVICE}
        onApply={(extra) =>
          dispatch({
            type: "upsertService",
            id: serviceId,
            value: mergeUnmodeled(svc as Record<string, unknown>, KNOWN_SERVICE, extra) as Service,
          })
        }
      />
    </div>
  );
}
