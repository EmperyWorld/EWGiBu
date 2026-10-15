import type { Command } from "../../domain/commands";
import { allowedPrerequisiteIds, defaultCondition, resolveEntity } from "../../domain/deps";
import { argvText, type Project, type Task } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import { IdChecklist } from "./IdChecklist";
import { MountListEditor } from "./MountListEditor";
import { UnmodeledFields } from "./UnmodeledFields";
import { KNOWN_TASK, mergeUnmodeled } from "./unmodeled";

type Props = {
  project: Project;
  taskId: string;
  task: Task;
  dispatch: (cmd: Command) => void;
  showCategory?: boolean;
};

export function TaskEditor({ project, taskId, task, dispatch, showCategory }: Props) {
  const { t } = useLocale();
  const xuiNode = project.spec["x-ui"]?.nodes?.[taskId];
  const imageIds = Object.keys(project.spec.images).sort();
  const networkIds = Object.keys(project.spec.networks ?? {}).sort();
  const secretIds = Object.keys(project.spec.secrets ?? {}).sort();
  const volumeIds = Object.keys(project.spec.volumes ?? {}).sort();
  const resourceIds = Object.keys(project.spec.resources ?? {}).sort();
  const dependTargets = [
    ...new Set([
      ...allowedPrerequisiteIds(project, taskId),
      ...(task.dependsOn ?? []).map((d) => d.target),
    ]),
  ].sort();

  const patch = (next: Partial<Task>) => {
    dispatch({ type: "upsertTask", id: taskId, value: { ...task, ...next } });
  };

  const secretRefs = new Set((task.secrets ?? []).map((s) => s.ref));

  return (
    <div
      className="grid2 task-editor"
      data-testid={`task-${taskId}`}
      data-field-path={`/spec/tasks/${taskId}`}
    >
      <label>
        {t("forms.phase")}
        <select
          value={task.phase}
          onChange={(e) => patch({ phase: e.target.value as Task["phase"] })}
        >
          <option value="preStart">preStart</option>
          <option value="migration">migration</option>
          <option value="postDeploy">postDeploy</option>
        </select>
      </label>
      <label>
        {t("svc.image")}
        <select
          data-testid={`task-image-${taskId}`}
          value={task.image}
          onChange={(e) => patch({ image: e.target.value })}
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
        {t("forms.rerun")}
        <select
          value={task.rerun ?? "oncePerDeployment"}
          onChange={(e) => patch({ rerun: e.target.value as Task["rerun"] })}
        >
          <option value="always">always</option>
          <option value="oncePerDeployment">oncePerDeployment</option>
          <option value="manual">manual</option>
        </select>
      </label>
      {showCategory ? (
        <label>
          {t("svc.category")}
          <select
            value={xuiNode?.category === "other" ? "custom" : (xuiNode?.category ?? "custom")}
            onChange={(e) => dispatch({ type: "setNodeCategory", id: taskId, category: e.target.value })}
          >
            <option value="app">app</option>
            <option value="database">database</option>
            <option value="proxy">proxy</option>
            <option value="cache">cache</option>
            <option value="custom">custom</option>
          </select>
        </label>
      ) : null}
      <label>
        {t("svc.user")}
        <input
          data-testid={`task-user-${taskId}`}
          value={task.user ?? ""}
          placeholder="1000 or 0"
          onChange={(e) => patch({ user: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("svc.entrypoint")}
        <input
          data-testid={`task-entrypoint-${taskId}`}
          value={argvText(task.entrypoint)}
          onChange={(e) => {
            const entrypoint = e.target.value
              .split(/\s+/)
              .map((s) => s.trim())
              .filter(Boolean);
            patch({ entrypoint: entrypoint.length ? entrypoint : undefined });
          }}
        />
      </label>
      <label className="span2">
        {t("forms.command")}
        <input
          data-testid={`task-command-${taskId}`}
          value={argvText(task.command)}
          onChange={(e) => {
            const command = e.target.value
              .split(/\s+/)
              .map((s) => s.trim())
              .filter(Boolean);
            patch({ command: command.length ? command : undefined });
          }}
        />
      </label>
      <label>
        {t("task.workdir")}
        <input
          value={task.workdir ?? ""}
          onChange={(e) => patch({ workdir: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("task.timeout")}
        <input
          value={task.timeout ?? ""}
          onChange={(e) => patch({ timeout: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("task.retryCount")}
        <input
          type="number"
          value={task.retry?.count ?? ""}
          onChange={(e) => {
            const count = e.target.value === "" ? undefined : Number(e.target.value);
            const next = { ...task.retry, count };
            const empty =
              next.count === undefined &&
              !next.initialBackoff &&
              next.multiplier === undefined &&
              !next.maxBackoff;
            patch({ retry: empty ? undefined : next });
          }}
        />
      </label>
      <label>
        {t("task.retryBackoff")}
        <input
          value={task.retry?.initialBackoff ?? ""}
          onChange={(e) => {
            const initialBackoff = e.target.value || undefined;
            const next = { ...task.retry, initialBackoff };
            const empty =
              next.count === undefined &&
              !next.initialBackoff &&
              next.multiplier === undefined &&
              !next.maxBackoff;
            patch({ retry: empty ? undefined : next });
          }}
        />
      </label>
      <label>
        {t("task.retryMultiplier")}
        <input
          type="number"
          step="0.1"
          value={task.retry?.multiplier ?? ""}
          onChange={(e) => {
            const multiplier = e.target.value === "" ? undefined : Number(e.target.value);
            const next = { ...task.retry, multiplier };
            const empty =
              next.count === undefined &&
              !next.initialBackoff &&
              next.multiplier === undefined &&
              !next.maxBackoff;
            patch({ retry: empty ? undefined : next });
          }}
        />
      </label>
      <label>
        {t("task.retryMaxBackoff")}
        <input
          value={task.retry?.maxBackoff ?? ""}
          onChange={(e) => {
            const maxBackoff = e.target.value || undefined;
            const next = { ...task.retry, maxBackoff };
            const empty =
              next.count === undefined &&
              !next.initialBackoff &&
              next.multiplier === undefined &&
              !next.maxBackoff;
            patch({ retry: empty ? undefined : next });
          }}
        />
      </label>
      <IdChecklist
        label={t("svc.networks")}
        testId={`task-networks-${taskId}`}
        ids={networkIds}
        selected={task.networks ?? []}
        emptyHint={t("stack.noNetworks")}
        onChange={(networks) => patch({ networks: networks.length ? networks : undefined })}
      />
      <IdChecklist
        label={t("svc.depends")}
        testId={`task-depends-${taskId}`}
        ids={dependTargets}
        selected={(task.dependsOn ?? []).map((d) => d.target)}
        emptyHint={t("stack.noPrereqs")}
        onChange={(targets) => {
          const prev = new Map((task.dependsOn ?? []).map((d) => [d.target, d] as const));
          patch({
            dependsOn: targets.length
              ? targets.map((target) => {
                  const existing = prev.get(target);
                  if (existing) return existing;
                  const prereq = resolveEntity(project, target);
                  return {
                    target,
                    condition: prereq ? defaultCondition(prereq) : ("started" as const),
                  };
                })
              : undefined,
          });
        }}
      />
      {(task.dependsOn?.length ?? 0) > 0 ? (
        <div className="span2 service-list">
          {(task.dependsOn ?? []).map((d) => (
            <label key={d.target}>
              {t("svc.dependsCondition")} ({d.target})
              <select
                value={d.condition ?? "started"}
                onChange={(e) => {
                  const condition = e.target.value as
                    | "started"
                    | "healthy"
                    | "completedSuccessfully";
                  patch({
                    dependsOn: (task.dependsOn ?? []).map((x) =>
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
        label={t("svc.secrets")}
        testId={`task-secrets-${taskId}`}
        ids={secretIds}
        selected={[...secretRefs].sort()}
        emptyHint={t("stack.noSecrets")}
        onChange={(refs) => {
          const byRef = new Map((task.secrets ?? []).map((s) => [s.ref, s] as const));
          const next = refs.map((ref) => byRef.get(ref) ?? { ref, target: `/run/secrets/${ref}` });
          patch({ secrets: next.length ? next : undefined });
        }}
      />
      {(task.secrets?.length ?? 0) > 0 ? (
        <div className="span2 service-list">
          {(task.secrets ?? []).map((s) => (
            <label key={s.ref}>
              {t("svc.secretTarget")} ({s.ref})
              <input
                value={s.target}
                onChange={(e) =>
                  patch({
                    secrets: (task.secrets ?? []).map((x) =>
                      x.ref === s.ref ? { ...x, target: e.target.value } : x,
                    ),
                  })
                }
              />
            </label>
          ))}
        </div>
      ) : null}
      <MountListEditor
        mounts={task.mounts ?? []}
        volumeIds={volumeIds}
        resourceIds={resourceIds}
        testIdPrefix={`task-${taskId}`}
        onChange={(mounts) => patch({ mounts })}
      />
      <UnmodeledFields
        testId={`task-unmodeled-${taskId}`}
        value={task as Record<string, unknown>}
        known={KNOWN_TASK}
        onApply={(extra) =>
          dispatch({
            type: "upsertTask",
            id: taskId,
            value: mergeUnmodeled(task as Record<string, unknown>, KNOWN_TASK, extra) as Task,
          })
        }
      />
    </div>
  );
}
