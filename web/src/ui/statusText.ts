import type { AppState } from "../app/types";
import type { MessageKey } from "../i18n/messages";

function syncKey(state: AppState): MessageKey | null {
  switch (state.sync.status) {
    case "external":
      return "toolbar.sync.external";
    case "conflict":
      return "toolbar.sync.conflict";
    case "invalidDisk":
      return "toolbar.sync.invalidDisk";
    case "deleted":
      return "toolbar.sync.deleted";
    case "permissionDenied":
      return "toolbar.sync.permissionDenied";
    case "sseDisconnected":
      return "toolbar.sync.sseDisconnected";
    default:
      return null;
  }
}

export function buildStatusText(state: AppState, t: (key: MessageKey) => string): string {
  const sync = syncKey(state);
  return [
    state.statusMessage,
    state.dirty ? t("toolbar.unsaved") : state.save.status === "saved" ? t("toolbar.saved") : "",
    state.save.status === "saving" ? t("toolbar.saving") : "",
    sync ? t(sync) : "",
    state.build.status === "cancelled" ? t("toolbar.build.cancelled") : "",
    state.build.status === "failed" ? t("toolbar.build.failed") : "",
    state.build.status === "succeeded" ? t("toolbar.build.ok") : "",
    state.save.status === "conflict" ? t("toolbar.save.conflict") : "",
  ].join("");
}
