import { useState } from "react";
import { IacpackClient } from "../../api/generated/client";
import { readCsrf } from "../../app/csrf";
import type { MessageKey } from "../../i18n/messages";
import { useLocale } from "../../i18n/useLocale";
import type { SecretKind } from "./secretKind";

type Props = {
  id: string;
  kind: SecretKind;
  note?: string;
  onKind: (kind: SecretKind) => void;
  onDetect: () => void;
};

const KINDS: SecretKind[] = ["password", "port", "name"];

function kindKey(kind: SecretKind): MessageKey {
  if (kind === "port") return "forms.secret.kind.port";
  if (kind === "name") return "forms.secret.kind.name";
  return "forms.secret.kind.password";
}

export function SecretFileTools({ id, kind, note, onKind, onDetect }: Props) {
  const { t } = useLocale();
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  async function generate(replace: boolean) {
    setBusy(true);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.generateSecretFile({ id, kind, replace });
      if (res.status === 409 && !replace) {
        const ok = window.confirm(t("forms.secret.replaceAsk").replace("{id}", id));
        if (ok) await generate(true);
        return;
      }
      if (!res.ok) {
        setStatus(t("forms.secret.failed"));
        return;
      }
      const body = (await res.json()) as { result?: { path?: unknown } };
      const path = body.result?.path;
      const expected = `secrets/${id}`;
      if (path !== expected) {
        setStatus(t("forms.secret.failed"));
        return;
      }
      setStatus(t("forms.secret.wrote").replace("{path}", expected));
    } catch {
      setStatus(t("forms.secret.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="secret-file-tools span2">
      <div className="secret-generate-row">
        <label>
          {t("forms.secret.kind")}
          <select
            data-testid={`secret-kind-${id}`}
            value={kind}
            onChange={(e) => onKind(e.target.value as SecretKind)}
          >
            {KINDS.map((item) => (
              <option key={item} value={item}>
                {t(kindKey(item))}
              </option>
            ))}
          </select>
        </label>
        <button type="button" data-testid={`secret-detect-${id}`} onClick={onDetect}>
          {t("forms.secret.detect")}
        </button>
        <button
          type="button"
          data-testid={`secret-generate-${id}`}
          disabled={busy}
          onClick={() => {
            void generate(false);
          }}
        >
          {t("forms.secret.generate")}
        </button>
      </div>
      {note ? <p className="muted">{note}</p> : null}
      {status ? (
        <p className="muted" data-testid={`secret-generate-status-${id}`}>
          {status}
        </p>
      ) : null}
    </div>
  );
}
