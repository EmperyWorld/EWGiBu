import { useMemo, useState } from "react";
import { useLocale } from "../i18n/useLocale";
import { diagIsRebaseHint, diagWantsPrepare } from "./importWizard";

export type ImportDiagnosticRow = {
  code?: string;
  severity?: string;
  message?: string;
  hint?: string;
};

const SEVERITY_ORDER = ["error", "warning", "info"] as const;
const PREVIEW_LIMIT = 8;

type Props = {
  diags: ImportDiagnosticRow[];
  onPrepare?: () => void;
  onGoToPaths?: () => void;
};

export function ImportDiagnosticsPanel({ diags, onPrepare, onGoToPaths }: Props) {
  const { t } = useLocale();
  const [expanded, setExpanded] = useState(false);

  const sorted = useMemo(() => {
    const rank = (s: string | undefined) => {
      const i = SEVERITY_ORDER.indexOf((s ?? "info") as (typeof SEVERITY_ORDER)[number]);
      return i >= 0 ? i : SEVERITY_ORDER.length;
    };
    return [...diags].sort((a, b) => rank(a.severity) - rank(b.severity) || (a.code ?? "").localeCompare(b.code ?? ""));
  }, [diags]);

  if (sorted.length === 0) return null;

  const preview = expanded ? sorted : sorted.slice(0, PREVIEW_LIMIT);
  const hidden = Math.max(0, sorted.length - PREVIEW_LIMIT);

  return (
    <div className="import-diags-wrap" data-testid="import-diagnostics">
      <strong>{t("import.diagnosticsTitle")}</strong>
      <ul className="import-diags">
        {preview.map((d, i) => (
          <li key={"diag-" + String(i) + (d.code ?? "")}>
            <strong>{d.severity ?? "info"}</strong> [{d.code}] {d.message}
            {d.hint ? <span className="import-diag-hint"> — {d.hint}</span> : null}
            {diagIsRebaseHint(d) ? (
              <p className="import-diag-tip muted">{t("import.diagRebaseTip")}</p>
            ) : null}
            {diagWantsPrepare(d) && onPrepare ? (
              <div className="import-diag-actions">
                {onGoToPaths ? (
                  <button type="button" className="import-diag-action" onClick={onGoToPaths}>
                    {t("import.diagGoPaths")}
                  </button>
                ) : null}
                <button type="button" className="import-diag-action" data-testid="import-diag-prepare" onClick={onPrepare}>
                  {t("import.diagPrepare")}
                </button>
              </div>
            ) : null}
          </li>
        ))}
      </ul>
      {!expanded && hidden > 0 ? (
        <button
          type="button"
          className="import-more-refusals"
          data-testid="import-diagnostics-more"
          onClick={() => setExpanded(true)}
        >
          +{hidden} …
        </button>
      ) : null}
    </div>
  );
}
