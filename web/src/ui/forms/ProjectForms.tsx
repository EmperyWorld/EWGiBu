import { useState } from "react";
import type { Command } from "../../domain/commands";
import {
  type Database,
  type Image,
  type Mode,
  type Parameter,
  type Project,
  type SecretDecl,
  type Service,
} from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import type { MessageKey } from "../../i18n/messages";
import { NamedMap } from "./NamedMap";
import { ImageEditor } from "./ImageEditor";
import { NetworkEditor } from "./NetworkEditor";
import { ResourceEditor } from "./ResourceEditor";
import { SecretFileTools } from "./SecretFileTools";
import { generateAllSecrets } from "./generateSecrets";
import { ServiceEditor } from "./ServiceEditor";
import { TaskEditor } from "./TaskEditor";
import { VolumeEditor } from "./VolumeEditor";
import { classifySecret, secretHints, type SecretKind } from "./secretKind";

type Props = {
  project: Project;
  dispatch: (cmd: Command) => void;
};

function idList(m: Record<string, unknown> | undefined): string[] {
  return m ? Object.keys(m).sort() : [];
}

export function ProjectForms({ project, dispatch }: Props) {
  const { t } = useLocale();
  const { metadata, spec } = project;
  const [secretKinds, setSecretKinds] = useState<Record<string, SecretKind>>({});
  const [secretNotes, setSecretNotes] = useState<Record<string, string>>({});
  const [secretGenBusy, setSecretGenBusy] = useState(false);
  const [secretBatchNote, setSecretBatchNote] = useState("");
  const secretIds = idList(spec.secrets as Record<string, unknown> | undefined);

  function kindLabel(kind: SecretKind): string {
    const key: MessageKey =
      kind === "port"
        ? "forms.secret.kind.port"
        : kind === "name"
          ? "forms.secret.kind.name"
          : "forms.secret.kind.password";
    return t(key);
  }

  function detectSecret(id: string) {
    const found = classifySecret(secretHints(project, id));
    setSecretKinds((prev) => ({ ...prev, [id]: found.kind }));
    setSecretNotes((prev) => ({
      ...prev,
      [id]: found.confident
        ? t("forms.secret.detected").replace("{kind}", kindLabel(found.kind))
        : t("forms.secret.unsure"),
    }));
  }
  return (
    <div className="forms">
      <section aria-labelledby="meta-h">
        <h2 id="meta-h">{t("forms.meta")}</h2>
        <div className="grid2">
          <label>
            {t("forms.name")}
            <input
              data-testid="meta-name"
              data-field-path="/metadata/name"
              value={metadata.name}
              onChange={(e) => dispatch({ type: "setMetadata", name: e.target.value })}
            />
          </label>
          <label>
            {t("forms.version")}
            <input
              data-testid="meta-version"
              data-field-path="/metadata/version"
              value={metadata.version}
              onChange={(e) => dispatch({ type: "setMetadata", version: e.target.value })}
            />
          </label>
          <label className="span2">
            {t("forms.description")}
            <input
              data-field-path="/metadata/description"
              value={metadata.description ?? ""}
              onChange={(e) => dispatch({ type: "setMetadata", description: e.target.value })}
            />
          </label>
          <label>
            {t("forms.mode")}
            <select
              data-testid="mode"
              data-field-path="/spec/mode"
              value={spec.mode}
              onChange={(e) => dispatch({ type: "setMode", mode: e.target.value as Mode })}
            >
              <option value="development">development</option>
              <option value="release">release</option>
            </select>
          </label>
          <label>
            {t("forms.profile")}
            <input value={spec.target.profile} readOnly />
          </label>
        </div>
      </section>

      <NamedMap
        title={t("forms.parameters")}
        testId="map-parameters"
        suggestPrefix="param"
        ids={idList(spec.parameters as Record<string, unknown> | undefined)}
        onAdd={(id) => {
          const value: Parameter = { phase: "deploy", type: "string", default: "" };
          dispatch({ type: "upsertParameter", id, value });
        }}
        onRemove={(id) => dispatch({ type: "removeParameter", id })}
        render={(id) => {
          const p = spec.parameters?.[id];
          if (!p) return null;
          return (
            <div className="grid2">
              <label>
                {t("forms.phase")}
                <select
                  value={p.phase}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertParameter",
                      id,
                      value: { ...p, phase: e.target.value as Parameter["phase"] },
                    })
                  }
                >
                  <option value="build">build</option>
                  <option value="deploy">deploy</option>
                </select>
              </label>
              <label>
                {t("forms.type")}
                <select
                  value={p.type}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertParameter",
                      id,
                      value: { ...p, type: e.target.value as Parameter["type"] },
                    })
                  }
                >
                  <option value="string">string</option>
                  <option value="integer">integer</option>
                  <option value="number">number</option>
                  <option value="boolean">boolean</option>
                  <option value="enum">enum</option>
                </select>
              </label>
              <label className="span2">
                {t("forms.default")}
                <input
                  value={p.default === undefined ? "" : String(p.default)}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertParameter",
                      id,
                      value: { ...p, default: e.target.value },
                    })
                  }
                />
              </label>
              {p.type === "enum" ? (
                <label className="span2">
                  {t("forms.choices")}
                  <input
                    data-testid={`parameter-choices-${id}`}
                    value={(p.choices ?? []).map(String).join(", ")}
                    onChange={(e) =>
                      dispatch({
                        type: "upsertParameter",
                        id,
                        value: {
                          ...p,
                          choices: e.target.value
                            .split(",")
                            .map((s) => s.trim())
                            .filter(Boolean),
                        },
                      })
                    }
                  />
                </label>
              ) : null}
              <label className="span2">
                {t("forms.description")}
                <input
                  value={p.description ?? ""}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertParameter",
                      id,
                      value: { ...p, description: e.target.value || undefined },
                    })
                  }
                />
              </label>
              {p.type === "integer" || p.type === "number" ? (
                <>
                  <label>
                    {t("forms.minimum")}
                    <input
                      type="number"
                      value={p.minimum ?? ""}
                      onChange={(e) =>
                        dispatch({
                          type: "upsertParameter",
                          id,
                          value: {
                            ...p,
                            minimum: e.target.value === "" ? undefined : Number(e.target.value),
                          },
                        })
                      }
                    />
                  </label>
                  <label>
                    {t("forms.maximum")}
                    <input
                      type="number"
                      value={p.maximum ?? ""}
                      onChange={(e) =>
                        dispatch({
                          type: "upsertParameter",
                          id,
                          value: {
                            ...p,
                            maximum: e.target.value === "" ? undefined : Number(e.target.value),
                          },
                        })
                      }
                    />
                  </label>
                </>
              ) : null}
              {p.type === "string" ? (
                <label className="span2">
                  {t("forms.pattern")}
                  <input
                    value={p.pattern ?? ""}
                    onChange={(e) =>
                      dispatch({
                        type: "upsertParameter",
                        id,
                        value: { ...p, pattern: e.target.value || undefined },
                      })
                    }
                  />
                </label>
              ) : null}
            </div>
          );
        }}
      />

      <p className="muted">{t("forms.secret.hint")}</p>
      <div className="secret-batch-actions">
        <button
          type="button"
          data-testid="secret-detect-all"
          disabled={secretIds.length === 0 || secretGenBusy}
          onClick={() => {
            for (const id of secretIds) detectSecret(id);
          }}
        >
          {t("forms.secret.detectAll")}
        </button>
        <button
          type="button"
          data-testid="secret-generate-all"
          disabled={secretIds.length === 0 || secretGenBusy}
          onClick={() => {
            void (async () => {
              setSecretGenBusy(true);
              try {
                const result = await generateAllSecrets(
                  secretIds.map((id) => ({
                    id,
                    kind: secretKinds[id] ?? classifySecret(secretHints(project, id)).kind,
                  })),
                  (count) => window.confirm(t("forms.secret.replaceAllAsk").replace("{n}", String(count))),
                );
                setSecretBatchNote(
                  t("forms.secret.generateAllDone")
                    .replace("{wrote}", String(result.wrote))
                    .replace("{kept}", String(result.kept))
                    .replace("{failed}", String(result.failed)),
                );
              } catch {
                setSecretBatchNote(t("forms.secret.failed"));
              } finally {
                setSecretGenBusy(false);
              }
            })();
          }}
        >
          {t("forms.secret.generateAll")}
        </button>
      </div>
      {secretBatchNote ? (
        <p className="muted" data-testid="secret-generate-all-status">
          {secretBatchNote}
        </p>
      ) : null}
      <NamedMap
        title={t("forms.secrets")}
        testId="map-secrets"
        suggestPrefix="secret"
        ids={secretIds}
        onAdd={(id) => {
          const value: SecretDecl = { required: true, delivery: "file" };
          dispatch({ type: "upsertSecret", id, value });
        }}
        onRemove={(id) => dispatch({ type: "removeSecret", id })}
        render={(id) => {
          const s = spec.secrets?.[id];
          if (!s) return null;
          return (
            <div className="grid2">
              <label>
                {t("forms.required")}
                <select
                  value={s.required === false ? "no" : "yes"}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertSecret",
                      id,
                      value: { ...s, required: e.target.value === "yes" },
                    })
                  }
                >
                  <option value="yes">{t("forms.yes")}</option>
                  <option value="no">{t("forms.no")}</option>
                </select>
              </label>
              <label>
                {t("forms.delivery")}
                <input value="file" readOnly disabled data-testid={`secret-delivery-${id}`} />
                <span className="muted">{t("forms.delivery.fileOnly")}</span>
              </label>
              <label className="span2">
                {t("forms.description")}
                <input
                  value={s.description ?? ""}
                  onChange={(e) =>
                    dispatch({
                      type: "upsertSecret",
                      id,
                      value: { ...s, description: e.target.value || undefined },
                    })
                  }
                />
              </label>
              <SecretFileTools
                id={id}
                kind={secretKinds[id] ?? classifySecret(secretHints(project, id)).kind}
                note={secretNotes[id]}
                onKind={(kind) => {
                  setSecretKinds((prev) => ({ ...prev, [id]: kind }));
                  setSecretNotes((prev) => ({ ...prev, [id]: "" }));
                }}
                onDetect={() => detectSecret(id)}
              />
            </div>
          );
        }}
      />

      <NamedMap
        title={t("forms.images")}
        testId="map-images"
        suggestPrefix="image"
        ids={idList(spec.images)}
        onAdd={(id) => {
          const value: Image =
            spec.mode === "development"
              ? { build: { context: ".", dockerfile: "Dockerfile", include: ["**"] } }
              : {
                  ref: "docker.io/library/busybox@sha256:1111111111111111111111111111111111111111111111111111111111111111",
                  delivery: "pull",
                };
          dispatch({ type: "upsertImage", id, value });
        }}
        onRemove={(id) => dispatch({ type: "removeImage", id })}
        render={(id) => {
          const img = spec.images[id];
          if (!img) return null;
          return <ImageEditor project={project} imageId={id} image={img} dispatch={dispatch} />;
        }}
      />

      <NamedMap
        title={t("forms.resources")}
        testId="map-resources"
        suggestPrefix="resource"
        ids={idList(spec.resources as Record<string, unknown> | undefined)}
        onAdd={(id) => {
          dispatch({
            type: "upsertResource",
            id,
            value: { source: ".", include: ["**"] },
          });
        }}
        onRemove={(id) => dispatch({ type: "removeResource", id })}
        render={(id) => {
          const r = spec.resources?.[id];
          if (!r) return null;
          return <ResourceEditor project={project} resourceId={id} resource={r} dispatch={dispatch} />;
        }}
      />

      <NamedMap
        title={t("forms.networks")}
        testId="map-networks"
        suggestPrefix="net"
        ids={idList(spec.networks as Record<string, unknown> | undefined)}
        onAdd={(id) => dispatch({ type: "upsertNetwork", id, value: {} })}
        onRemove={(id) => dispatch({ type: "removeNetwork", id })}
        render={(id) => {
          const n = spec.networks?.[id] ?? {};
          return <NetworkEditor project={project} networkId={id} network={n} dispatch={dispatch} />;
        }}
      />

      <NamedMap
        title={t("forms.volumes")}
        testId="map-volumes"
        suggestPrefix="vol"
        ids={idList(spec.volumes as Record<string, unknown> | undefined)}
        onAdd={(id) => dispatch({ type: "upsertVolume", id, value: { retention: "retain" } })}
        onRemove={(id) => dispatch({ type: "removeVolume", id })}
        render={(id) => {
          const v = spec.volumes?.[id];
          if (!v) return null;
          return <VolumeEditor project={project} volumeId={id} volume={v} dispatch={dispatch} />;
        }}
      />

      <NamedMap
        title={t("forms.services")}
        testId="map-services"
        suggestPrefix="svc"
        ids={idList(spec.services)}
        onAdd={(id) => {
          const firstImage = Object.keys(spec.images)[0] ?? "web";
          const value: Service = { image: firstImage, networks: Object.keys(spec.networks ?? {}) };
          dispatch({ type: "upsertService", id, value });
        }}
        onRemove={(id) => dispatch({ type: "removeService", id })}
        render={(id) => {
          const svc = spec.services[id];
          if (!svc) return null;
          return (
            <ServiceEditor project={project} serviceId={id} service={svc} dispatch={dispatch} />
          );
        }}
      />

      <NamedMap
        title={t("forms.databases")}
        testId="map-databases"
        suggestPrefix="db"
        ids={idList(spec.databases as Record<string, unknown> | undefined)}
        onAdd={(id) => {
          const first = Object.keys(spec.services)[0] ?? "postgres";
          const value: Database = {
            on: first,
            name: id,
            ownerFrom: first,
            migrations: "",
            urlParameter: `${id}-database-url`,
          };
          dispatch({ type: "upsertDatabase", id, value });
        }}
        onRemove={(id) => dispatch({ type: "removeDatabase", id })}
        render={(id) => {
          const db = spec.databases?.[id];
          if (!db) return null;
          const services = Object.keys(spec.services).sort();
          const patch = (next: Partial<Database>) =>
            dispatch({ type: "upsertDatabase", id, value: { ...db, ...next } });
          return (
            <div className="grid2" data-testid={`database-${id}`}>
              <label>
                {t("db.on")}
                <select value={db.on} onChange={(e) => patch({ on: e.target.value })}>
                  {services.map((sid) => (
                    <option key={sid} value={sid}>
                      {sid}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {t("db.ownerFrom")}
                <select value={db.ownerFrom} onChange={(e) => patch({ ownerFrom: e.target.value })}>
                  {services.map((sid) => (
                    <option key={sid} value={sid}>
                      {sid}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {t("db.name")}
                <input value={db.name} onChange={(e) => patch({ name: e.target.value })} />
              </label>
              <label>
                {t("db.urlParameter")}
                <input value={db.urlParameter} onChange={(e) => patch({ urlParameter: e.target.value })} />
              </label>
              <label className="span2">
                {t("db.migrations")}
                <input value={db.migrations} onChange={(e) => patch({ migrations: e.target.value })} />
              </label>
            </div>
          );
        }}
      />

      <NamedMap
        title={t("forms.tasks")}
        testId="map-tasks"
        suggestPrefix="task"
        ids={idList(spec.tasks as Record<string, unknown> | undefined)}
        onAdd={(id) => {
          const firstImage = Object.keys(spec.images)[0] ?? "web";
          dispatch({
            type: "upsertTask",
            id,
            value: { phase: "preStart", image: firstImage, command: ["true"], rerun: "oncePerDeployment" },
          });
        }}
        onRemove={(id) => dispatch({ type: "removeTask", id })}
        render={(id) => {
          const task = spec.tasks?.[id];
          if (!task) return null;
          return <TaskEditor project={project} taskId={id} task={task} dispatch={dispatch} />;
        }}
      />

      <section aria-labelledby="pkg-h">
        <h2 id="pkg-h">{t("forms.package")}</h2>
        <label>
          <input
            type="checkbox"
            checked={Boolean(spec.package?.generateReadme)}
            onChange={(e) =>
              dispatch({
                type: "setPackage",
                value: { ...spec.package, generateReadme: e.target.checked },
              })
            }
          />{" "}
          {t("forms.generateReadme")}
        </label>
        <label>
          <input
            type="checkbox"
            checked={Boolean(spec.package?.generateEnvExample)}
            onChange={(e) =>
              dispatch({
                type: "setPackage",
                value: { ...spec.package, generateEnvExample: e.target.checked },
              })
            }
          />{" "}
          {t("forms.generateEnvExample")}
        </label>
        <label>
          <input
            type="checkbox"
            checked={Boolean(spec.package?.includeRuntime)}
            onChange={(e) =>
              dispatch({
                type: "setPackage",
                value: { ...spec.package, includeRuntime: e.target.checked },
              })
            }
          />{" "}
          {t("forms.includeRuntime")}
        </label>
        <label>
          <input
            type="checkbox"
            checked={Boolean(spec.package?.embedImages)}
            onChange={(e) =>
              dispatch({
                type: "setPackage",
                value: { ...spec.package, embedImages: e.target.checked },
              })
            }
          />{" "}
          {t("forms.embedImages")}
        </label>
      </section>
    </div>
  );
}
