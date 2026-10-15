import { useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";
import { useLocale } from "../i18n/useLocale";

type FilePlan = {
  path: string;
  action: string;
  sha256: string;
  bytes: number;
};

type Props = {
  open: boolean;
  onClose: () => void;
};

export function ReleaseTemplatesPanel({ open, onClose }: Props) {
  const { t } = useLocale();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [force, setForce] = useState(false);
  const [files, setFiles] = useState<FilePlan[] | null>(null);
  const [wrote, setWrote] = useState(false);

  if (!open) return null;

  const propose = async () => {
    setBusy(true);
    setError(null);
    setWrote(false);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.releaseTemplatesPropose({ force });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: { files?: FilePlan[] };
        detail?: string;
        diagnostics?: Array<{ message?: string }>;
      };
      if (!body.ok || !body.result?.files) {
        setError(body.detail ?? body.diagnostics?.[0]?.message ?? t("release.proposeFailed"));
        setFiles(body.result?.files ?? null);
        return;
      }
      setFiles(body.result.files);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("release.proposeFailed"));
    } finally {
      setBusy(false);
    }
  };

  const confirmWrite = async () => {
    setBusy(true);
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.releaseTemplatesWrite({ force });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: { files?: FilePlan[]; wrote?: boolean };
        detail?: string;
        diagnostics?: Array<{ message?: string }>;
      };
      if (!body.ok) {
        setError(body.detail ?? body.diagnostics?.[0]?.message ?? t("release.writeFailed"));
        setFiles(body.result?.files ?? files);
        return;
      }
      setFiles(body.result?.files ?? []);
      setWrote(Boolean(body.result?.wrote));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("release.writeFailed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="import-backdrop" role="presentation" data-testid="release-templates">
      <div className="import-panel" role="dialog" aria-modal="true" aria-labelledby="release-title">
        <header className="import-head">
          <h2 id="release-title">{t("release.title")}</h2>
          <button type="button" onClick={onClose} data-testid="release-cancel">
            {t("release.close")}
          </button>
        </header>
        <p className="import-hint">{t("release.hint")}</p>

        <label className="import-check">
          <input
            type="checkbox"
            checked={force}
            onChange={(e) => setForce(e.target.checked)}
            data-testid="release-force"
          />
          <span>{t("release.force")}</span>
        </label>

        <div className="import-actions">
          <button
            type="button"
            data-testid="release-propose"
            disabled={busy}
            onClick={() => {
              void propose();
            }}
          >
            {t("release.propose")}
          </button>
          <button
            type="button"
            data-testid="release-confirm"
            disabled={busy || !files}
            onClick={() => {
              void confirmWrite();
            }}
          >
            {t("release.confirm")}
          </button>
        </div>

        {error ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}

        {wrote ? (
          <p className="muted" data-testid="release-wrote">
            {t("release.wrote")}
          </p>
        ) : null}

        {files ? (
          <ul className="import-diags" data-testid="release-files">
            {files.map((f) => (
              <li key={f.path}>
                <strong>{f.action}</strong> {f.path} ({f.bytes} B)
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </div>
  );
}
