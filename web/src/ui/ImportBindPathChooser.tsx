import { bindItemKey, type BindChoiceState, type BindPlanItem } from "./ImportBindPathChooser.types";
import { useLocale } from "../i18n/useLocale";
import { WorkspaceFilePicker } from "./WorkspaceFilePicker";
import { WorkspacePathUpload, type ExpectedPathKind } from "./WorkspacePathUpload";

type Props = {
  items: BindPlanItem[];
  choices: Record<string, BindChoiceState>;
  disabled?: boolean;
  onChange: (key: string, next: BindChoiceState) => void;
  onUploaded: () => void;
  onError: (message: string) => void;
};

function expectedKind(item: BindPlanItem): ExpectedPathKind {
  if (item.kind === "file") return "file";
  if (item.kind === "buildContext") return "buildContext";
  return "directory";
}

export function ImportBindPathChooser({
  items,
  choices,
  disabled,
  onChange,
  onUploaded,
  onError,
}: Props) {
  const { t } = useLocale();

  const missing = items.filter((i) => i.status === "missing");
  if (missing.length === 0) return null;

  return (
    <div className="import-bind-chooser" data-testid="import-bind-chooser">
      <p className="muted">{t("import.bindChooserHint")}</p>
      <table className="import-bind-table">
        <thead>
          <tr>
            <th>{t("import.bindCol.path")}</th>
            <th>{t("import.bindCol.kind")}</th>
            <th>{t("import.bindCol.action")}</th>
          </tr>
        </thead>
        <tbody>
          {missing.map((item) => {
            const key = bindItemKey(item);
            const ch = choices[key] ?? { mode: "stub" as const, path: item.missingPath };
            const tid = key.replace(/[^\w.-]+/g, "_");
            return (
              <tr key={key} data-testid={"import-bind-row-" + tid}>
                <td>
                  <code>{item.missingPath}</code>
                  {item.resourceId ? (
                    <span className="muted import-bind-meta"> {item.resourceId}</span>
                  ) : null}
                  {item.imageId ? (
                    <span className="muted import-bind-meta"> image:{item.imageId}</span>
                  ) : null}
                </td>
                <td>{item.kind}</td>
                <td>
                  <div className="import-bind-actions">
                    <label className="import-bind-radio">
                      <input
                        type="radio"
                        name={`bind-${key}`}
                        checked={ch.mode === "stub"}
                        disabled={disabled}
                        onChange={() => onChange(key, { mode: "stub", path: ch.path })}
                      />
                      {t("import.bindStub")}
                    </label>
                    <label className="import-bind-radio">
                      <input
                        type="radio"
                        name={`bind-${key}`}
                        checked={ch.mode === "existing"}
                        disabled={disabled}
                        onChange={() =>
                          onChange(key, {
                            mode: "existing",
                            path: ch.path || item.source || item.missingPath,
                          })
                        }
                      />
                      {t("import.bindExisting")}
                    </label>
                    {ch.mode === "existing" ? (
                      <input
                        className="import-bind-path-input"
                        data-testid={"import-bind-path-" + tid}
                        disabled={disabled}
                        value={ch.path}
                        placeholder={item.missingPath}
                        onChange={(e) => onChange(key, { mode: "existing", path: e.target.value })}
                      />
                    ) : null}
                    <WorkspaceFilePicker
                      disabled={disabled}
                      onError={onError}
                      onPick={(path) => {
                        onChange(key, { mode: "existing", path });
                        onUploaded();
                      }}
                    />
                    <WorkspacePathUpload
                      expected={expectedKind(item)}
                      targetPath={
                        item.kind === "file" && ch.mode === "existing" && ch.path.trim()
                          ? ch.path.trim()
                          : item.missingPath
                      }
                      disabled={disabled}
                      testIdPrefix={"import-bind-" + tid}
                      onDone={onUploaded}
                      onError={onError}
                    />
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
