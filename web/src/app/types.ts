import type { Project } from "../domain/project";

export type Diagnostic = {
  severity?: string;
  code?: string;
  message?: string;
  path?: string;
  field?: string;
  hint?: string;
  /** File path string or structured `{ pointer, line, column, file }` from API. */
  source?: string | Record<string, unknown>;
  [key: string]: unknown;
};

export type PreviewState =
  | { status: "idle" }
  | { status: "stale" }
  | { status: "loading"; requestId: number }
  | { status: "current"; requestId: number; compose: string; files: string[] }
  | { status: "error"; requestId: number; message: string; diagnostics: Diagnostic[] };

export type BuildState =
  | { status: "idle" }
  | { status: "running"; id: string }
  | { status: "succeeded"; id: string; output?: string }
  | { status: "failed"; id: string; diagnostics: Diagnostic[] }
  | { status: "cancelled"; id: string };

export type SaveState =
  | { status: "idle" }
  | { status: "saving" }
  | { status: "saved" }
  | { status: "conflict"; detail: string }
  | { status: "error"; detail: string };

export type AppPhase =
  | "initial"
  | "loading"
  | "loaded"
  | "missing"
  | "parseError"
  | "disconnected";

export type ServerGraphState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "pending" }
  | { status: "current"; model: unknown }
  | { status: "invalid"; diagnostics: Diagnostic[] };

/** Disk awareness relative to the in-memory model (P14). */
export type SyncState =
  | { status: "idle" }
  | { status: "external"; revision: string }
  | { status: "conflict"; revision: string; reason: string }
  | { status: "invalidDisk"; revision: string }
  | { status: "deleted" }
  | { status: "absent" }
  | { status: "permissionDenied"; revision: string }
  | { status: "sseDisconnected" };

export type HistoryEntry = {
  project: Project;
  dirty: boolean;
};

export type EditHistory = {
  past: HistoryEntry[];
  future: HistoryEntry[];
};

export type AppState = {
  phase: AppPhase;
  project: Project | null;
  lastValid: Project | null;
  etag: string | null;
  dirty: boolean;
  validating: boolean;
  valid: boolean | null;
  diagnostics: Diagnostic[];
  preview: PreviewState;
  build: BuildState;
  save: SaveState;
  sync: SyncState;
  /** Explicit recovery arm for overwriting an invalid/deleted disk file. */
  allowDiskOverwrite: boolean;
  statusMessage: string;
  serverGraph: ServerGraphState;
  graphAnnounce: string;
  /** Local undo/redo stacks for domain commands (not persisted). */
  history: EditHistory;
  /** Bumped when validation reports issues so the UI can raise the diagnostics window. */
  diagnosticsAttention: number;
};
