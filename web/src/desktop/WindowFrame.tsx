import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import { useLocale } from "../i18n/useLocale";
import { clampWindowPos } from "./layout";

export type WindowId = "designer" | "forms" | "preview" | "diagnostics" | "inspector";

type Pos = { x: number; y: number; w: number; h: number };

type Props = {
  id: WindowId;
  title: string;
  accent?: string;
  zIndex: number;
  focused: boolean;
  minimized?: boolean;
  defaultPos: Pos;
  /** Fill the stage until the user restores the window. */
  fillStage?: boolean;
  onFocus: () => void;
  onClose: () => void;
  onMinimize: () => void;
  children: ReactNode;
};

const STORAGE = "iacpack.desktop.windows.v3";

function clamp(pos: Pos, stage: DOMRect): Pos {
  return clampWindowPos(pos, stage.width, stage.height);
}

function loadPos(id: string): Pos | null {
  try {
    const raw = localStorage.getItem(STORAGE);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Record<string, Pos>;
    const p = parsed[id];
    if (!p || ![p.x, p.y, p.w, p.h].every(Number.isFinite)) return null;
    return p;
  } catch {
    return null;
  }
}

function savePos(id: string, pos: Pos) {
  try {
    const raw = localStorage.getItem(STORAGE);
    const parsed = raw ? (JSON.parse(raw) as Record<string, Pos>) : {};
    parsed[id] = pos;
    localStorage.setItem(STORAGE, JSON.stringify(parsed));
  } catch {
    /* ignore */
  }
}

function stageRect(): DOMRect | null {
  return document.getElementById("main-content")?.getBoundingClientRect() ?? null;
}

export function WindowFrame({
  id,
  title,
  accent,
  zIndex,
  focused,
  minimized,
  defaultPos,
  fillStage = false,
  onFocus,
  onClose,
  onMinimize,
  children,
}: Props) {
  const { t } = useLocale();
  const drag = useRef<{
    pointerId: number;
    sx: number;
    sy: number;
    ox: number;
    oy: number;
  } | null>(null);
  const resize = useRef<{
    pointerId: number;
    sx: number;
    sy: number;
    ow: number;
    oh: number;
  } | null>(null);
  const [pos, setPos] = useState<Pos>(() => loadPos(id) ?? defaultPos);
  const [dragging, setDragging] = useState(false);
  const [maximized, setMaximized] = useState(fillStage);
  const restore = useRef<Pos | null>(fillStage ? (loadPos(id) ?? defaultPos) : null);

  useEffect(() => {
    let timer: number | undefined;
    const reclamp = () => {
      const stage = stageRect();
      if (!stage) return;
      if (maximized) {
        setPos((p) => {
          const next = { x: 8, y: 8, w: stage.width - 16, h: stage.height - 16 };
          return p.x === next.x && p.y === next.y && p.w === next.w && p.h === next.h ? p : next;
        });
        return;
      }
      setPos((p) => {
        const next = clamp(p, stage);
        return p.x === next.x && p.y === next.y && p.w === next.w && p.h === next.h ? p : next;
      });
    };
    const onResize = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(reclamp, 120);
    };
    reclamp();
    window.addEventListener("resize", onResize);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("resize", onResize);
    };
  }, [maximized]);

  const onBarDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return;
      if (e.target instanceof HTMLElement && e.target.closest("button")) return;
      if (maximized) return;
      onFocus();
      e.currentTarget.setPointerCapture(e.pointerId);
      drag.current = { pointerId: e.pointerId, sx: e.clientX, sy: e.clientY, ox: pos.x, oy: pos.y };
      setDragging(true);
    },
    [onFocus, pos.x, pos.y, maximized],
  );

  const onBarMove = useCallback((e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || d.pointerId !== e.pointerId) return;
    const stage = stageRect();
    if (!stage) return;
    setPos((p) =>
      clamp({ ...p, x: d.ox + e.clientX - d.sx, y: d.oy + e.clientY - d.sy }, stage),
    );
  }, []);

  const endDrag = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      const d = drag.current;
      if (!d || d.pointerId !== e.pointerId) return;
      drag.current = null;
      setDragging(false);
      setPos((p) => {
        savePos(id, p);
        return p;
      });
    },
    [id],
  );

  const toggleMaximize = useCallback(() => {
    const stage = stageRect();
    if (!stage) return;
    onFocus();
    if (maximized && restore.current) {
      setPos(clamp(restore.current, stage));
      setMaximized(false);
      return;
    }
    restore.current = pos;
    setPos({ x: 8, y: 8, w: stage.width - 16, h: stage.height - 16 });
    setMaximized(true);
  }, [maximized, onFocus, pos]);

  const onResizeDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0 || maximized) return;
      e.stopPropagation();
      onFocus();
      e.currentTarget.setPointerCapture(e.pointerId);
      resize.current = {
        pointerId: e.pointerId,
        sx: e.clientX,
        sy: e.clientY,
        ow: pos.w,
        oh: pos.h,
      };
    },
    [maximized, onFocus, pos.w, pos.h],
  );

  const onResizeMove = useCallback((e: ReactPointerEvent<HTMLDivElement>) => {
    const r = resize.current;
    if (!r || r.pointerId !== e.pointerId) return;
    const stage = stageRect();
    if (!stage) return;
    setPos((p) =>
      clamp({ ...p, w: r.ow + e.clientX - r.sx, h: r.oh + e.clientY - r.sy }, stage),
    );
  }, []);

  const endResize = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      const r = resize.current;
      if (!r || r.pointerId !== e.pointerId) return;
      resize.current = null;
      setPos((p) => {
        savePos(id, p);
        return p;
      });
    },
    [id],
  );

  if (minimized) return null;

  return (
    <article
      className={`desk-window${focused ? " focused" : ""}${dragging ? " dragging" : ""}${maximized ? " maximized" : ""}`}
      data-window={id}
      aria-labelledby={`win-title-${id}`}
      style={
        {
          left: pos.x,
          top: pos.y,
          width: pos.w,
          height: pos.h,
          zIndex,
          ["--win-accent"]: accent ?? "rgb(var(--desktop-accent-rgb))",
        } as CSSProperties
      }
      onMouseDown={onFocus}
    >
      <div
        className="desk-window-bar"
        onPointerDown={onBarDown}
        onPointerMove={onBarMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onDoubleClick={toggleMaximize}
      >
        <div className="desk-window-title">
          <span className="desk-window-dot" aria-hidden="true" />
          <h2 id={`win-title-${id}`}>{title}</h2>
        </div>
        <div className="desk-window-actions">
          <button type="button" className="win-min" aria-label={t("desktop.minimize")} onClick={onMinimize}>
            –
          </button>
          <button
            type="button"
            className="win-max"
            aria-label={maximized ? t("desktop.restore") : t("desktop.maximize")}
            onClick={toggleMaximize}
          >
            {maximized ? "❐" : "□"}
          </button>
          <button type="button" className="win-close" aria-label={t("desktop.close")} onClick={onClose}>
            ×
          </button>
        </div>
      </div>
      <div className="desk-window-body">{children}</div>
      {!maximized ? (
        <div
          className="desk-resize"
          aria-hidden="true"
          onPointerDown={onResizeDown}
          onPointerMove={onResizeMove}
          onPointerUp={endResize}
          onPointerCancel={endResize}
        />
      ) : null}
    </article>
  );
}
