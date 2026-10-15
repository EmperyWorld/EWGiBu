import type { Command } from "../../domain/commands";
import type { Project, Volume } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import { StackUsage } from "./StackUsage";

type Props = {
  project: Project;
  volumeId: string;
  volume: Volume;
  dispatch: (cmd: Command) => void;
};

export function VolumeEditor({ project, volumeId, volume: v, dispatch }: Props) {
  const { t } = useLocale();
  const usedBy = Object.keys(project.spec.services)
    .filter((sid) => (project.spec.services[sid]?.mounts ?? []).some((m) => m.volume === volumeId))
    .sort();
  const external = Boolean(v.external);

  return (
    <div
      className="grid2 volume-editor"
      data-testid={`volume-${volumeId}`}
      data-field-path={`/spec/volumes/${volumeId}`}
    >
      <label>
        {t("forms.external")}
        <select
          data-testid={`volume-external-${volumeId}`}
          value={external ? "yes" : "no"}
          onChange={(e) => {
            if (e.target.value === "yes") {
              dispatch({
                type: "upsertVolume",
                id: volumeId,
                value: { external: { name: v.external?.name ?? volumeId } },
              });
            } else {
              dispatch({ type: "upsertVolume", id: volumeId, value: { retention: "retain" } });
            }
          }}
        >
          <option value="no">{t("forms.no")}</option>
          <option value="yes">{t("forms.yes")}</option>
        </select>
      </label>
      {external ? (
        <label>
          {t("forms.externalName")}
          <input
            data-testid={`volume-external-name-${volumeId}`}
            value={v.external?.name ?? ""}
            onChange={(e) =>
              dispatch({
                type: "upsertVolume",
                id: volumeId,
                value: { external: { name: e.target.value } },
              })
            }
          />
        </label>
      ) : (
        <label>
          {t("forms.retention")}
          <select
            data-testid={`volume-retention-${volumeId}`}
            value={v.retention ?? "retain"}
            onChange={(e) =>
              dispatch({
                type: "upsertVolume",
                id: volumeId,
                value: { retention: e.target.value as "retain" | "delete" },
              })
            }
          >
            <option value="retain">retain</option>
            <option value="delete">delete</option>
          </select>
        </label>
      )}
      <div className="span2">
        <StackUsage
          title={t("stack.usage")}
          noneLabel={t("svc.none")}
          testId={`volume-stack-${volumeId}`}
          lines={[{ label: t("stack.services"), ids: usedBy }]}
        />
      </div>
    </div>
  );
}
