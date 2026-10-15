import { useRef, useState, type DragEvent, type ChangeEvent } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";
import { useLocale } from "../i18n/useLocale";
import { basenameUpload, classifyUploadName, safeUploadRelPath } from "./uploadPath";
import { isTarGz, isZipFile, uploadWorkspaceFile, uploadWorkspaceTar, uploadWorkspaceTree, uploadWorkspaceZip } from "./workspaceUpload";

export type DropKind = "compose" | "dockerfile" | "auto";

type Props = {
  kind: DropKind;
  disabled?: boolean;
  testId: string;
  onUploaded: (path: string, kind: "compose" | "dockerfile") => void;
  onError: (message: string) => void;
};

async function writeWorkspaceFile(
  path: string,
  content: string,
): Promise<{ ok: boolean; path?: string; detail?: string }> {
  const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
  let attempt = path;
  for (let i = 0; i < 4; i++) {
    const res = await client.saveAsManifest(attempt, content);
    if (res.status === 201 || res.ok) {
      const body = (await res.json().catch(() => ({}))) as { path?: string };
      return { ok: true, path: typeof body.path === "string" ? body.path : attempt };
    }
    if (res.status === 409) {
      const stamp = String(Date.now() % 1_000_000);
      const base = basenameUpload(path);
      attempt = `imports/${stamp}-${base}`;
      continue;
    }
    const body = (await res.json().catch(() => ({}))) as { detail?: string };
    return { ok: false, detail: body.detail ?? `upload failed (${String(res.status)})` };
  }
  return { ok: false, detail: "upload conflict" };
}

export function FileDropUpload({ kind, disabled, testId, onUploaded, onError }: Props) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const dirRef = useRef<HTMLInputElement>(null);
  const zipRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState(false);

  const accept =
    kind === "dockerfile"
      ? ".dockerfile,Dockerfile,Dockerfile.*"
      : kind === "compose"
        ? ".yml,.yaml,.zip,.tar.gz,.tgz,compose.yaml,compose.yml,docker-compose.yaml,docker-compose.yml"
        : ".yml,.yaml,.zip,.tar.gz,.tgz,Dockerfile,.dockerfile,compose.yaml,docker-compose.yaml";

  const ingest = async (files: FileList | File[]) => {
    const list = Array.from(files);
    if (!list.length) return;
    setBusy(true);
    try {
      for (const file of list) {
        const detected = kind === "auto" ? classifyUploadName(file.name) : kind;
        if (isZipFile(file.name) || isTarGz(file.name)) {
          const stamp = String(Date.now() % 1_000_000);
          const target = `imports/extract-${stamp}`;
          const zipRes = isTarGz(file.name)
            ? await uploadWorkspaceTar(target, file, { overwrite: true })
            : await uploadWorkspaceZip(target, file, { overwrite: true });
          if (!zipRes.ok) {
            onError(zipRes.detail ?? t("import.uploadFailed"));
            return;
          }
          const composePath =
            zipRes.written?.find((p) => /\.(ya?ml)$/i.test(p) && !p.includes("docker-compose")) ??
            zipRes.written?.find((p) => /compose|stack|docker-compose/i.test(p)) ??
            zipRes.written?.[0];
          if (!composePath) {
            onError(t("import.uploadZipNoCompose"));
            return;
          }
          onUploaded(composePath, detected === "dockerfile" ? "dockerfile" : "compose");
          continue;
        }
        const rel = safeUploadRelPath(file.name);
        const lower = file.name.toLowerCase();
        const isText =
          lower.endsWith(".yml") ||
          lower.endsWith(".yaml") ||
          lower.includes("dockerfile") ||
          lower.endsWith(".compose");
        if (isText) {
          const text = await file.text();
          const written = await writeWorkspaceFile(rel, text);
          if (!written.ok || !written.path) {
            onError(written.detail ?? t("import.uploadFailed"));
            return;
          }
          onUploaded(written.path, detected);
        } else {
          const up = await uploadWorkspaceFile(`imports/${rel}`, file, { overwrite: false });
          if (!up.ok || !up.written?.[0]) {
            onError(up.detail ?? t("import.uploadFailed"));
            return;
          }
          onUploaded(up.written[0], detected);
        }
      }
    } catch (e) {
      onError(e instanceof Error ? e.message : t("import.uploadFailed"));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setDragging(false);
    if (disabled || busy) return;
    if (e.dataTransfer.files?.length) void ingest(e.dataTransfer.files);
  };

  const onBrowse = (e: ChangeEvent<HTMLInputElement>) => {
    if (e.target.files?.length) void ingest(e.target.files);
  };

  const ingestTree = async (list: FileList | null) => {
    if (!list?.length || disabled || busy) return;
    setBusy(true);
    try {
      const stamp = String(Date.now() % 1_000_000);
      const target = `imports/tree-${stamp}`;
      const res = await uploadWorkspaceTree(target, Array.from(list), {
        overwrite: true,
        stripTopFolder: true,
      });
      if (!res.ok) {
        onError(res.detail ?? t("import.uploadFailed"));
        return;
      }
      const written = res.written ?? [];
      const picks = written.filter((p) => {
        const base = p.split("/").pop()?.toLowerCase() ?? "";
        if (kind === "dockerfile") return base === "dockerfile" || base.endsWith(".dockerfile");
        return base.endsWith(".yml") || base.endsWith(".yaml");
      });
      const chosen = picks.length ? picks : written.slice(0, 1);
      if (!chosen.length) {
        onError(t("import.uploadTreeEmpty"));
        return;
      }
      for (const path of chosen) {
        const base = path.split("/").pop()?.toLowerCase() ?? "";
        const uploadedKind =
          base === "dockerfile" || base.endsWith(".dockerfile") ? "dockerfile" : "compose";
        onUploaded(path, kind === "auto" ? uploadedKind : kind === "dockerfile" ? "dockerfile" : "compose");
      }
    } catch (e) {
      onError(e instanceof Error ? e.message : t("import.uploadFailed"));
    } finally {
      setBusy(false);
      if (dirRef.current) dirRef.current.value = "";
    }
  };

  const ingestZipOnly = async (file: File | undefined) => {
    if (!file) return;
    await ingest([file]);
    if (zipRef.current) zipRef.current.value = "";
  };

  return (
    <div
      className={"import-drop" + (dragging ? " is-dragging" : "")}
      data-testid={testId}
      onDragEnter={(e) => {
        e.preventDefault();
        if (!disabled) setDragging(true);
      }}
      onDragOver={(e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = "copy";
      }}
      onDragLeave={(e) => {
        if (e.currentTarget.contains(e.relatedTarget as Node)) return;
        setDragging(false);
      }}
      onDrop={onDrop}
    >
      <p className="import-drop-hint">{t("import.dropHint")}</p>
      <div className="import-drop-actions">
        <button
          type="button"
          data-testid={testId + "-browse"}
          disabled={disabled || busy}
          onClick={() => inputRef.current?.click()}
        >
          {t("import.uploadFile")}
        </button>
        <button
          type="button"
          data-testid={testId + "-folder"}
          disabled={disabled || busy}
          onClick={() => dirRef.current?.click()}
        >
          {t("import.uploadFolder")}
        </button>
        <button
          type="button"
          data-testid={testId + "-zip"}
          disabled={disabled || busy}
          onClick={() => zipRef.current?.click()}
        >
          {t("import.uploadZip")}
        </button>
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          multiple
          hidden
          data-testid={testId + "-input"}
          onChange={onBrowse}
        />
        <input
          ref={dirRef}
          type="file"
          hidden
          // @ts-expect-error non-standard directory picker
          webkitdirectory=""
          data-testid={testId + "-dir-input"}
          onChange={(e) => {
            void ingestTree(e.target.files);
          }}
        />
        <input
          ref={zipRef}
          type="file"
          accept=".zip,application/zip"
          hidden
          data-testid={testId + "-zip-input"}
          onChange={(e) => {
            void ingestZipOnly(e.target.files?.[0]);
          }}
        />
      </div>
    </div>
  );
}
