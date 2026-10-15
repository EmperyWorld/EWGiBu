import type { Command } from "../domain/commands";
import type { EditHistory, HistoryEntry } from "./types";

export type { EditHistory, HistoryEntry };

export const emptyHistory = (): EditHistory => ({ past: [], future: [] });

export const HISTORY_LIMIT = 50;

/** Commands that fire too often to record (pan/zoom). */
export function skipsHistory(cmd: Command): boolean {
  return cmd.type === "setViewport";
}

export function pushHistory(
  history: EditHistory,
  entry: HistoryEntry,
  limit = HISTORY_LIMIT,
): EditHistory {
  const past = [...history.past, entry];
  while (past.length > limit) past.shift();
  return { past, future: [] };
}

export function undoHistory(
  history: EditHistory,
  current: HistoryEntry,
): { history: EditHistory; entry: HistoryEntry } | null {
  if (history.past.length === 0) return null;
  const past = [...history.past];
  const entry = past.pop()!;
  return {
    history: { past, future: [...history.future, current] },
    entry,
  };
}

export function redoHistory(
  history: EditHistory,
  current: HistoryEntry,
): { history: EditHistory; entry: HistoryEntry } | null {
  if (history.future.length === 0) return null;
  const future = [...history.future];
  const entry = future.pop()!;
  return {
    history: { past: [...history.past, current], future },
    entry,
  };
}
