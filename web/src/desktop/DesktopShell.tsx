import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { useLocale } from "../i18n/useLocale";
import { LangSwitch } from "../i18n/LangSwitch";
import type { MessageKey } from "../i18n/messages";
import { computeWindowDefaults, type WinPos } from "./layout";
import { WindowFrame, type WindowId } from "./WindowFrame";
import "./desktop.css";

type WindowState = {
  open: boolean;
  minimized: boolean;
  z: number;
};

const ICONS: {
  id: WindowId;
  glyph: string;
  accent: string;
  testId?: string;
  tab?: boolean;
  blurbKey: MessageKey;
}[] = [
  {
    id: "designer",
    glyph: "◇",
    accent: "#38bdf8",
    testId: "tab-designer",
    tab: true,
    blurbKey: "desktop.blurb.designer",
  },
  {
    id: "forms",
    glyph: "☰",
    accent: "#34d399",
    testId: "tab-forms",
    tab: true,
    blurbKey: "desktop.blurb.forms",
  },
  { id: "preview", glyph: "▣", accent: "#a78bfa", blurbKey: "desktop.blurb.preview" },
  { id: "diagnostics", glyph: "!", accent: "#fbbf24", blurbKey: "desktop.blurb.diagnostics" },
  { id: "inspector", glyph: "◎", accent: "#fb7185", blurbKey: "desktop.blurb.inspector" },
];

const TAB_IDS: WindowId[] = ["designer", "forms"];

function nextZ(windows: Record<WindowId, WindowState>): number {
  return Math.max(...Object.values(windows).map((w) => w.z)) + 1;
}

type Props = {
  toolbar: ReactNode;
  syncBanner: ReactNode;
  statusText: ReactNode;
  panes: Record<WindowId, ReactNode>;
  /** Bump to force-open/focus a window (e.g. inspector on graph select). */
  raise?: { id: WindowId; token: number } | null;
};

function useClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(id);
  }, []);
  return {
    label: now.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" }),
    iso: now.toISOString(),
  };
}

export function DesktopShell({ toolbar, syncBanner, statusText, panes, raise }: Props) {
  const { t } = useLocale();
  const clock = useClock();
  const startRef = useRef<HTMLDivElement>(null);
  const [defaults, setDefaults] = useState<Record<WindowId, WinPos>>(() =>
    computeWindowDefaults(1280, 720),
  );
  const [windows, setWindows] = useState<Record<WindowId, WindowState>>({
    designer: { open: true, minimized: false, z: 12 },
    forms: { open: false, minimized: false, z: 11 },
    preview: { open: true, minimized: true, z: 13 },
    diagnostics: { open: true, minimized: true, z: 10 },
    inspector: { open: true, minimized: true, z: 9 },
  });
  const [focus, setFocus] = useState<WindowId>("designer");
  const [authoringTab, setAuthoringTab] = useState<"designer" | "forms">("designer");
  const [startOpen, setStartOpen] = useState(false);

  useEffect(() => {
    let lastW = 0;
    let lastH = 0;
    let timer: number | undefined;
    const measure = () => {
      const stage = document.getElementById("main-content");
      if (!stage) return;
      const r = stage.getBoundingClientRect();
      if (r.width < 40 || r.height < 40) return;
      if (Math.abs(r.width - lastW) < 16 && Math.abs(r.height - lastH) < 16 && lastW > 0) return;
      lastW = r.width;
      lastH = r.height;
      setDefaults(computeWindowDefaults(r.width, r.height));
    };
    const onResize = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(measure, 120);
    };
    measure();
    window.addEventListener("resize", onResize);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("resize", onResize);
    };
  }, []);

  const titles = useMemo(
    (): Record<WindowId, string> => ({
      designer: t("tab.designer"),
      forms: t("tab.forms"),
      preview: t("preview.title"),
      diagnostics: t("diagnostics"),
      inspector: t("inspector.aria"),
    }),
    [t],
  );

  const accents = useMemo(() => {
    const m = {} as Record<WindowId, string>;
    for (const icon of ICONS) m[icon.id] = icon.accent;
    return m;
  }, []);

  const openWindow = useCallback((id: WindowId) => {
    if (id === "designer" || id === "forms") setAuthoringTab(id);
    setWindows((w) => ({
      ...w,
      [id]: { open: true, minimized: false, z: nextZ(w) },
    }));
    setFocus(id);
    setStartOpen(false);
  }, []);

  const focusWindow = useCallback((id: WindowId) => {
    if (id === "designer" || id === "forms") setAuthoringTab(id);
    setWindows((w) => ({
      ...w,
      [id]: { ...w[id], open: true, minimized: false, z: nextZ(w) },
    }));
    setFocus(id);
  }, []);

  const raiseToken = raise?.token;
  const raiseId = raise?.id;
  const [appliedRaise, setAppliedRaise] = useState<number | null>(null);
  useEffect(() => {
    if (!raiseId || raiseToken == null || appliedRaise === raiseToken) return;
    // External raise token from App (import Accept / preview) — sync window chrome once.
    // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional one-shot raise sync
    setAppliedRaise(raiseToken);
    if (raiseId === "designer" || raiseId === "forms") setAuthoringTab(raiseId);
    setWindows((w) => ({
      ...w,
      [raiseId]: { open: true, minimized: false, z: nextZ(w) },
    }));
    setFocus(raiseId);
    setStartOpen(false);
  }, [raiseId, raiseToken, appliedRaise]);

  useEffect(() => {
    if (!startOpen) return;
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") setStartOpen(false);
    };
    const onPointer = (e: MouseEvent) => {
      const el = startRef.current;
      if (!el) return;
      const target = e.target as Node | null;
      if (target && (el.contains(target) || (target instanceof Element && target.closest(".desk-start-btn")))) {
        return;
      }
      setStartOpen(false);
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", onPointer);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onPointer);
    };
  }, [startOpen]);

  const closeWindow = useCallback((id: WindowId) => {
    setWindows((w) => ({ ...w, [id]: { ...w[id], open: false, minimized: false } }));
  }, []);

  const minimizeWindow = useCallback((id: WindowId) => {
    setWindows((w) => ({ ...w, [id]: { ...w[id], minimized: true } }));
  }, []);

  const toggleTask = useCallback(
    (id: WindowId) => {
      const cur = windows[id];
      if (!cur.open || cur.minimized) openWindow(id);
      else if (focus === id) minimizeWindow(id);
      else focusWindow(id);
    },
    [windows, focus, openWindow, minimizeWindow, focusWindow],
  );

  const onTabKey = useCallback(
    (e: KeyboardEvent<HTMLButtonElement>, id: WindowId) => {
      if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
      e.preventDefault();
      const idx = TAB_IDS.indexOf(id);
      if (idx < 0) return;
      const next = TAB_IDS[(idx + 1) % TAB_IDS.length]!;
      openWindow(next);
      document.getElementById(`tab-${next}`)?.focus();
    },
    [openWindow],
  );

  const iconLayout = useMemo(
    () =>
      ICONS.map((icon, i) => ({
        ...icon,
        x: 18,
        y: 18 + i * 104,
      })),
    [],
  );

  return (
    <div className="desk-shell">
      <a className="skip-link" href="#main-content">
        {t("skip.main")}
      </a>
      <div className="desk-wallpaper" aria-hidden="true">
        <div className="desk-wallpaper-grid" />
        <div className="desk-wallpaper-glow desk-wallpaper-glow-a" />
        <div className="desk-wallpaper-glow desk-wallpaper-glow-b" />
      </div>
      <header className="desk-menubar" role="banner">
        <div className="desk-brand">
          <h1>iacpack</h1>
        </div>
        <div className="desk-tab-icons" role="tablist" aria-label={t("tabs.aria")}>
          {iconLayout
            .filter((icon) => icon.tab)
            .map((icon) => {
              const selected = authoringTab === icon.id;
              return (
                <button
                  key={icon.id}
                  type="button"
                  id={`tab-${icon.id}`}
                  role="tab"
                  aria-selected={selected}
                  aria-controls={`panel-${icon.id}`}
                  tabIndex={selected ? 0 : -1}
                  className={`desk-tab${selected ? " active" : ""}`}
                  data-testid={icon.testId}
                  onClick={() => openWindow(icon.id)}
                  onKeyDown={(e) => onTabKey(e, icon.id)}
                >
                  <strong>{titles[icon.id]}</strong>
                </button>
              );
            })}
        </div>
        <div className="desk-menubar-actions">{toolbar}</div>
        <LangSwitch />
      </header>
      {syncBanner}
      <main id="main-content" className="desk-stage" tabIndex={-1}>
        {(Object.keys(windows) as WindowId[]).map((id) =>
          windows[id].open ? (
            <WindowFrame
              key={id}
              id={id}
              title={titles[id]}
              accent={accents[id]}
              zIndex={windows[id].z}
              focused={focus === id}
              minimized={windows[id].minimized}
              defaultPos={defaults[id]}
              fillStage={id === "designer" || id === "forms"}
              onFocus={() => focusWindow(id)}
              onClose={() => closeWindow(id)}
              onMinimize={() => minimizeWindow(id)}
            >
              <div
                className={`desk-pane desk-pane-${id}`}
                role={id === "designer" || id === "forms" ? "tabpanel" : undefined}
                id={id === "designer" || id === "forms" ? `panel-${id}` : undefined}
                aria-labelledby={id === "designer" || id === "forms" ? `tab-${id}` : undefined}
              >
                {panes[id]}
              </div>
            </WindowFrame>
          ) : null,
        )}
        {startOpen ? (
          <div
            ref={startRef}
            className="desk-start"
            role="dialog"
            aria-modal="true"
            aria-label={t("desktop.start")}
          >
            <div className="desk-start-head">
              <div>
                <p className="desk-start-mark">{t("desktop.start.mark")}</p>
                <h2>{t("desktop.start")}</h2>
              </div>
              <button type="button" aria-label={t("desktop.close")} onClick={() => setStartOpen(false)}>
                ×
              </button>
            </div>
            <ul className="desk-start-list">
              {ICONS.map((icon) => (
                <li key={icon.id}>
                  <button type="button" onClick={() => openWindow(icon.id)}>
                    <span
                      className="desk-start-glyph"
                      style={{ ["--icon-accent" as string]: icon.accent }}
                      aria-hidden="true"
                    >
                      {icon.glyph}
                    </span>
                    <span className="desk-start-copy">
                      <strong>{titles[icon.id]}</strong>
                      <span>{t(icon.blurbKey)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </main>
      <footer className="desk-taskbar" role="contentinfo" aria-label={t("desktop.taskbar")}>
        <button
          type="button"
          className={`desk-start-btn${startOpen ? " active" : ""}`}
          aria-expanded={startOpen}
          onClick={() => setStartOpen((v) => !v)}
        >
          iacpack
        </button>
        <div className="desk-tasks" role="toolbar" aria-label={t("desktop.running")}>
          {(Object.keys(windows) as WindowId[])
            .filter((id) => windows[id].open)
            .sort((a, b) => windows[a].z - windows[b].z)
            .map((id) => (
              <button
                key={id}
                type="button"
                className={`desk-task${focus === id && !windows[id].minimized ? " active" : ""}`}
                style={{ ["--win-accent" as string]: accents[id] }}
                aria-pressed={focus === id && !windows[id].minimized}
                onClick={() => toggleTask(id)}
              >
                {titles[id]}
              </button>
            ))}
        </div>
        <div className="desk-tray">
          <span className="desk-tray-status" role="status" aria-live="polite" data-testid="status">
            {statusText}
          </span>
          <time className="desk-clock" dateTime={clock.iso}>
            {clock.label}
          </time>
        </div>
      </footer>
    </div>
  );
}
