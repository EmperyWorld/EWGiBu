import { fieldFlashMs, scrollBehavior } from "./motion";

/** Map JSON-pointer-ish diagnostic fields to UI navigation targets. */

export type FieldNav = {
  /** Desktop window to raise. */
  window: "forms" | "designer" | "inspector";
  /** Preferred data-field-path selectors (longest match wins in DOM). */
  selectors: string[];
  /** Optional graph node id (service:name / task:name / resource:name). */
  selectNodeId?: string;
};

function normalizePointer(field: string): string {
  let p = field.trim();
  if (!p.startsWith("/")) p = `/${p}`;
  return p.replace(/~1/g, "/").replace(/~0/g, "~");
}

export function fieldNavFromPointer(field: string): FieldNav | null {
  const p = normalizePointer(field);
  const parts = p.split("/").filter(Boolean);

  if (parts[0] === "metadata") {
    const leaf = parts[1] ?? "name";
    return {
      window: "forms",
      selectors: [`[data-field-path="/metadata/${leaf}"]`, '[data-field-path="/metadata"]'],
    };
  }

  if (parts[0] !== "spec") return null;

  const section = parts[1];
  if (!section) return { window: "forms", selectors: ['[data-field-path="/spec"]'] };

  if (section === "mode" || section === "target" || section === "package") {
    return {
      window: "forms",
      selectors: [`[data-field-path="/spec/${section}"]`, '[data-field-path="/spec"]'],
    };
  }

  const maps = new Set([
    "services",
    "images",
    "tasks",
    "resources",
    "networks",
    "volumes",
    "parameters",
    "secrets",
  ]);
  if (!maps.has(section)) {
    return { window: "forms", selectors: [`[data-field-path="/spec/${section}"]`] };
  }

  const id = parts[2];
  if (!id) {
    return {
      window: "forms",
      selectors: [`[data-testid="map-${section}"]`, `[data-field-path="/spec/${section}"]`],
    };
  }

  const entityPath = `/spec/${section}/${id}`;
  const leaf = parts[3];
  const selectors = [
    leaf ? `[data-field-path="${entityPath}/${leaf}"]` : "",
    `[data-field-path="${entityPath}"]`,
    `[data-testid="${section === "services" ? "service" : section.slice(0, -1)}-${id}"]`,
  ].filter(Boolean);

  let selectNodeId: string | undefined;
  if (section === "services") selectNodeId = `service:${id}`;
  if (section === "tasks") selectNodeId = `task:${id}`;
  if (section === "resources") selectNodeId = `resource:${id}`;
  if (section === "networks") selectNodeId = `network:${id}`;
  if (section === "volumes") selectNodeId = `volume:${id}`;

  return { window: "forms", selectors, selectNodeId };
}

export function focusFieldPath(selectors: string[]): HTMLElement | null {
  for (const sel of selectors) {
    const el = document.querySelector(sel);
    if (!(el instanceof HTMLElement)) continue;
    el.scrollIntoView({ block: "nearest", behavior: scrollBehavior() });
    const focusable =
      el.matches("input,select,textarea,button")
        ? el
        : el.querySelector<HTMLElement>("input,select,textarea,button");
    (focusable ?? el).focus();
    const flashMs = fieldFlashMs();
    if (flashMs > 0) {
      el.classList.add("field-flash");
      window.setTimeout(() => el.classList.remove("field-flash"), flashMs);
    }
    return el;
  }
  return null;
}
