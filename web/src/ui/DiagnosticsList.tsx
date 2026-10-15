import type { Diagnostic } from "../app/types";
import { useLocale } from "../i18n/useLocale";
import { diagnosticField } from "./diagnosticField";

type Props = {
  diagnostics: Diagnostic[];
  onFieldNavigate?: (field: string) => void;
};

export function DiagnosticsList({ diagnostics, onFieldNavigate }: Props) {
  const { t } = useLocale();
  if (diagnostics.length === 0) {
    return <p className="muted">{t("diagnostics.empty")}</p>;
  }
  return (
    <ul className="diagnostics" aria-label={t("diagnostics.aria")} data-testid="diagnostics-list">
      {diagnostics.map((d, i) => {
        const sev = typeof d.severity === "string" ? d.severity : "error";
        const field = diagnosticField(d);
        const code = typeof d.code === "string" ? d.code : "";
        const message = typeof d.message === "string" ? d.message : "Diagnostic";
        const hint = typeof d.hint === "string" ? d.hint : "";
        const source = typeof d.source === "string" ? d.source : "";
        return (
          <li key={`${code}-${field}-${String(i)}`} className={`diag diag-${sev}`}>
            <span className="diag-sev">{sev}</span>
            {code ? <span className="diag-code">{code}</span> : null}
            <span className="diag-msg">{message}</span>
            {field ? (
              <span className="diag-field">
                {" "}
                {t("diagnostics.field")}{" "}
                {onFieldNavigate ? (
                  <button
                    type="button"
                    className="linkish diag-field-link"
                    data-testid={`diag-field-${String(i)}`}
                    onClick={() => onFieldNavigate(field)}
                  >
                    <code>{field}</code>
                  </button>
                ) : (
                  <code>{field}</code>
                )}
              </span>
            ) : null}
            {source ? <span className="diag-source"> @ {source}</span> : null}
            {hint ? (
              <div className="diag-hint">
                {t("diagnostics.hint")} {hint}
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
