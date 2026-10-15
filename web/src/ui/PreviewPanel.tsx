import type { PreviewState } from "../app/types";
import { useLocale } from "../i18n/useLocale";
import { DiagnosticsList } from "./DiagnosticsList";

type Props = {
  preview: PreviewState;
};

export function PreviewPanel({ preview }: Props) {
  const { t } = useLocale();
  return (
    <section className="panel panel-flat" aria-labelledby="preview-heading">
      <h2 id="preview-heading" className="sr-only">
        {t("preview.title")}
      </h2>
      {preview.status === "idle" && <p className="muted">{t("preview.idle")}</p>}
      {preview.status === "stale" && <p className="warn">{t("preview.stale")}</p>}
      {preview.status === "loading" && <p className="muted">{t("preview.loading")}</p>}
      {preview.status === "error" && (
        <div>
          <p className="error" role="alert">
            {preview.message}
          </p>
          <DiagnosticsList diagnostics={preview.diagnostics} />
        </div>
      )}
      {preview.status === "current" && (
        <div className="preview-current">
          <h3>compose.preview.yaml</h3>
          <pre className="codeblock" data-testid="compose-preview" tabIndex={0}>
            {preview.compose || t("preview.empty")}
          </pre>
          <h3>{t("preview.tree")}</h3>
          <ul aria-label={t("preview.files.aria")} data-testid="artifact-tree">
            {preview.files.map((f) => (
              <li key={f}>
                <code>{f}</code>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
