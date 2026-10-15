import { useEffect, useState } from "react";
import { IacpackClient, type WorkspaceInfo } from "../api/generated/client";
import { readCsrf } from "./csrf";

export function useWorkspaceInfo(): { info: WorkspaceInfo | null; loading: boolean } {
  const [info, setInfo] = useState<WorkspaceInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
    void client
      .getWorkspaceInfo()
      .then(async (res) => {
        if (!res.ok || cancelled) return;
        const body = (await res.json()) as WorkspaceInfo;
        if (!cancelled && body.workspace && body.manifest) setInfo(body);
      })
      .catch(() => {
        if (!cancelled) setInfo(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { info, loading };
}

/** Escape path for single-quoted shell literal. */
export function shellQuote(path: string): string {
  return `'${path.replace(/'/g, `'\\''`)}'`;
}

export const defaultImportDraftPath = "imports/import-draft.json";

export function buildImportStubsCliScript(
  info: WorkspaceInfo,
  projectFile = defaultImportDraftPath,
): string {
  const ws = shellQuote(info.workspace);
  const projLocal = shellQuote(projectFile);
  return [
    `# UI: Save draft JSON / Prepare workspace, or copy a local draft into the workspace:`,
    `cd ${ws}`,
    `iacpack import prepare --workspace . --project-file /path/to/draft.json --path ${projLocal} --replace --output json`,
    `# or separately:`,
    `iacpack import write-draft --workspace . --project-file /path/to/draft.json --path ${projLocal} --replace --output json`,
    `iacpack import stub-preview --workspace . --project-file ${projLocal} --output json`,
    `iacpack import resource-stubs --workspace . --project-file ${projLocal} --output json`,
  ].join("\n");
}

export function buildRuntimeCliScript(info: WorkspaceInfo, projectName?: string): string {
  const ws = shellQuote(info.workspace);
  const manifest = shellQuote(info.manifest);
  const bundleRel = `${info.outputRoot.replace(/\/$/, "")}/bundle.iacbundle`;
  const bundle = shellQuote(bundleRel);
  const project = shellQuote((projectName ?? "").trim() || "PROJECT");
  const lines = [
    `# iacpack must be on PATH. After cd, paths are workspace-relative.`,
    `# Container engine: Podman by default (auto), or: export IACPACK_CONTAINER_ENGINE=docker`,
    `# Podman socket (optional): export IACPACK_DOCKER_SOCKET=/run/user/$(id -u)/podman/podman.sock`,
    `cd ${ws}`,
    `iacpack doctor --container-engine podman`,
    `iacpack validate --file ${manifest}`,
    `iacpack lock --file ${manifest}`,
    `iacpack build --file ${manifest} --format bundle --output ${bundle}`,
    `iacpack verify ${bundle}`,
    `iacpack plan ${bundle}`,
    `# Secrets stay files. Directory name = secret id:`,
    `iacpack plan ${bundle} --values values.json --secrets-dir ./secrets`,
    `iacpack apply ${bundle} --values values.json --secrets-dir ./secrets --container-engine podman --instance default --yes`,
    `iacpack status ${project} --instance default`,
    `iacpack destroy ${project} --container-engine podman --instance default --yes`,
  ];
  return lines.join("\n");
}
