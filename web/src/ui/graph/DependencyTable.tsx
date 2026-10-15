import type { DependsOn, Project } from "../../domain/project";
import { listDependencies } from "../../domain/graphMap";
import { useLocale } from "../../i18n/useLocale";

type Props = {
  project: Project;
  onRemove: (dependent: string, target: string) => void;
  onConditionChange: (
    dependent: string,
    target: string,
    condition: NonNullable<DependsOn["condition"]>,
  ) => void;
  onSelect: (nodeId: string) => void;
};

export function DependencyTable({ project, onRemove, onConditionChange, onSelect }: Props) {
  const { t } = useLocale();
  const rows = listDependencies(project);
  return (
    <details className="dep-drawer" data-testid="dep-drawer">
      <summary id="dep-table-h">
        {t("deps.title")} ({rows.length})
      </summary>
      {rows.length === 0 ? (
        <p className="muted">{t("deps.empty")}</p>
      ) : (
        <table className="dep-table">
          <caption className="sr-only">{t("deps.caption")}</caption>
          <thead>
            <tr>
              <th scope="col">{t("deps.dependent")}</th>
              <th scope="col">{t("deps.prereq")}</th>
              <th scope="col">{t("deps.condition")}</th>
              <th scope="col">{t("deps.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.dependentKind}-${r.dependent}-${r.target}`}>
                <td>
                  <button
                    type="button"
                    className="linkish"
                    onClick={() => onSelect(`${r.dependentKind}:${r.dependent}`)}
                  >
                    {r.dependent}
                    {r.dependentKind === "task" ? " (task)" : ""}
                  </button>
                </td>
                <td>
                  {r.target}
                  {r.targetKind === "task" ? " (task)" : ""}
                </td>
                <td>
                  <select
                    aria-label={`${t("deps.condition")} ${r.dependent}→${r.target}`}
                    data-testid={`dep-condition-${r.dependent}-${r.target}`}
                    value={r.condition}
                    onChange={(e) =>
                      onConditionChange(
                        r.dependent,
                        r.target,
                        e.target.value as NonNullable<DependsOn["condition"]>,
                      )
                    }
                  >
                    <option value="started">started</option>
                    <option value="healthy">healthy</option>
                    <option value="completedSuccessfully">completedSuccessfully</option>
                  </select>
                </td>
                <td>
                  <button type="button" onClick={() => onRemove(r.dependent, r.target)}>
                    {t("deps.disconnect")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </details>
  );
}
