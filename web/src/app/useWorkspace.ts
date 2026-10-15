import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { applyCommand, isPresentationCommand, type Command } from "../domain/commands";
import { emptyProject, type Project } from "../domain/project";
import { parseProjectYaml, projectToYaml } from "../domain/serialize";
import { readCsrf } from "./csrf";
import {
  emptyHistory,
  pushHistory,
  redoHistory,
  skipsHistory,
  undoHistory,
} from "./editHistory";
import type { AppState, Diagnostic } from "./types";

function asDiagnostics(v: unknown): Diagnostic[] {
  if (!Array.isArray(v)) return [];
  return v.filter((x): x is Diagnostic => typeof x === "object" && x !== null) as Diagnostic[];
}

type ManifestEventKind = "changed" | "invalid" | "deleted" | "permissionDenied" | "resyncRequired";

type ManifestEvent = {
  kind: ManifestEventKind;
  revision: string;
};

const initial: AppState = {
  phase: "initial",
  project: null,
  lastValid: null,
  etag: null,
  dirty: false,
  validating: false,
  valid: null,
  diagnostics: [],
  preview: { status: "idle" },
  build: { status: "idle" },
  save: { status: "idle" },
  sync: { status: "idle" },
  allowDiskOverwrite: false,
  statusMessage: "",
  serverGraph: { status: "idle" },
  graphAnnounce: "",
  history: emptyHistory(),
  diagnosticsAttention: 0,
};

export function useWorkspace() {
  const client = useMemo(() => new IacpackClient({ baseUrl: "", csrf: readCsrf() }), []);
  const [state, setState] = useState<AppState>(initial);
  const previewSeq = useRef(0);
  const previewTimer = useRef<number | undefined>(undefined);
  const graphSeq = useRef(0);
  const graphTimer = useRef<number | undefined>(undefined);
  const applyingSelfRef = useRef(false);

  const dispatch = useCallback((cmd: Command) => {
    setState((s) => {
      if (!s.project && cmd.type !== "replaceProject" && cmd.type !== "resetEmpty") {
        return s;
      }
      if (s.sync.status === "permissionDenied") {
        return { ...s, statusMessage: "Cannot edit while disk is unreadable", graphAnnounce: "Edit blocked" };
      }
      const base = s.project ?? emptyProject();
      const result = applyCommand(base, cmd);
      if (result.error) {
        return {
          ...s,
          graphAnnounce: result.error,
          statusMessage: result.error,
        };
      }
      const presentation = isPresentationCommand(cmd);
      const nextDirty = true;
      let sync = s.sync;
      if (s.sync.status === "external" && nextDirty) {
        sync = { status: "conflict", revision: s.sync.revision, reason: "dirty-during-external" };
      }
      const history = skipsHistory(cmd)
        ? s.history
        : pushHistory(s.history, { project: base, dirty: s.dirty });
      return {
        ...s,
        project: result.project,
        dirty: nextDirty,
        sync,
        history,
        preview: presentation
          ? s.preview
          : s.preview.status === "current"
            ? { status: "stale" as const }
            : s.preview,
        serverGraph: presentation ? s.serverGraph : { status: "pending" as const },
        save: { status: "idle" },
        statusMessage: presentation ? "Layout changed (x-ui only)" : "Local changes (not saved)",
        graphAnnounce: presentation ? "Node layout updated" : "Manifest updated",
      };
    });
  }, []);

  const undo = useCallback(() => {
    setState((s) => {
      if (!s.project) return s;
      const stepped = undoHistory(s.history, { project: s.project, dirty: s.dirty });
      if (!stepped) {
        return {
          ...s,
          statusMessage: "Nothing to undo",
          graphAnnounce: "Nothing to undo",
        };
      }
      return {
        ...s,
        project: stepped.entry.project,
        dirty: stepped.entry.dirty,
        history: stepped.history,
        preview:
          s.preview.status === "current" ? { status: "stale" as const } : s.preview,
        serverGraph: { status: "pending" as const },
        save: { status: "idle" },
        statusMessage: "Undone",
        graphAnnounce: "Undone",
      };
    });
  }, []);

  const redo = useCallback(() => {
    setState((s) => {
      if (!s.project) return s;
      const stepped = redoHistory(s.history, { project: s.project, dirty: s.dirty });
      if (!stepped) {
        return {
          ...s,
          statusMessage: "Nothing to redo",
          graphAnnounce: "Nothing to redo",
        };
      }
      return {
        ...s,
        project: stepped.entry.project,
        dirty: stepped.entry.dirty,
        history: stepped.history,
        preview:
          s.preview.status === "current" ? { status: "stale" as const } : s.preview,
        serverGraph: { status: "pending" as const },
        save: { status: "idle" },
        statusMessage: "Redone",
        graphAnnounce: "Redone",
      };
    });
  }, []);

  const applyManifestEvent = useCallback((ev: ManifestEvent) => {
    if (applyingSelfRef.current) return;
    setState((s) => {
      if (ev.kind === "resyncRequired") {
        if (s.dirty) {
          return {
            ...s,
            sync: { status: "conflict", revision: ev.revision, reason: "resync" },
            statusMessage: "Resync required — local edits may conflict with disk",
            graphAnnounce: "Manifest resync required",
          };
        }
        return {
          ...s,
          sync: { status: "external", revision: ev.revision },
          statusMessage: "Resync required — reload from disk",
          graphAnnounce: "Manifest resync required",
        };
      }
      if (ev.kind === "deleted") {
        return {
          ...s,
          sync: { status: "deleted" },
          allowDiskOverwrite: false,
          statusMessage: "Manifest deleted on disk",
          graphAnnounce: "Manifest deleted on disk",
        };
      }
      if (ev.kind === "permissionDenied") {
        return {
          ...s,
          sync: { status: "permissionDenied", revision: ev.revision },
          allowDiskOverwrite: false,
          statusMessage: "Manifest unreadable on disk",
          graphAnnounce: "Manifest permission denied",
        };
      }
      if (ev.kind === "invalid") {
        return {
          ...s,
          sync: { status: "invalidDisk", revision: ev.revision },
          allowDiskOverwrite: false,
          project: s.lastValid ?? s.project,
          statusMessage: "Invalid YAML on disk — keeping last valid model",
          graphAnnounce: "Invalid disk manifest",
          diagnostics: [
            {
              severity: "error",
              message: "External manifest YAML is invalid; disk file left untouched",
              code: "DISK_INVALID",
            },
          ],
        };
      }
      // changed
      if (s.dirty) {
        return {
          ...s,
          sync: { status: "conflict", revision: ev.revision, reason: "external-while-dirty" },
          allowDiskOverwrite: false,
          statusMessage: "External change while dirty — conflict",
          graphAnnounce: "Manifest conflict",
        };
      }
      return {
        ...s,
        sync: { status: "external", revision: ev.revision },
        allowDiskOverwrite: false,
        statusMessage: "External manifest change detected",
        graphAnnounce: "External manifest change",
      };
    });
  }, []);

  const load = useCallback(async () => {
    setState((s) => ({
      ...s,
      phase: "loading",
      statusMessage: "Loading manifest…",
      allowDiskOverwrite: false,
    }));
    try {
      const res = await client.getManifest();
      if (res.status === 404) {
        const project = emptyProject();
        setState({
          ...initial,
          phase: "missing",
          project,
          lastValid: project,
          dirty: false,
          sync: { status: "absent" },
          statusMessage: "Manifest missing — open Import",
          history: emptyHistory(),
        });
        return;
      }
      if (!res.ok) {
        setState((s) => ({
          ...s,
          phase: "disconnected",
          statusMessage: `Load failed (${String(res.status)})`,
        }));
        return;
      }
      const etag = res.headers.get("ETag");
      const text = await res.text();
      const parsed = parseProjectYaml(text);
      if (!parsed.ok) {
        setState((s) => ({
          ...s,
          phase: "parseError",
          project: s.lastValid,
          etag,
          sync: { status: "invalidDisk", revision: etag ?? "unknown" },
          allowDiskOverwrite: false,
          diagnostics: [{ severity: "error", message: parsed.message }],
          statusMessage: "Manifest YAML invalid; keeping last valid model if any",
        }));
        return;
      }
      setState({
        ...initial,
        phase: "loaded",
        project: parsed.project,
        lastValid: parsed.project,
        etag,
        statusMessage: "Loaded",
        history: emptyHistory(),
      });
    } catch (err) {
      const detail = err instanceof Error ? err.message : "unknown";
      setState((s) => ({
        ...s,
        phase: "disconnected",
        statusMessage: `Network error while loading: ${detail}`,
      }));
    }
  }, [client]);

  const validate = useCallback(async () => {
    setState((s) => ({ ...s, validating: true, statusMessage: "Validating…" }));
    try {
      const res = await client.validateManifest({});
      const body = (await res.json()) as { ok?: boolean; diagnostics?: unknown };
      const diagnostics = asDiagnostics(body.diagnostics);
      const ok = Boolean(body.ok);
      setState((s) => ({
        ...s,
        validating: false,
        valid: ok,
        diagnostics,
        statusMessage: ok
          ? "Validation OK (saved file)"
          : `Validation reported ${String(diagnostics.length)} issue(s) (saved file)`,
        diagnosticsAttention: ok ? s.diagnosticsAttention : s.diagnosticsAttention + 1,
      }));
    } catch {
      setState((s) => ({
        ...s,
        validating: false,
        statusMessage: "Validate failed (network)",
      }));
    }
  }, [client]);

  const save = useCallback(async () => {
    const project = state.project;
    if (!project) return;
    if (state.sync.status === "permissionDenied") {
      setState((s) => ({ ...s, statusMessage: "Save blocked: disk unreadable" }));
      return;
    }
    const unsafe =
      state.sync.status === "invalidDisk" ||
      state.sync.status === "deleted" ||
      state.sync.status === "conflict" ||
      state.sync.status === "external";
    if (unsafe && !state.allowDiskOverwrite) {
      setState((s) => ({
        ...s,
        statusMessage: "Save blocked until you choose a recovery action",
        graphAnnounce: "Save blocked — resolve sync first",
      }));
      return;
    }
    setState((s) => ({ ...s, save: { status: "saving" }, statusMessage: "Saving…" }));
    const yaml = projectToYaml(project);
    applyingSelfRef.current = true;
    try {
      const res =
        state.phase === "missing" || state.etag === null || state.sync.status === "deleted"
          ? await client.putManifest(yaml, { ifNoneMatch: "*" })
          : await client.putManifest(yaml, { ifMatch: state.etag });
      if (res.status === 409) {
        setState((s) => ({
          ...s,
          save: { status: "conflict", detail: "ETag conflict — reload and merge" },
          sync: { status: "conflict", revision: "etag", reason: "etag" },
          allowDiskOverwrite: false,
          statusMessage: "Save conflict",
          graphAnnounce: "Save conflict",
        }));
        return;
      }
      if (!res.ok) {
        const detail = await res.text();
        setState((s) => ({
          ...s,
          save: { status: "error", detail },
          statusMessage: `Save failed (${String(res.status)})`,
        }));
        return;
      }
      const etag = res.headers.get("ETag");
      const body = (await res.json()) as { etag?: string };
      const lockRes = await client.lockManifest();
      const lockBody = (await lockRes.json()) as { ok?: boolean; diagnostics?: unknown };
      const lockFailed = !lockRes.ok || lockBody.ok === false;
      const lockDiags = lockFailed ? asDiagnostics(lockBody.diagnostics) : [];

      let diagnostics: Diagnostic[] = lockDiags;
      let validOk = !lockFailed;
      try {
        const valRes = await client.validateManifest({});
        const valBody = (await valRes.json()) as { ok?: boolean; diagnostics?: unknown };
        const valDiags = asDiagnostics(valBody.diagnostics);
        validOk = Boolean(valBody.ok) && !lockFailed;
        // Prefer validate diagnostics when present; keep lock diags if validate is empty.
        diagnostics = valDiags.length ? valDiags : lockDiags;
      } catch {
        if (!lockFailed) {
          setState((s) => ({
            ...s,
            phase: "loaded",
            etag: etag ?? body.etag ?? s.etag,
            dirty: false,
            lastValid: project,
            save: { status: "saved" },
            sync: { status: "idle" },
            allowDiskOverwrite: false,
            statusMessage: "Saved, but post-save validate failed (network)",
            preview: { status: "stale" },
          }));
          return;
        }
      }

      const ok = validOk && diagnostics.length === 0;
      let statusMessage: string;
      if (ok) statusMessage = "Saved and validated OK";
      else if (lockFailed && diagnostics.length)
        statusMessage = `Saved — ${String(diagnostics.length)} issue(s); lock unavailable`;
      else if (lockFailed) statusMessage = "Saved, but lock failed — preview/build may be unavailable";
      else statusMessage = `Saved — validation reported ${String(diagnostics.length)} issue(s)`;

      setState((s) => ({
        ...s,
        phase: "loaded",
        etag: etag ?? body.etag ?? s.etag,
        dirty: false,
        lastValid: project,
        save: { status: "saved" },
        sync: { status: "idle" },
        allowDiskOverwrite: false,
        diagnostics,
        valid: ok,
        statusMessage,
        preview: { status: "stale" },
        diagnosticsAttention: ok ? s.diagnosticsAttention : s.diagnosticsAttention + 1,
        graphAnnounce: ok ? "Saved and validated" : "Saved with validation issues",
      }));
    } catch {
      setState((s) => ({
        ...s,
        save: { status: "error", detail: "network" },
        statusMessage: "Save failed (network)",
      }));
    } finally {
      window.setTimeout(() => {
        applyingSelfRef.current = false;
      }, 1500);
    }
  }, [client, state.allowDiskOverwrite, state.etag, state.phase, state.project, state.sync.status]);

  const saveAs = useCallback(
    async (path: string) => {
      const project = state.project;
      if (!project) return;
      const yaml = projectToYaml(project);
      setState((s) => ({ ...s, save: { status: "saving" }, statusMessage: "Save As…" }));
      try {
        const res = await client.saveAsManifest(path, yaml);
        if (res.status === 409) {
          setState((s) => ({
            ...s,
            save: { status: "conflict", detail: "path exists" },
            statusMessage: "Save As refused overwrite",
          }));
          return;
        }
        if (!res.ok) {
          setState((s) => ({
            ...s,
            save: { status: "error", detail: awaitable(res.status) },
            statusMessage: "Save As failed",
          }));
          return;
        }
        setState((s) => ({
          ...s,
          save: { status: "saved" },
          statusMessage: `Saved as ${path}`,
        }));
      } catch {
        setState((s) => ({
          ...s,
          save: { status: "error", detail: "network" },
          statusMessage: "Save As failed (network)",
        }));
      }
    },
    [client, state.project],
  );

  const exportCanonical = useCallback(async () => {
    try {
      const res = await client.exportManifest();
      if (!res.ok) {
        setState((s) => ({ ...s, statusMessage: `Export failed (${String(res.status)})` }));
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "manifest.canonical.json";
      a.click();
      URL.revokeObjectURL(url);
      setState((s) => ({ ...s, statusMessage: "Exported canonical JSON" }));
    } catch {
      setState((s) => ({ ...s, statusMessage: "Export failed (network)" }));
    }
  }, [client]);

  const exportLocalYaml = useCallback(() => {
    const project = state.project;
    if (!project) return;
    const yaml = projectToYaml(project);
    const blob = new Blob([yaml], { type: "application/yaml" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "iacpack.local.yaml";
    a.click();
    URL.revokeObjectURL(url);
    setState((s) => ({
      ...s,
      statusMessage: "Exported local YAML (not written to configured path)",
      graphAnnounce: "Local YAML exported",
    }));
  }, [state.project]);

  const armOverwrite = useCallback(() => {
    setState((s) => ({
      ...s,
      allowDiskOverwrite: true,
      statusMessage: "Overwrite armed — Save or Overwrite will replace disk with local model",
      graphAnnounce: "Disk overwrite armed",
    }));
  }, []);

  const recoverOverwrite = useCallback(async () => {
    const project = state.project;
    if (!project) return;
    setState((s) => ({
      ...s,
      allowDiskOverwrite: true,
      save: { status: "saving" },
      statusMessage: "Overwriting disk with local model…",
    }));
    const yaml = projectToYaml(project);
    applyingSelfRef.current = true;
    try {
      const res =
        state.etag === null || state.sync.status === "deleted"
          ? await client.putManifest(yaml, { ifNoneMatch: "*" })
          : await client.putManifest(yaml, { ifMatch: state.etag });
      if (res.status === 409) {
        const g = await client.getManifest();
        const fresh = g.headers.get("ETag");
        if (!g.ok || !fresh) {
          setState((s) => ({
            ...s,
            save: { status: "conflict", detail: "ETag conflict" },
            sync: { status: "conflict", revision: "etag", reason: "etag" },
            allowDiskOverwrite: false,
            statusMessage: "Overwrite conflict — reload and retry",
          }));
          return;
        }
        const retry = await client.putManifest(yaml, { ifMatch: fresh });
        if (!retry.ok) {
          setState((s) => ({
            ...s,
            save: { status: "error", detail: String(retry.status) },
            statusMessage: "Overwrite failed",
          }));
          return;
        }
        const etag = retry.headers.get("ETag");
        setState((s) => ({
          ...s,
          phase: "loaded",
          etag: etag ?? s.etag,
          dirty: false,
          lastValid: project,
          save: { status: "saved" },
          sync: { status: "idle" },
          allowDiskOverwrite: false,
          statusMessage: "Disk overwritten with local model",
          preview: { status: "stale" },
        }));
        return;
      }
      if (!res.ok) {
        setState((s) => ({
          ...s,
          save: { status: "error", detail: String(res.status) },
          statusMessage: "Overwrite failed",
        }));
        return;
      }
      const etag = res.headers.get("ETag");
      setState((s) => ({
        ...s,
        phase: "loaded",
        etag: etag ?? s.etag,
        dirty: false,
        lastValid: project,
        save: { status: "saved" },
        sync: { status: "idle" },
        allowDiskOverwrite: false,
        statusMessage: "Disk overwritten with local model",
        preview: { status: "stale" },
      }));
    } catch {
      setState((s) => ({
        ...s,
        save: { status: "error", detail: "network" },
        statusMessage: "Overwrite failed (network)",
      }));
    } finally {
      window.setTimeout(() => {
        applyingSelfRef.current = false;
      }, 1500);
    }
  }, [client, state.etag, state.project, state.sync.status]);

  const refreshPreview = useCallback(async () => {
    const requestId = ++previewSeq.current;
    setState((s) => ({ ...s, preview: { status: "loading", requestId } }));
    try {
      const [composeRes, artRes] = await Promise.all([
        client.previewCompose({}),
        client.previewArtifacts(),
      ]);
      if (requestId !== previewSeq.current) return;
      const composeBody = (await composeRes.json()) as {
        ok?: boolean;
        result?: { compose?: string; files?: string[] };
        diagnostics?: unknown;
      };
      const artBody = (await artRes.json()) as {
        result?: { files?: string[] };
        diagnostics?: unknown;
      };
      if (!composeRes.ok || composeBody.ok === false) {
        setState((s) => ({
          ...s,
          preview: {
            status: "error",
            requestId,
            message: "Preview unavailable",
            diagnostics: asDiagnostics(composeBody.diagnostics),
          },
        }));
        return;
      }
      const files = Array.isArray(artBody.result?.files)
        ? artBody.result.files.filter((x): x is string => typeof x === "string")
        : Array.isArray(composeBody.result?.files)
          ? composeBody.result.files.filter((x): x is string => typeof x === "string")
          : [];
      setState((s) => ({
        ...s,
        preview: {
          status: "current",
          requestId,
          compose: typeof composeBody.result?.compose === "string" ? composeBody.result.compose : "",
          files,
        },
        diagnostics: asDiagnostics(composeBody.diagnostics),
      }));
    } catch {
      if (requestId !== previewSeq.current) return;
      setState((s) => ({
        ...s,
        preview: { status: "error", requestId, message: "network", diagnostics: [] },
      }));
    }
  }, [client]);

  const schedulePreview = useCallback(() => {
    window.clearTimeout(previewTimer.current);
    previewTimer.current = window.setTimeout(() => {
      void refreshPreview();
    }, 400);
  }, [refreshPreview]);

  const startBuild = useCallback(async () => {
    setState((s) => ({ ...s, build: { status: "running", id: "" }, statusMessage: "Starting build…" }));
    try {
      const res = await client.startBuild({ format: "directory" });
      if (res.status === 429) {
        setState((s) => ({
          ...s,
          build: { status: "failed", id: "", diagnostics: [{ message: "build queue full" }] },
          statusMessage: "Build queue full",
        }));
        return;
      }
      const body = (await res.json()) as { id?: string; status?: string };
      const id = body.id ?? "";
      setState((s) => ({ ...s, build: { status: "running", id }, statusMessage: `Build ${id} running` }));
      for (let i = 0; i < 60; i++) {
        await new Promise((r) => setTimeout(r, 250));
        const st = await client.getBuild(id);
        const job = (await st.json()) as {
          status?: string;
          output?: string;
          diagnostics?: unknown;
        };
        if (job.status === "succeeded") {
          setState((s) => ({
            ...s,
            build: { status: "succeeded", id, output: job.output },
            statusMessage: "Build succeeded",
          }));
          return;
        }
        if (job.status === "failed") {
          setState((s) => ({
            ...s,
            build: { status: "failed", id, diagnostics: asDiagnostics(job.diagnostics) },
            statusMessage: "Build failed",
          }));
          return;
        }
        if (job.status === "cancelled") {
          setState((s) => ({
            ...s,
            build: { status: "cancelled", id },
            statusMessage: "Build cancelled",
          }));
          return;
        }
      }
    } catch {
      setState((s) => ({
        ...s,
        build: { status: "failed", id: "", diagnostics: [{ message: "network" }] },
        statusMessage: "Build failed (network)",
      }));
    }
  }, [client]);

  const cancelBuild = useCallback(async () => {
    const id = state.build.status === "running" ? state.build.id : "";
    if (!id) return;
    try {
      await client.cancelBuild(id);
      setState((s) => ({
        ...s,
        build: { status: "cancelled", id },
        statusMessage: "Build cancelled",
      }));
    } catch {
      setState((s) => ({ ...s, statusMessage: "Cancel failed" }));
    }
  }, [client, state.build]);

  const refreshServerGraph = useCallback(async () => {
    const requestId = ++graphSeq.current;
    setState((s) => ({ ...s, serverGraph: { status: "loading" } }));
    try {
      const res = await client.inspectGraph();
      if (requestId !== graphSeq.current) return;
      const body = (await res.json()) as { ok?: boolean; result?: unknown; diagnostics?: unknown };
      if (!res.ok || body.ok === false) {
        setState((s) => ({
          ...s,
          serverGraph: { status: "invalid", diagnostics: asDiagnostics(body.diagnostics) },
        }));
        return;
      }
      setState((s) => ({
        ...s,
        serverGraph: { status: "current", model: body.result ?? null },
        diagnostics: asDiagnostics(body.diagnostics).length
          ? asDiagnostics(body.diagnostics)
          : s.diagnostics,
      }));
    } catch {
      if (requestId !== graphSeq.current) return;
      setState((s) => ({
        ...s,
        serverGraph: { status: "invalid", diagnostics: [{ message: "graph request failed" }] },
      }));
    }
  }, [client]);

  const scheduleServerGraph = useCallback(() => {
    window.clearTimeout(graphTimer.current);
    graphTimer.current = window.setTimeout(() => {
      setState((s) =>
        s.serverGraph.status === "current" ? { ...s, serverGraph: { status: "pending" } } : s,
      );
      void refreshServerGraph();
    }, 350);
  }, [refreshServerGraph]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      void load();
    }, 0);
    return () => window.clearTimeout(id);
  }, [load]);

  useEffect(() => {
    if (state.phase === "loaded" && !state.dirty && state.sync.status === "idle") {
      schedulePreview();
      scheduleServerGraph();
    }
  }, [state.phase, state.dirty, state.etag, state.sync.status, schedulePreview, scheduleServerGraph]);

  useEffect(() => {
    let es: EventSource | null = null;
    let closed = false;
    let retryTimer: number | undefined;

    const connect = () => {
      if (closed) return;
      es = new EventSource("/api/v1/manifest/events");
      es.addEventListener("manifest", (raw) => {
        try {
          const data = JSON.parse((raw as MessageEvent).data as string) as ManifestEvent;
          if (!data?.kind) return;
          // Ignore bootstrap resync on first paint; load() already hydrated.
          if (data.kind === "resyncRequired" && data.revision === "bootstrap") return;
          applyManifestEvent(data);
        } catch {
          /* ignore malformed */
        }
      });
      es.onerror = () => {
        setState((s) =>
          s.sync.status === "sseDisconnected"
            ? s
            : {
                ...s,
                sync:
                  s.sync.status === "idle"
                    ? { status: "sseDisconnected" }
                    : s.sync,
                statusMessage: s.statusMessage.includes("sync")
                  ? s.statusMessage
                  : `${s.statusMessage}${s.statusMessage ? " · " : ""}sync reconnecting`,
              },
        );
        es?.close();
        retryTimer = window.setTimeout(connect, 1500);
      };
      es.onopen = () => {
        setState((s) =>
          s.sync.status === "sseDisconnected"
            ? { ...s, sync: { status: "idle" }, statusMessage: "Live sync reconnected" }
            : s,
        );
      };
    };
    connect();
    return () => {
      closed = true;
      window.clearTimeout(retryTimer);
      es?.close();
    };
  }, [applyManifestEvent]);

  const announce = useCallback((msg: string) => {
    setState((s) => ({ ...s, graphAnnounce: msg, statusMessage: msg }));
  }, []);

  const saveBlocked =
    (state.sync.status === "invalidDisk" ||
      state.sync.status === "deleted" ||
      state.sync.status === "conflict" ||
      state.sync.status === "external" ||
      state.sync.status === "permissionDenied") &&
    !state.allowDiskOverwrite;

  return {
    state,
    saveBlocked,
    canUndo: state.history.past.length > 0,
    canRedo: state.history.future.length > 0,
    dispatch,
    undo,
    redo,
    announce,
    load,
    validate,
    save,
    saveAs,
    exportCanonical,
    exportLocalYaml,
    armOverwrite,
    recoverOverwrite,
    refreshPreview,
    refreshServerGraph,
    startBuild,
    cancelBuild,
    setProjectForTest(project: Project) {
      setState((s) => ({ ...s, project, dirty: true, phase: "loaded", history: emptyHistory() }));
    },
  };
}

function awaitable(n: number): string {
  return String(n);
}
