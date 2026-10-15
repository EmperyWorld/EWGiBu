import { useEffect, useRef } from "react";
import type { AppState } from "../app/types";
import { useLocale } from "../i18n/useLocale";

type Props = {
  state: AppState;
  onReload: () => void;
  onDiscardLocal: () => void;
  onExportLocal: () => void;
  onArmOverwrite: () => void;
  onRecoverOverwrite: () => void;
  onImport?: () => void;
};

function firstFocusable(root: HTMLElement | null): HTMLElement | null {
  if (!root) return null;
  return root.querySelector<HTMLElement>("button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])");
}

export function SyncBanner({
  state,
  onReload,
  onDiscardLocal,
  onExportLocal,
  onArmOverwrite,
  onRecoverOverwrite,
  onImport,
}: Props) {
  const { t } = useLocale();
  const { sync, dirty, allowDiskOverwrite } = state;
  const bannerRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const prevStatusRef = useRef(sync.status);

  useEffect(() => {
    const prev = prevStatusRef.current;
    prevStatusRef.current = sync.status;

    if (prev === "idle" && sync.status !== "idle") {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body) {
        restoreFocusRef.current = active;
      }
      const id = window.setTimeout(() => {
        const target = firstFocusable(bannerRef.current) ?? bannerRef.current;
        target?.focus();
      }, 0);
      return () => window.clearTimeout(id);
    }

    if (prev !== "idle" && sync.status === "idle") {
      const el = restoreFocusRef.current;
      restoreFocusRef.current = null;
      if (el && document.contains(el)) {
        const id = window.setTimeout(() => el.focus(), 0);
        return () => window.clearTimeout(id);
      }
    }
    return undefined;
  }, [sync.status]);

  if (sync.status === "idle") return null;

  let title = "";
  let detail = "";
  let tone: "warn" | "error" = "warn";
  if (sync.status === "external") {
    title = t("sync.external.title");
    detail = t("sync.external.detail");
  } else if (sync.status === "conflict") {
    title = t("sync.conflict.title");
    detail = sync.reason === "etag" ? t("sync.conflict.etag") : t("sync.conflict.dirty");
    tone = "error";
  } else if (sync.status === "invalidDisk") {
    title = t("sync.invalid.title");
    detail = t("sync.invalid.detail");
    tone = "error";
  } else if (sync.status === "absent") {
    title = t("sync.absent.title");
    detail = t("sync.absent.detail");
    tone = "warn";
  } else if (sync.status === "deleted") {
    title = t("sync.deleted.title");
    detail = t("sync.deleted.detail");
    tone = "error";
  } else if (sync.status === "permissionDenied") {
    title = t("sync.denied.title");
    detail = t("sync.denied.detail");
    tone = "error";
  } else if (sync.status === "sseDisconnected") {
    title = t("sync.sse.title");
    detail = t("sync.sse.detail");
  }

  const canReloadClean = sync.status === "external" || sync.status === "conflict";
  const showOverwrite =
    sync.status === "invalidDisk" || sync.status === "deleted" || sync.status === "conflict";

  return (
    <div
      ref={bannerRef}
      className={`sync-banner sync-${tone}`}
      role="alert"
      tabIndex={-1}
      data-testid="sync-banner"
      data-sync={sync.status}
    >
      <div className="sync-copy">
        <strong>{title}</strong>
        <span className="sync-label" aria-hidden="true">
          [{sync.status}]
        </span>
        <p>{detail}</p>
        {dirty && sync.status !== "absent" ? <p className="muted">{t("sync.dirtyNote")}</p> : null}
        {sync.status !== "absent" ? <p className="muted save-note">{t("sync.saveNote")}</p> : null}
      </div>
      <div className="sync-actions" role="group" aria-label={t("sync.actions.aria")}>
        {sync.status === "absent" && onImport ? (
          <button type="button" data-testid="sync-import" onClick={onImport}>
            {t("toolbar.import")}
          </button>
        ) : null}
        {canReloadClean && !dirty ? (
          <button type="button" data-testid="sync-reload" onClick={onReload}>
            {t("sync.reload")}
          </button>
        ) : null}
        {dirty ? (
          <button type="button" data-testid="sync-discard" onClick={onDiscardLocal}>
            {t("sync.discard")}
          </button>
        ) : null}
        <button type="button" data-testid="sync-export" onClick={onExportLocal}>
          {t("sync.export")}
        </button>
        {showOverwrite && !allowDiskOverwrite ? (
          <button type="button" data-testid="sync-arm-overwrite" onClick={onArmOverwrite}>
            {t("sync.arm")}
          </button>
        ) : null}
        {showOverwrite && allowDiskOverwrite ? (
          <button type="button" data-testid="sync-overwrite" onClick={onRecoverOverwrite}>
            {t("sync.overwrite")}
          </button>
        ) : null}
      </div>
    </div>
  );
}
