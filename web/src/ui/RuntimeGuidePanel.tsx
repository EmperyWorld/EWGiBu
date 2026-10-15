import { useMemo, useState } from "react";
import { buildRuntimeCliScript, useWorkspaceInfo } from "../app/useWorkspaceInfo";
import { useLocale } from "../i18n/useLocale";

type Props = {
  open: boolean;
  onClose: () => void;
  /** Highlight after a successful Save. */
  emphasizeSave?: boolean;
  /** metadata.name — status and destroy address this project, not the bundle path. */
  projectName?: string;
};

export function RuntimeGuidePanel({ open, onClose, emphasizeSave, projectName }: Props) {
  const { t } = useLocale();
  const { info, loading } = useWorkspaceInfo();
  const script = useMemo(
    () => (info ? buildRuntimeCliScript(info, projectName) : ""),
    [info, projectName],
  );
  const [copied, setCopied] = useState(false);

  if (!open) return null;

  const copy = async () => {
    if (!script) return;
    try {
      await navigator.clipboard.writeText(script);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="import-backdrop" role="presentation" data-testid="runtime-guide">
      <div className="import-panel" role="dialog" aria-modal="true" aria-labelledby="runtime-title">
        <header className="import-head">
          <h2 id="runtime-title">{t("runtime.title")}</h2>
          <button type="button" onClick={onClose} data-testid="runtime-close">
            {t("import.close")}
          </button>
        </header>
        <p className="import-hint">{t("runtime.hint")}</p>
        {emphasizeSave ? (
          <p className="runtime-saved-note" role="status">
            {t("runtime.afterSave")}
          </p>
        ) : null}
        {loading ? <p className="muted">{t("loading")}</p> : null}
        {!loading && info ? (
          <>
            <p className="runtime-meta" data-testid="runtime-workspace">
              <strong>{t("runtime.workspace")}</strong> {info.workspace}
              <br />
              <strong>{t("runtime.manifest")}</strong> {info.manifest}
            </p>
            <pre className="runtime-script" data-testid="runtime-script">
              {script}
            </pre>
            <div className="import-actions">
              <button type="button" data-testid="runtime-copy" onClick={() => void copy()} disabled={!script}>
                {copied ? t("runtime.copied") : t("runtime.copy")}
              </button>
            </div>
          </>
        ) : null}
        {!loading && !info ? (
          <p className="error" role="alert">
            {t("runtime.unavailable")}
          </p>
        ) : null}
      </div>
    </div>
  );
}
