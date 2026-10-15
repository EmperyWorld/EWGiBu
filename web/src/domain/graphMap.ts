import type { Project, XUI } from "./project";
import { listDependencies, resolveEntity, wouldCreateCycle } from "./deps";

export type NodeKind = "service" | "task" | "resource" | "volume" | "network";
export type EdgeKind = "dependency" | "network" | "volume" | "resource";
export type Category = "app" | "database" | "proxy" | "cache" | "custom";

export type GraphNode = {
  id: string;
  kind: NodeKind;
  label: string;
  category: Category;
  x: number;
  y: number;
  /** False for informational infrastructure nodes. */
  semantic: boolean;
};

export type GraphEdge = {
  id: string;
  kind: EdgeKind;
  /** Prerequisite / source (or attachment source for info edges) */
  source: string;
  /** Dependent / target (or consumer for info edges) */
  target: string;
  condition?: string;
  label: string;
};

export type MappedGraph = {
  nodes: GraphNode[];
  edges: GraphEdge[];
};

export type MapGraphOptions = {
  /** Include network / volume / resource attachment edges (default true). */
  informational?: boolean;
};

const COL_W = 220;
const ROW_H = 120;

export function nodeId(kind: NodeKind, name: string): string {
  return `${kind}:${name}`;
}

export function parseNodeId(id: string): { kind: NodeKind; name: string } | null {
  const i = id.indexOf(":");
  if (i <= 0) return null;
  const kind = id.slice(0, i);
  const name = id.slice(i + 1);
  if (
    kind !== "service" &&
    kind !== "task" &&
    kind !== "resource" &&
    kind !== "volume" &&
    kind !== "network"
  ) {
    return null;
  }
  if (!name) return null;
  return { kind, name };
}

export function isSemanticKind(kind: NodeKind): kind is "service" | "task" {
  return kind === "service" || kind === "task";
}

function categoryOf(xui: XUI | undefined, name: string): Category {
  const c = xui?.nodes?.[name]?.category;
  if (c === "app" || c === "database" || c === "proxy" || c === "cache" || c === "custom") return c;
  if (c === "other") return "custom";
  return "custom";
}

/** Deterministic auto-layout positions by kind then name. */
export function autoLayoutPositions(project: Project): Record<string, { x: number; y: number }> {
  const services = Object.keys(project.spec.services).sort();
  const tasks = Object.keys(project.spec.tasks ?? {}).sort();
  const resources = Object.keys(project.spec.resources ?? {}).sort();
  const volumes = Object.keys(project.spec.volumes ?? {}).sort();
  const networks = Object.keys(project.spec.networks ?? {}).sort();
  const out: Record<string, { x: number; y: number }> = {};
  services.forEach((name, i) => {
    out[name] = { x: (i % 4) * COL_W, y: Math.floor(i / 4) * ROW_H };
  });
  tasks.forEach((name, i) => {
    out[name] = { x: (i % 4) * COL_W, y: 280 + Math.floor(i / 4) * ROW_H };
  });
  networks.forEach((name, i) => {
    out[`network:${name}`] = { x: (i % 4) * COL_W, y: 440 + Math.floor(i / 4) * ROW_H };
  });
  volumes.forEach((name, i) => {
    out[`volume:${name}`] = { x: (i % 4) * COL_W, y: 560 + Math.floor(i / 4) * ROW_H };
  });
  resources.forEach((name, i) => {
    out[`resource:${name}`] = { x: (i % 4) * COL_W, y: 680 + Math.floor(i / 4) * ROW_H };
  });
  return out;
}

const CARD_W = 200;
const CARD_H = 84;
const CARD_GAP = 22;

type Pos = { x: number; y: number };

function finitePos(node: { x?: number; y?: number } | undefined): Pos | null {
  if (!node || !Number.isFinite(node.x) || !Number.isFinite(node.y)) return null;
  return { x: node.x as number, y: node.y as number };
}

function overlaps(a: Pos, b: Pos): boolean {
  return Math.abs(a.x - b.x) < CARD_W - 8 && Math.abs(a.y - b.y) < CARD_H - 8;
}

function placeFree(prefer: Pos, taken: Pos[], bias: "left" | "right" | "up"): Pos {
  const hits = (p: Pos) => taken.some((q) => overlaps(p, q));
  if (!hits(prefer)) return prefer;
  const order =
    bias === "left"
      ? [
          { x: -1, y: 0 },
          { x: -1, y: -1 },
          { x: -1, y: 1 },
          { x: 0, y: -1 },
          { x: 0, y: 1 },
        ]
      : bias === "up"
        ? [
            { x: 0, y: -1 },
            { x: -1, y: -1 },
            { x: 1, y: -1 },
            { x: -1, y: 0 },
            { x: 1, y: 0 },
          ]
        : [
            { x: 1, y: 0 },
            { x: 1, y: -1 },
            { x: 1, y: 1 },
            { x: 0, y: 1 },
            { x: 0, y: -1 },
          ];
  for (let ring = 1; ring <= 18; ring++) {
    for (const d of order) {
      const p = {
        x: prefer.x + d.x * ring * (CARD_W + CARD_GAP),
        y: prefer.y + d.y * ring * (CARD_H + 12),
      };
      if (!hits(p)) return p;
    }
  }
  return { x: prefer.x, y: prefer.y + (taken.length + 1) * 12 };
}

type Consumer = {
  id: string;
  networks: string[];
  resources: string[];
  volumes: string[];
};

function consumersOf(project: Project): Consumer[] {
  const out: Consumer[] = [];
  const push = (id: string, def: { networks?: string[]; mounts?: { volume?: string; resource?: string }[] } | undefined) => {
    if (!def) return;
    const resources: string[] = [];
    const volumes: string[] = [];
    for (const mount of def.mounts ?? []) {
      if (mount.resource && !resources.includes(mount.resource)) resources.push(mount.resource);
      if (mount.volume && !volumes.includes(mount.volume)) volumes.push(mount.volume);
    }
    resources.sort();
    volumes.sort();
    out.push({ id, networks: [...(def.networks ?? [])].sort(), resources, volumes });
  };
  for (const id of Object.keys(project.spec.services).sort()) push(id, project.spec.services[id]);
  for (const id of Object.keys(project.spec.tasks ?? {}).sort()) push(id, project.spec.tasks?.[id]);
  return out;
}

function mountGrid(count: number): { cols: number; rows: number } {
  if (count <= 0) return { cols: 0, rows: 1 };
  if (count <= 3) return { cols: 1, rows: count };
  if (count <= 8) return { cols: 2, rows: Math.ceil(count / 2) };
  return { cols: 3, rows: Math.ceil(count / 3) };
}

/** Service/task positions that leave a gutter for each service's own mounts. */
function clusterServicePositions(project: Project): Record<string, Pos> {
  const items = consumersOf(project);
  const pos: Record<string, Pos> = {};
  const maxRow = 1320;
  let x = 0;
  let y = 0;
  let rowH = 0;
  for (const item of items) {
    const mounts = item.volumes.length + item.resources.length;
    const grid = mountGrid(mounts);
    const width = CARD_W + (mounts ? grid.cols * (CARD_W + 16) + CARD_GAP : 0);
    const height = Math.max(CARD_H, grid.rows * (CARD_H + 12));
    if (x > 0 && x + width > maxRow) {
      x = 0;
      y += rowH + 72;
      rowH = 0;
    }
    pos[item.id] = { x, y };
    x += width + 56;
    rowH = Math.max(rowH, height);
  }
  return pos;
}

/**
 * Networks sit to the left of the services that use them.
 * Volumes and resources sit to the right of the service that mounts them.
 * A shared mount sits between its consumers.
 */
function layoutAttachments(project: Project, anchors: Record<string, Pos>): Record<string, Pos> {
  const out: Record<string, Pos> = {};
  const taken: Pos[] = Object.values(anchors).map((p) => ({ ...p }));
  const items = consumersOf(project);

  const resourceOwners = new Map<string, string[]>();
  const volumeOwners = new Map<string, string[]>();
  for (const item of items) {
    for (const name of item.resources) {
      const list = resourceOwners.get(name) ?? [];
      list.push(item.id);
      resourceOwners.set(name, list);
    }
    for (const name of item.volumes) {
      const list = volumeOwners.get(name) ?? [];
      list.push(item.id);
      volumeOwners.set(name, list);
    }
  }

  for (const item of items) {
    const anchor = anchors[item.id];
    if (!anchor) continue;
    const owned = [
      ...item.volumes.filter((name) => volumeOwners.get(name)?.length === 1).map((name) => ({ kind: "volume" as const, name })),
      ...item.resources
        .filter((name) => resourceOwners.get(name)?.length === 1)
        .map((name) => ({ kind: "resource" as const, name })),
    ];
    const grid = mountGrid(owned.length);
    owned.forEach((mount, i) => {
      const col = grid.cols ? i % grid.cols : 0;
      const row = grid.cols ? Math.floor(i / grid.cols) : 0;
      const prefer = {
        x: anchor.x + CARD_W + CARD_GAP + col * (CARD_W + 16),
        y: anchor.y + row * (CARD_H + 12),
      };
      const spot = placeFree(prefer, taken, "right");
      out[`${mount.kind}:${mount.name}`] = spot;
      taken.push(spot);
    });
  }

  const placeShared = (kind: "volume" | "resource", owners: Map<string, string[]>) => {
    for (const name of [...owners.keys()].sort()) {
      const ids = owners.get(name) ?? [];
      if (ids.length < 2) continue;
      const pts = ids.map((id) => anchors[id]).filter((p): p is Pos => Boolean(p));
      if (!pts.length) continue;
      const prefer = {
        x: pts.reduce((s, p) => s + p.x, 0) / pts.length,
        y: Math.min(...pts.map((p) => p.y)) - (CARD_H + 16),
      };
      const spot = placeFree(prefer, taken, "up");
      out[`${kind}:${name}`] = spot;
      taken.push(spot);
    }
  };
  placeShared("volume", volumeOwners);
  placeShared("resource", resourceOwners);

  const byNet = new Map<string, Pos[]>();
  for (const item of items) {
    const anchor = anchors[item.id];
    if (!anchor) continue;
    for (const net of item.networks) {
      const list = byNet.get(net) ?? [];
      list.push(anchor);
      byNet.set(net, list);
    }
  }
  for (const net of [...byNet.keys()].sort()) {
    const pts = byNet.get(net) ?? [];
    const prefer = {
      x: Math.min(...pts.map((p) => p.x)) - (CARD_W + CARD_GAP),
      y: pts.reduce((s, p) => s + p.y, 0) / pts.length,
    };
    const spot = placeFree(prefer, taken, "left");
    out[`network:${net}`] = spot;
    taken.push(spot);
  }

  const bottom = taken.reduce((m, p) => Math.max(m, p.y), 0) + CARD_H + 48;
  let rail = 0;
  const park = (key: string) => {
    if (out[key]) return;
    const spot = placeFree({ x: (rail % 4) * (CARD_W + 16), y: bottom + Math.floor(rail / 4) * (CARD_H + 12) }, taken, "right");
    rail += 1;
    out[key] = spot;
    taken.push(spot);
  };
  for (const name of Object.keys(project.spec.networks ?? {}).sort()) park(`network:${name}`);
  for (const name of Object.keys(project.spec.volumes ?? {}).sort()) park(`volume:${name}`);
  for (const name of Object.keys(project.spec.resources ?? {}).sort()) park(`resource:${name}`);
  return out;
}

function pushInfoNode(
  nodes: GraphNode[],
  layout: Record<string, { x: number; y: number }>,
  kind: "network" | "volume" | "resource",
  name: string,
): void {
  const key = `${kind}:${name}`;
  const pos = layout[key] ?? { x: 0, y: 0 };
  nodes.push({
    id: nodeId(kind, name),
    kind,
    label: name,
    category: "custom",
    x: pos.x,
    y: pos.y,
    semantic: false,
  });
}

/** Attachment edges from services/tasks to networks, volumes, and resources. */
export function mapInformationalEdges(project: Project): GraphEdge[] {
  const edges: GraphEdge[] = [];
  const consumers: Array<{ kind: "service" | "task"; id: string }> = [
    ...Object.keys(project.spec.services)
      .sort()
      .map((id) => ({ kind: "service" as const, id })),
    ...Object.keys(project.spec.tasks ?? {})
      .sort()
      .map((id) => ({ kind: "task" as const, id })),
  ];

  for (const c of consumers) {
    const def =
      c.kind === "service" ? project.spec.services[c.id] : project.spec.tasks?.[c.id];
    if (!def) continue;
    const consumerId = nodeId(c.kind, c.id);
    for (const net of def.networks ?? []) {
      edges.push({
        id: `net:${c.kind}:${c.id}:${net}`,
        kind: "network",
        source: nodeId("network", net),
        target: consumerId,
        label: `network:${net}`,
      });
    }
    for (const mount of def.mounts ?? []) {
      if (mount.volume) {
        edges.push({
          id: `vol:${c.kind}:${c.id}:${mount.volume}:${mount.target}`,
          kind: "volume",
          source: nodeId("volume", mount.volume),
          target: consumerId,
          label: `volume:${mount.volume}`,
        });
      }
      if (mount.resource) {
        edges.push({
          id: `res:${c.kind}:${c.id}:${mount.resource}:${mount.target}`,
          kind: "resource",
          source: nodeId("resource", mount.resource),
          target: consumerId,
          label: `resource:${mount.resource}`,
        });
      }
    }
  }
  return edges;
}

export function mapProjectGraph(project: Project, opts: MapGraphOptions = {}): MappedGraph {
  const informational = opts.informational !== false;
  const xui = project.spec["x-ui"];
  const auto = autoLayoutPositions(project);
  const clustered = informational ? clusterServicePositions(project) : null;
  const semanticPos = (name: string): Pos => {
    const saved = finitePos(xui?.nodes?.[name]);
    if (saved) return saved;
    return clustered?.[name] ?? auto[name] ?? { x: 0, y: 0 };
  };
  const anchors: Record<string, Pos> = {};
  for (const name of Object.keys(project.spec.services)) anchors[name] = semanticPos(name);
  for (const name of Object.keys(project.spec.tasks ?? {})) anchors[name] = semanticPos(name);
  const attached = informational ? layoutAttachments(project, anchors) : {};
  const nodes: GraphNode[] = [];

  for (const name of Object.keys(project.spec.services).sort()) {
    const pos = anchors[name] ?? { x: 0, y: 0 };
    nodes.push({
      id: nodeId("service", name),
      kind: "service",
      label: name,
      category: categoryOf(xui, name),
      x: pos.x,
      y: pos.y,
      semantic: true,
    });
  }
  for (const name of Object.keys(project.spec.tasks ?? {}).sort()) {
    const pos = anchors[name] ?? { x: 0, y: 0 };
    nodes.push({
      id: nodeId("task", name),
      kind: "task",
      label: name,
      category: categoryOf(xui, name),
      x: pos.x,
      y: pos.y,
      semantic: true,
    });
  }

  if (informational) {
    for (const name of Object.keys(project.spec.networks ?? {}).sort()) {
      pushInfoNode(nodes, attached, "network", name);
    }
    for (const name of Object.keys(project.spec.volumes ?? {}).sort()) {
      pushInfoNode(nodes, attached, "volume", name);
    }
    for (const name of Object.keys(project.spec.resources ?? {}).sort()) {
      pushInfoNode(nodes, attached, "resource", name);
    }
  }

  const edges: GraphEdge[] = [];
  for (const row of listDependencies(project)) {
    const srcKind = resolveEntity(project, row.target)?.kind ?? row.targetKind;
    const tgtKind = resolveEntity(project, row.dependent)?.kind ?? row.dependentKind;
    edges.push({
      id: `dep:${row.target}->${row.dependent}:${row.condition}`,
      kind: "dependency",
      source: nodeId(srcKind, row.target),
      target: nodeId(tgtKind, row.dependent),
      condition: row.condition,
      label: row.condition,
    });
  }

  if (informational) {
    edges.push(...mapInformationalEdges(project));
  }

  return { nodes, edges };
}

export { listDependencies, wouldCreateCycle };
export type { DepRow } from "./deps";
