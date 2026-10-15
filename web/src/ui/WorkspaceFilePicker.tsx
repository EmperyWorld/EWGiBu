import { useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";
import { useLocale } from "../i18n/useLocale";

type Entry = { name: string; path: string; kind: string };

type Props = {
  disabled?: boolean;
  onPick: (path: string) => void;
  onError: (message: string) => void;
};

export function WorkspaceFilePicker({ disabled, onPick, onError }: Props) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const [dir, setDir] = useState("");
  const [entries, setEntries] = useState<Entry[]>([]);
  const [busy, setBusy] = useState(false);

  const load = async (next: string) => {
    setBusy(true);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.listWorkspaceFiles(next || undefined);
      const body = (await res.json()) as {
        ok?: boolean;
        result?: { dir?: string; entries?: Entry[] };
        diagnostics?: { message?: string }[];
        detail?: string;
      };
      if (!body.ok) {
        onError(body.detail ?? body.diagnostics?.[0]?.message ?? t("import.uploadFailed"));
        return;
      }
      setDir(body.result?.dir ?? next);
      setEntries(body.result?.entries ?? []);
      setOpen(true);
    } catch (e) {
      onError(e instanceof Error ? e.message : t("import.uploadFailed"));
    } finally {
      setBusy(false);
    }
  };

  const parent = dir.includes("/") ? dir.slice(0, dir.lastIndexOf("/")) : "";

  return (
    <div className="import-ws-picker">
      <button type="button" disabled={disabled || busy} onClick={() => void load(dir)}>
        {t("import.pickWorkspace")}
      </button>
      {open ? (
        <div className="import-ws-picker-list" data-testid="import-workspace-files">
          <div className="import-ws-picker-nav">
            <button type="button" disabled={busy || dir === ""} onClick={() => void load(parent)}>
              {t("import.pickUp")}
            </button>
            <code>{dir || "."}</code>
          </div>
          <ul>
            {entries.map((e) => (
              <li key={e.path}>
                <button type="button" disabled={disabled} onClick={() => onPick(e.path)}>
                  {e.kind === "dir" ? t("import.pickUseDir") : t("import.pickUseFile")} {e.name}
                </button>
                {e.kind === "dir" ? (
                  <button type="button" disabled={busy} onClick={() => void load(e.path)}>
                    {t("import.pickOpen")}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
