import type { Command } from "../../domain/commands";
import { parseNodeId } from "../../domain/graphMap";
import type { Project } from "../../domain/project";
import { primaryNodeId, type GraphSelection } from "../../domain/selection";
import { useLocale } from "../../i18n/useLocale";
import { NetworkEditor } from "../forms/NetworkEditor";
import { ResourceEditor } from "../forms/ResourceEditor";
import { ServiceEditor } from "../forms/ServiceEditor";
import { TaskEditor } from "../forms/TaskEditor";
import { VolumeEditor } from "../forms/VolumeEditor";

type Props = {
  project: Project;
  selection: GraphSelection;
  announce: string;
  dispatch: (cmd: Command) => void;
};

function labelForNodeId(id: string): string {
  const parsed = parseNodeId(id);
  return parsed ? `${parsed.kind}:${parsed.name}` : id;
}

export function PropertyPanel({ project, selection, announce, dispatch }: Props) {
  const { t } = useLocale();
  const live = (
    <p className="sr-only" role="status" aria-live="polite">
      {announce}
    </p>
  );

  if (selection.nodeIds.length > 1) {
    return (
      <section className="panel" aria-labelledby="prop-h" data-testid="prop-multi">
        <h2 id="prop-h">{t("prop.multi.title")}</h2>
        {live}
        <p data-testid="prop-multi-count">
          {t("prop.multi.count").replace("{n}", String(selection.nodeIds.length))}
        </p>
        <ul className="prop-multi-list">
          {selection.nodeIds.map((id) => (
            <li key={id}>{labelForNodeId(id)}</li>
          ))}
        </ul>
        {selection.edgeIds.length > 0 ? (
          <p className="muted">
            {t("prop.multi.edges").replace("{n}", String(selection.edgeIds.length))}
          </p>
        ) : null}
        <p className="muted">{t("prop.multi.hint")}</p>
      </section>
    );
  }

  if (selection.nodeIds.length === 0 && selection.edgeIds.length > 0) {
    return (
      <section className="panel" aria-labelledby="prop-h" data-testid="prop-edges">
        <h2 id="prop-h">{t("prop.edges.title")}</h2>
        {live}
        <p>{t("prop.edges.count").replace("{n}", String(selection.edgeIds.length))}</p>
        <p className="muted">{t("prop.edges.hint")}</p>
      </section>
    );
  }

  const selectedId = primaryNodeId(selection);
  if (!selectedId) {
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">{t("prop.title")}</h2>
        <p className="muted">{t("prop.none")}</p>
        {live}
      </section>
    );
  }
  const parsed = parseNodeId(selectedId);
  if (!parsed) {
    return (
      <section className="panel">
        <h2>{t("prop.title")}</h2>
        <p className="error">{t("svc.unknown")}</p>
      </section>
    );
  }
  const { kind, name } = parsed;

  if (kind === "service") {
    const svc = project.spec.services[name];
    if (!svc) return <p className="error">{t("svc.missing")}</p>;
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">
          {t("prop.service")} {name}
        </h2>
        {live}
        <ServiceEditor
          project={project}
          serviceId={name}
          service={svc}
          dispatch={dispatch}
          showCategory
        />
        {(svc.dependsOn?.length ?? 0) > 0 ? (
          <>
            <h3>{t("svc.dependsList")}</h3>
            <ul>
              {(svc.dependsOn ?? []).map((d) => (
                <li key={d.target}>
                  {d.target} ({d.condition ?? "started"}){" "}
                  <button
                    type="button"
                    className="linkish"
                    onClick={() => dispatch({ type: "removeDependency", service: name, target: d.target })}
                  >
                    {t("forms.remove")}
                  </button>
                </li>
              ))}
            </ul>
          </>
        ) : null}
      </section>
    );
  }

  if (kind === "task") {
    const task = project.spec.tasks?.[name];
    if (!task) return <p className="error">{t("prop.task.missing")}</p>;
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">
          {t("prop.task")} {name}
        </h2>
        {live}
        <TaskEditor project={project} taskId={name} task={task} dispatch={dispatch} showCategory />
      </section>
    );
  }

  if (kind === "network") {
    const network = project.spec.networks?.[name] ?? {};
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">
          {t("prop.network")} {name}
        </h2>
        {live}
        <NetworkEditor project={project} networkId={name} network={network} dispatch={dispatch} />
      </section>
    );
  }

  if (kind === "volume") {
    const volume = project.spec.volumes?.[name];
    if (!volume) {
      return (
        <section className="panel" aria-labelledby="prop-h">
          <h2 id="prop-h">
            {t("prop.volume")} {name}
          </h2>
          <p className="error">{t("prop.volume.missing")}</p>
        </section>
      );
    }
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">
          {t("prop.volume")} {name}
        </h2>
        {live}
        <VolumeEditor project={project} volumeId={name} volume={volume} dispatch={dispatch} />
      </section>
    );
  }

  if (kind === "resource") {
    const resource = project.spec.resources?.[name];
    if (!resource) {
      return (
        <section className="panel" aria-labelledby="prop-h">
          <h2 id="prop-h">
            {t("prop.resource")} {name}
          </h2>
          <p className="error">{t("prop.resource.missing")}</p>
        </section>
      );
    }
    return (
      <section className="panel" aria-labelledby="prop-h">
        <h2 id="prop-h">
          {t("prop.resource")} {name}
        </h2>
        {live}
        <ResourceEditor project={project} resourceId={name} resource={resource} dispatch={dispatch} />
      </section>
    );
  }

  return (
    <section className="panel" aria-labelledby="prop-h">
      <h2 id="prop-h">{t("prop.title")}</h2>
      <p className="muted">{t("prop.none")}</p>
    </section>
  );
}
