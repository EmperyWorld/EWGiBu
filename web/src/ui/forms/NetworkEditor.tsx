import type { Command } from "../../domain/commands";
import type { Network, Project } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import { StackUsage } from "./StackUsage";

type Props = {
  project: Project;
  networkId: string;
  network: Network;
  dispatch: (cmd: Command) => void;
};

export function NetworkEditor({ project, networkId, network: n, dispatch }: Props) {
  const { t } = useLocale();
  const usedBy = Object.keys(project.spec.services)
    .filter((sid) => (project.spec.services[sid]?.networks ?? []).includes(networkId))
    .sort();
  const external = Boolean(n.external);

  return (
    <div
      className="grid2 network-editor"
      data-testid={`network-${networkId}`}
      data-field-path={`/spec/networks/${networkId}`}
    >
      <label>
        {t("forms.external")}
        <select
          data-testid={`network-external-${networkId}`}
          value={external ? "yes" : "no"}
          onChange={(e) => {
            if (e.target.value === "yes") {
              dispatch({
                type: "upsertNetwork",
                id: networkId,
                value: { external: { name: n.external?.name ?? networkId } },
              });
            } else {
              dispatch({ type: "upsertNetwork", id: networkId, value: {} });
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
            data-testid={`network-external-name-${networkId}`}
            value={n.external?.name ?? ""}
            onChange={(e) =>
              dispatch({
                type: "upsertNetwork",
                id: networkId,
                value: { external: { name: e.target.value } },
              })
            }
          />
        </label>
      ) : (
        <p className="muted span2">{t("forms.network.hint")}</p>
      )}
      <div className="span2">
        <StackUsage
          title={t("stack.usage")}
          noneLabel={t("svc.none")}
          testId={`network-stack-${networkId}`}
          lines={[{ label: t("stack.services"), ids: usedBy }]}
        />
      </div>
    </div>
  );
}
