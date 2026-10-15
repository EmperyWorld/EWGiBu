import { useState } from "react";
import { useLocale } from "../i18n/useLocale";

type Props = {
  open: boolean;
  defaultPath?: string;
  busy?: boolean;
  onClose: () => void;
  onSubmit: (path: string) => void;
};

export function SaveAsDialog(props: Props) {
  if (!props.open) return null;
  return <SaveAsDialogBody {...props} />;
}

function SaveAsDialogBody({ defaultPath, busy, onClose, onSubmit }: Props) {
  const { t } = useLocale();
  const [path, setPath] = useState(defaultPath ?? "iacpack.copy.yaml");

  const submit = () => {
    const trimmed = path.trim();
    if (!trimmed || trimmed.includes("..") || trimmed.startsWith("/")) return;
    onSubmit(trimmed);
  };

  return (
    <div className="import-backdrop" role="presentation" data-testid="save-as-dialog">
      <div className="import-panel import-panel-narrow" role="dialog" aria-modal="true" aria-labelledby="save-as-title">
        <header className="import-head">
          <h2 id="save-as-title">{t("toolbar.saveAs")}</h2>
          <button type="button" onClick={onClose} data-testid="save-as-cancel">
            {t("import.close")}
          </button>
        </header>
        <p className="import-hint">{t("saveAs.hint")}</p>
        <label>
          {t("saveAs.path")}
          <input
            data-testid="save-as-path"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                submit();
              }
            }}
            autoFocus
          />
        </label>
        <div className="import-actions">
          <button type="button" data-testid="save-as-submit" disabled={busy || !path.trim()} onClick={submit}>
            {t("toolbar.saveAs")}
          </button>
        </div>
      </div>
    </div>
  );
}
