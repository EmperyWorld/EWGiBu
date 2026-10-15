/**
 * Typed fetch client for the iacpack loopback OpenAPI contract.
 * Paths and operationIds must stay aligned with openapi/v1/openapi.yaml
 * (enforced by Go drift tests and scripts/check-drift.mjs).
 */

export type Problem = {
  type: string;
  title: string;
  status: number;
  detail?: string;
  diagnostics?: unknown[];
};

export type CommandResult = {
  ok: boolean;
  result: unknown;
  diagnostics: unknown[];
};

export type SaveResult = {
  path: string;
  etag: string;
  revision: string;
};

export type WorkspaceInfo = {
  workspace: string;
  manifest: string;
  outputRoot: string;
};

export type BuildJob = {
  id: string;
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  output?: string;
  diagnostics?: unknown[];
};

export type ClientOptions = {
  baseUrl: string;
  csrf: string;
  credentials?: RequestCredentials;
  fetch?: typeof fetch;
};

const paths = {
  getHealth: "/api/v1/health",
  getWorkspaceInfo: "/api/v1/workspace",
  listProjects: "/api/v1/projects",
  createProject: "/api/v1/projects",
  selectProject: "/api/v1/projects/select",
  getManifestSchema: "/api/v1/schema/manifest",
  getManifest: "/api/v1/manifest",
  putManifest: "/api/v1/manifest",
  saveAsManifest: "/api/v1/manifest/save-as",
  workspaceUpload: "/api/v1/workspace/upload",
  listWorkspaceFiles: "/api/v1/workspace/files",
  exportManifest: "/api/v1/export/manifest",
  validateManifest: "/api/v1/validate",
  inspectGraph: "/api/v1/inspect/graph",
  previewCompose: "/api/v1/preview/compose",
  previewArtifacts: "/api/v1/preview/artifacts",
  lockManifest: "/api/v1/lock",
  startBuild: "/api/v1/builds",
  getBuild: "/api/v1/builds/{id}",
  cancelBuild: "/api/v1/builds/{id}",
  buildEvents: "/api/v1/builds/{id}/events",
  manifestEvents: "/api/v1/manifest/events",
  listInputs: "/api/v1/inputs",
  discoverWorkspace: "/api/v1/discover",
  importPropose: "/api/v1/import/propose",
  importOnboard: "/api/v1/import/onboard",
  importEnvExample: "/api/v1/import/env-example",
  importStubPreview: "/api/v1/import/stub-preview",
  importPrepare: "/api/v1/import/prepare",
  importWriteDraft: "/api/v1/import/write-draft",
  importResourceStubs: "/api/v1/import/resource-stubs",
  releaseTemplatesPropose: "/api/v1/release/templates/propose",
  releaseTemplatesWrite: "/api/v1/release/templates/write",
  generateSecretFile: "/api/v1/secrets/generate",
} as const;

export type OperationId = keyof typeof paths;

export function operationPaths(): Readonly<Record<OperationId, string>> {
  return paths;
}

export class IacpackClient {
  private readonly baseUrl: string;
  private readonly csrf: string;
  private readonly credentials: RequestCredentials;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: ClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/$/, "");
    this.csrf = opts.csrf;
    this.credentials = opts.credentials ?? "same-origin";
    // Preserve Window binding; bare `fetch` as a value throws Illegal invocation.
    this.fetchImpl = opts.fetch ?? ((input, init) => globalThis.fetch(input, init));
  }

  private async request(method: string, path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    if (method !== "GET" && method !== "HEAD") {
      headers.set("X-Iacpack-CSRF", this.csrf);
    }
    return this.fetchImpl(this.baseUrl + path, {
      ...init,
      method,
      headers,
      credentials: this.credentials,
    });
  }

  getHealth(): Promise<Response> {
    return this.request("GET", paths.getHealth);
  }

  getWorkspaceInfo(): Promise<Response> {
    return this.request("GET", paths.getWorkspaceInfo);
  }

  listProjects(): Promise<Response> {
    return this.request("GET", paths.listProjects);
  }

  createProject(name: string): Promise<Response> {
    return this.request("POST", paths.createProject, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
  }

  selectProject(id: string): Promise<Response> {
    return this.request("POST", paths.selectProject, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id }),
    });
  }

  getManifestSchema(): Promise<Response> {
    return this.request("GET", paths.getManifestSchema);
  }

  getManifest(): Promise<Response> {
    return this.request("GET", paths.getManifest);
  }

  putManifest(body: string, headers: { ifMatch?: string; ifNoneMatch?: string }): Promise<Response> {
    const h = new Headers({ "Content-Type": "application/yaml" });
    if (headers.ifMatch) h.set("If-Match", headers.ifMatch);
    if (headers.ifNoneMatch) h.set("If-None-Match", headers.ifNoneMatch);
    return this.request("PUT", paths.putManifest, { headers: h, body });
  }

  saveAsManifest(path: string, content: string): Promise<Response> {
    return this.request("POST", paths.saveAsManifest, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path, content }),
    });
  }

  listWorkspaceFiles(dir?: string): Promise<Response> {
    const q = dir ? `?dir=${encodeURIComponent(dir)}` : "";
    return this.request("GET", paths.listWorkspaceFiles + q);
  }

  workspaceUpload(body: Record<string, unknown>): Promise<Response> {
    return this.request("POST", paths.workspaceUpload, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  exportManifest(): Promise<Response> {
    return this.request("POST", paths.exportManifest, {
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
  }

  validateManifest(body: unknown = {}): Promise<Response> {
    return this.request("POST", paths.validateManifest, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  inspectGraph(): Promise<Response> {
    return this.request("POST", paths.inspectGraph, {
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
  }

  previewCompose(body: unknown = {}): Promise<Response> {
    return this.request("POST", paths.previewCompose, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  previewArtifacts(): Promise<Response> {
    return this.request("POST", paths.previewArtifacts, {
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
  }

  lockManifest(): Promise<Response> {
    return this.request("POST", paths.lockManifest, {
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
  }

  startBuild(body: { format?: string; output?: string } = {}): Promise<Response> {
    return this.request("POST", paths.startBuild, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  getBuild(id: string): Promise<Response> {
    return this.request("GET", paths.getBuild.replace("{id}", encodeURIComponent(id)));
  }

  cancelBuild(id: string): Promise<Response> {
    return this.request("DELETE", paths.cancelBuild.replace("{id}", encodeURIComponent(id)));
  }

  buildEvents(id: string): Promise<Response> {
    return this.request("GET", paths.buildEvents.replace("{id}", encodeURIComponent(id)));
  }

  manifestEvents(): Promise<Response> {
    return this.request("GET", paths.manifestEvents);
  }

  listInputs(): Promise<Response> {
    return this.request("GET", paths.listInputs);
  }

  discoverWorkspace(): Promise<Response> {
    return this.request("GET", paths.discoverWorkspace);
  }

  importPropose(body: {
    compose?: string;
    composes?: string[];
    dockerfiles?: string[];
    name?: string;
    activeProfiles?: string[];
    strict?: boolean;
    existingProject?: unknown;
  }): Promise<Response> {
    return this.request("POST", paths.importPropose, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importOnboard(body: {
    project: unknown;
    secretIds?: string[];
    suggestions?: unknown[];
    paramDefaults?: Record<string, unknown>;
    attachMounts?: boolean;
    writeEnvExample?: boolean;
  }): Promise<Response> {
    return this.request("POST", paths.importOnboard, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importEnvExample(body: {
    project?: unknown;
    secretIds?: string[];
    write?: boolean;
  }): Promise<Response> {
    return this.request("POST", paths.importEnvExample, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importStubPreview(body: {
    project: unknown;
    bindChoices?: { resourceId?: string; imageId?: string; mode: string; path?: string }[];
  }): Promise<Response> {
    return this.request("POST", paths.importStubPreview, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importPrepare(body: {
    project: unknown;
    path?: string;
    replace?: boolean;
  }): Promise<Response> {
    return this.request("POST", paths.importPrepare, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importWriteDraft(body: {
    project: unknown;
    path?: string;
    replace?: boolean;
  }): Promise<Response> {
    return this.request("POST", paths.importWriteDraft, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  importResourceStubs(body: { project: unknown }): Promise<Response> {
    return this.request("POST", paths.importResourceStubs, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  releaseTemplatesPropose(body?: { force?: boolean }): Promise<Response> {
    return this.request("POST", paths.releaseTemplatesPropose, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
    });
  }

  releaseTemplatesWrite(body?: { force?: boolean }): Promise<Response> {
    return this.request("POST", paths.releaseTemplatesWrite, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
    });
  }

  generateSecretFile(body: { id: string; kind: "password" | "port" | "name"; replace?: boolean }): Promise<Response> {
    return this.request("POST", paths.generateSecretFile, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }
}
