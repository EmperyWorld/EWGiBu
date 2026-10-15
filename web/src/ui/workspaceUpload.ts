import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";

export type WorkspaceUploadMode = "file" | "tree" | "zip";

export type WorkspaceUploadResult = {
  ok: boolean;
  written?: string[];
  skipped?: string[];
  detail?: string;
};

function toBase64(bytes: Uint8Array): string {
  const chunk = 0x8000;
  let binary = "";
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

/** Drop the selected folder name so files land inside target, not target/folder/… */
export function treeMemberPath(file: File, stripTopFolder: boolean): string {
  const raw = ((file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name).replace(
    /\\/g,
    "/",
  );
  if (!stripTopFolder) return raw;
  const parts = raw.split("/").filter(Boolean);
  if (parts.length <= 1) return parts[0] ?? file.name;
  return parts.slice(1).join("/");
}

async function postUpload(body: Record<string, unknown>): Promise<WorkspaceUploadResult> {
  const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
  const res = await client.workspaceUpload(body);
  const payload = (await res.json()) as {
    ok?: boolean;
    result?: { written?: string[]; skipped?: string[] };
    diagnostics?: { message?: string }[];
    detail?: string;
  };
  if (!payload.ok) {
    const msg =
      payload.detail ??
      payload.diagnostics?.[0]?.message ??
      `upload failed (${String(res.status)})`;
    return { ok: false, detail: msg };
  }
  return {
    ok: true,
    written: payload.result?.written ?? [],
    skipped: payload.result?.skipped ?? [],
  };
}

export async function uploadWorkspaceFile(
  targetPath: string,
  file: File,
  opts?: { overwrite?: boolean },
): Promise<WorkspaceUploadResult> {
  const buf = new Uint8Array(await file.arrayBuffer());
  return postUpload({
    mode: "file",
    target: targetPath,
    overwrite: opts?.overwrite ?? false,
    contentBase64: toBase64(buf),
  });
}

export async function uploadWorkspaceTree(
  targetDir: string,
  files: File[],
  opts?: { overwrite?: boolean; stripTopFolder?: boolean },
): Promise<WorkspaceUploadResult> {
  const strip = opts?.stripTopFolder ?? true;
  const parts: { path: string; contentBase64: string }[] = [];
  for (const f of files) {
    const rel = treeMemberPath(f, strip);
    if (!rel) continue;
    const buf = new Uint8Array(await f.arrayBuffer());
    parts.push({ path: rel, contentBase64: toBase64(buf) });
  }
  return postUpload({
    mode: "tree",
    target: targetDir,
    overwrite: opts?.overwrite ?? false,
    files: parts,
  });
}

export async function uploadWorkspaceZip(
  targetDir: string,
  file: File,
  opts?: { overwrite?: boolean },
): Promise<WorkspaceUploadResult> {
  const buf = new Uint8Array(await file.arrayBuffer());
  return postUpload({
    mode: "zip",
    target: targetDir,
    overwrite: opts?.overwrite ?? false,
    contentBase64: toBase64(buf),
  });
}

export function isZipFile(name: string): boolean {
  return name.toLowerCase().endsWith(".zip");
}

export function isTarGz(name: string): boolean {
  const lower = name.toLowerCase();
  return lower.endsWith(".tar.gz") || lower.endsWith(".tgz");
}

export async function uploadWorkspaceTar(
  targetDir: string,
  file: File,
  opts?: { overwrite?: boolean },
): Promise<WorkspaceUploadResult> {
  const buf = new Uint8Array(await file.arrayBuffer());
  return postUpload({
    mode: "tar",
    target: targetDir,
    overwrite: opts?.overwrite ?? false,
    contentBase64: toBase64(buf),
  });
}
