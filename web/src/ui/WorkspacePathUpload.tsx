import { useRef } from "react";
import { useLocale } from "../i18n/useLocale";
import {
  isTarGz,
  isZipFile,
  uploadWorkspaceFile,
  uploadWorkspaceTar,
  uploadWorkspaceTree,
  uploadWorkspaceZip,
} from "./workspaceUpload";

export type ExpectedPathKind = "file" | "directory" | "buildContext";

type Props = {
  expected: ExpectedPathKind;
  targetPath: string;
  disabled?: boolean;
  testIdPrefix: string;
  onDone: () => void;
  onError: (message: string) => void;
};

export function WorkspacePathUpload({
  expected,
  targetPath,
  disabled,
  testIdPrefix,
  onDone,
  onError,
}: Props) {
  const { t } = useLocale();
  const fileRef = useRef<HTMLInputElement>(null);
  const dirRef = useRef<HTMLInputElement>(null);
  const zipRef = useRef<HTMLInputElement>(null);

  const pickFile = () => fileRef.current?.click();
  const pickDir = () => dirRef.current?.click();
  const pickZip = () => zipRef.current?.click();

  const handleFile = async (file: File | undefined) => {
    if (!file) return;
    const lower = file.name.toLowerCase();
    let res;
    if (expected !== "file" && isZipFile(lower)) {
      res = await uploadWorkspaceZip(targetPath, file, { overwrite: true });
    } else if (expected === "file") {
      res = await uploadWorkspaceFile(targetPath, file, { overwrite: true });
    } else {
      res = await uploadWorkspaceFile(
        targetPath.replace(/\/$/, "") + "/" + file.name.replace(/[^\w.\-+=@]+/g, "_"),
        file,
        { overwrite: true },
      );
    }
    if (!res.ok) {
      onError(res.detail ?? t("import.uploadFailed"));
      return;
    }
    onDone();
  };

  const handleDir = async (list: FileList | null) => {
    if (!list?.length) return;
    const res = await uploadWorkspaceTree(targetPath, Array.from(list), {
      overwrite: true,
      stripTopFolder: true,
    });
    if (!res.ok) {
      onError(res.detail ?? t("import.uploadFailed"));
      return;
    }
    onDone();
  };

  return (
    <span className="import-ws-upload">
      <input
        ref={fileRef}
        type="file"
        hidden
        data-testid={testIdPrefix + "-file-input"}
        onChange={(e) => {
          void handleFile(e.target.files?.[0]);
          e.target.value = "";
        }}
      />
      <input
        ref={dirRef}
        type="file"
        hidden
        // @ts-expect-error webkitdirectory is non-standard but widely supported
        webkitdirectory=""
        data-testid={testIdPrefix + "-dir-input"}
        onChange={(e) => {
          void handleDir(e.target.files);
          e.target.value = "";
        }}
      />
      <input
        ref={zipRef}
        type="file"
        accept=".zip,.tar.gz,.tgz,application/zip,application/gzip"
        hidden
        data-testid={testIdPrefix + "-zip-input"}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (!f) return;
          const run = isTarGz(f.name)
            ? uploadWorkspaceTar(targetPath, f, { overwrite: true })
            : uploadWorkspaceZip(targetPath, f, { overwrite: true });
          void run.then((res) => {
            if (!res.ok) onError(res.detail ?? t("import.uploadFailed"));
            else onDone();
          });
          e.target.value = "";
        }}
      />
      {expected === "file" ? (
        <button type="button" disabled={disabled} data-testid={testIdPrefix + "-upload-file"} onClick={pickFile}>
          {t("import.uploadFile")}
        </button>
      ) : (
        <>
          <button type="button" disabled={disabled} data-testid={testIdPrefix + "-upload-dir"} onClick={pickDir}>
            {t("import.uploadFolder")}
          </button>
          <button type="button" disabled={disabled} data-testid={testIdPrefix + "-upload-zip"} onClick={pickZip}>
            {t("import.uploadArchive")}
          </button>
        </>
      )}
    </span>
  );
}
