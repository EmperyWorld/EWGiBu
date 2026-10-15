import { autoLayoutPositions } from "./graphMap";
import {
  defaultCondition,
  dependsOnOf,
  resolveEntity,
  validateDependency,
} from "./deps";
import type {
  Database,
  DependsOn,
  Image,
  Mode,
  Network,
  PackageOpts,
  Parameter,
  Project,
  Resource,
  SecretDecl,
  Service,
  Task,
  ValueSource,
  Volume,
  XUI,
} from "./project";
import { emptyProject } from "./project";

export type Command =
  | { type: "replaceProject"; project: Project }
  | { type: "resetEmpty" }
  | { type: "setMetadata"; name?: string; version?: string; description?: string }
  | { type: "setMode"; mode: Mode }
  | { type: "upsertParameter"; id: string; value: Parameter }
  | { type: "removeParameter"; id: string }
  | { type: "upsertSecret"; id: string; value: SecretDecl }
  | { type: "removeSecret"; id: string }
  | { type: "upsertImage"; id: string; value: Image }
  | { type: "removeImage"; id: string }
  | { type: "upsertNetwork"; id: string; value?: Network }
  | { type: "removeNetwork"; id: string }
  | { type: "upsertVolume"; id: string; value: Volume }
  | { type: "removeVolume"; id: string }
  | { type: "upsertResource"; id: string; value: Resource }
  | { type: "removeResource"; id: string }
  | { type: "upsertService"; id: string; value: Service }
  | { type: "removeService"; id: string }
  | { type: "upsertTask"; id: string; value: Task }
  | { type: "removeTask"; id: string }
  | { type: "upsertDatabase"; id: string; value: Database }
  | { type: "removeDatabase"; id: string }
  | { type: "setPackage"; value: PackageOpts }
  | { type: "setXUI"; value: XUI }
  | { type: "patchUnmodeled"; path: string[]; value: unknown }
  | {
      type: "addDependency";
      /** Dependent service or task that gains dependsOn */
      service: string;
      /** Prerequisite service or task target */
      target: string;
      condition?: DependsOn["condition"];
    }
  | { type: "removeDependency"; service: string; target: string }
  | {
      type: "setDependencyCondition";
      service: string;
      target: string;
      condition: DependsOn["condition"];
    }
  | { type: "moveNode"; id: string; x: number; y: number }
  | { type: "setNodeCategory"; id: string; category: string }
  | { type: "setViewport"; viewport: NonNullable<XUI["viewport"]> }
  | { type: "applyAutoLayout" };

/** Presentation-only commands that must not invalidate Compose preview. */
export function isPresentationCommand(cmd: Command): boolean {
  return (
    cmd.type === "moveNode" ||
    cmd.type === "setNodeCategory" ||
    cmd.type === "setViewport" ||
    cmd.type === "applyAutoLayout"
  );
}

export type CommandResult = { project: Project; error?: string };

/** Apply several commands as one logical edit (caller records a single undo). */
export function applyCommands(state: Project, cmds: Command[]): CommandResult {
  let project = state;
  for (const cmd of cmds) {
    const result = applyCommand(project, cmd);
    if (result.error) return { project: state, error: result.error };
    project = result.project;
  }
  return { project };
}

function clone<T>(v: T): T {
  return structuredClone(v);
}

function omitKey<T>(m: Record<string, T>, id: string): Record<string, T> {
  return Object.fromEntries(Object.entries(m).filter(([k]) => k !== id));
}

/** Drop dependsOn / x-ui refs pointing at a removed service or task id. */
function scrubRemovedEntity(project: Project, id: string): void {
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    if (!svc.dependsOn?.length) continue;
    const dependsOn = svc.dependsOn.filter((d) => d.target !== id);
    project.spec.services[sid] = {
      ...svc,
      dependsOn: dependsOn.length ? dependsOn : undefined,
    };
  }
  if (project.spec.tasks) {
    for (const [tid, task] of Object.entries(project.spec.tasks)) {
      if (!task.dependsOn?.length) continue;
      const dependsOn = task.dependsOn.filter((d) => d.target !== id);
      project.spec.tasks[tid] = {
        ...task,
        dependsOn: dependsOn.length ? dependsOn : undefined,
      };
    }
  }
  const xuiNodes = project.spec["x-ui"]?.nodes;
  if (xuiNodes?.[id]) {
    const nodes = omitKey(xuiNodes, id);
    project.spec["x-ui"] = {
      ...project.spec["x-ui"],
      nodes: Object.keys(nodes).length ? nodes : undefined,
    };
  }
}

function scrubNetworkRefs(project: Project, networkId: string): void {
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    if (!svc.networks?.includes(networkId)) continue;
    const networks = svc.networks.filter((n) => n !== networkId);
    project.spec.services[sid] = {
      ...svc,
      networks: networks.length ? networks : undefined,
    };
  }
  if (project.spec.tasks) {
    for (const [tid, task] of Object.entries(project.spec.tasks)) {
      if (!task.networks?.includes(networkId)) continue;
      const networks = task.networks.filter((n) => n !== networkId);
      project.spec.tasks[tid] = {
        ...task,
        networks: networks.length ? networks : undefined,
      };
    }
  }
}

function scrubMountRefs(
  project: Project,
  kind: "volume" | "resource",
  refId: string,
): void {
  const filterMounts = <T extends { mounts?: Project["spec"]["services"][string]["mounts"] }>(
    entity: T,
  ): T => {
    if (!entity.mounts?.length) return entity;
    const mounts = entity.mounts.filter((m) =>
      kind === "volume" ? m.volume !== refId : m.resource !== refId,
    );
    return { ...entity, mounts: mounts.length ? mounts : undefined };
  };
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    project.spec.services[sid] = filterMounts(svc);
  }
  if (project.spec.tasks) {
    for (const [tid, task] of Object.entries(project.spec.tasks)) {
      project.spec.tasks[tid] = filterMounts(task);
    }
  }
}

function scrubSecretRefs(project: Project, secretId: string): void {
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    if (!svc.secrets?.length) continue;
    const secrets = svc.secrets.filter((s) => s.ref !== secretId);
    project.spec.services[sid] = {
      ...svc,
      secrets: secrets.length ? secrets : undefined,
    };
  }
  if (project.spec.tasks) {
    for (const [tid, task] of Object.entries(project.spec.tasks)) {
      if (!task.secrets?.length) continue;
      const secrets = task.secrets.filter((s) => s.ref !== secretId);
      project.spec.tasks[tid] = {
        ...task,
        secrets: secrets.length ? secrets : undefined,
      };
    }
  }
}

function scrubParameterFromValueSource(src: ValueSource | undefined, paramId: string): ValueSource | undefined {
  if (!src) return src;
  if ("parameter" in src && src.parameter === paramId) return undefined;
  return src;
}

function scrubParameterRefs(project: Project, paramId: string): void {
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    let changed = false;
    let environment = svc.environment;
    if (environment) {
      const nextEnv: NonNullable<typeof environment> = {};
      for (const [k, v] of Object.entries(environment)) {
        if ("parameter" in v && v.parameter === paramId) {
          changed = true;
          continue;
        }
        nextEnv[k] = v;
      }
      environment = Object.keys(nextEnv).length ? nextEnv : undefined;
    }
    let ports = svc.ports;
    if (ports?.length) {
      ports = ports.map((p) => {
        if (p.host && typeof p.host === "object" && "parameter" in p.host && p.host.parameter === paramId) {
          changed = true;
          return { ...p, host: undefined };
        }
        return p;
      });
    }
    if (changed) {
      project.spec.services[sid] = {
        ...svc,
        environment,
        ports,
      };
    }
  }
  for (const [iid, img] of Object.entries(project.spec.images)) {
    if (!("build" in img) || !img.build.args) continue;
    const args: NonNullable<typeof img.build.args> = {};
    let changed = false;
    for (const [k, v] of Object.entries(img.build.args)) {
      const kept = scrubParameterFromValueSource(v, paramId);
      if (!kept) {
        changed = true;
        continue;
      }
      args[k] = kept;
    }
    if (changed) {
      project.spec.images[iid] = {
        ...img,
        build: {
          ...img.build,
          args: Object.keys(args).length ? args : undefined,
        },
      };
    }
  }
}

function imageUsers(project: Project, imageId: string): string[] {
  const users: string[] = [];
  for (const [sid, svc] of Object.entries(project.spec.services)) {
    if (svc.image === imageId) users.push(`service:${sid}`);
  }
  for (const [tid, task] of Object.entries(project.spec.tasks ?? {})) {
    if (task.image === imageId) users.push(`task:${tid}`);
  }
  return users.sort();
}

function patchDependsOn(
  project: Project,
  id: string,
  dependsOn: DependsOn[] | undefined,
): void {
  const ent = resolveEntity(project, id);
  if (!ent) return;
  if (ent.kind === "service") {
    const svc = project.spec.services[id]!;
    project.spec.services[id] = { ...svc, dependsOn };
  } else {
    const task = project.spec.tasks?.[id];
    if (!task) return;
    project.spec.tasks = {
      ...project.spec.tasks,
      [id]: { ...task, dependsOn },
    };
  }
}

let lastGraphError = "";

export function applyCommand(state: Project, cmd: Command): CommandResult {
  lastGraphError = "";
  const project = reduceProject(state, cmd);
  if (lastGraphError) return { project: state, error: lastGraphError };
  return { project };
}

export function reduceProject(state: Project, cmd: Command): Project {
  const next = clone(state);
  switch (cmd.type) {
    case "replaceProject":
      return clone(cmd.project);
    case "resetEmpty":
      return emptyProject();
    case "setMetadata": {
      if (cmd.name !== undefined) next.metadata.name = cmd.name;
      if (cmd.version !== undefined) next.metadata.version = cmd.version;
      if (cmd.description !== undefined) {
        if (cmd.description === "") delete next.metadata.description;
        else next.metadata.description = cmd.description;
      }
      return next;
    }
    case "setMode":
      next.spec.mode = cmd.mode;
      return next;
    case "upsertParameter":
      next.spec.parameters = { ...next.spec.parameters, [cmd.id]: cmd.value };
      return next;
    case "removeParameter": {
      if (!next.spec.parameters) return next;
      const rest = omitKey(next.spec.parameters, cmd.id);
      next.spec.parameters = Object.keys(rest).length ? rest : undefined;
      scrubParameterRefs(next, cmd.id);
      return next;
    }
    case "upsertSecret":
      next.spec.secrets = { ...next.spec.secrets, [cmd.id]: cmd.value };
      return next;
    case "removeSecret": {
      if (!next.spec.secrets) return next;
      const rest = omitKey(next.spec.secrets, cmd.id);
      next.spec.secrets = Object.keys(rest).length ? rest : undefined;
      scrubSecretRefs(next, cmd.id);
      return next;
    }
    case "upsertImage":
      next.spec.images = { ...next.spec.images, [cmd.id]: cmd.value };
      return next;
    case "removeImage": {
      const users = imageUsers(next, cmd.id);
      if (users.length) {
        lastGraphError = `image is used by ${users.join(", ")}`;
        return state;
      }
      next.spec.images = omitKey(next.spec.images, cmd.id);
      return next;
    }
    case "upsertNetwork":
      next.spec.networks = { ...next.spec.networks, [cmd.id]: cmd.value ?? {} };
      return next;
    case "removeNetwork": {
      if (!next.spec.networks) return next;
      const rest = omitKey(next.spec.networks, cmd.id);
      next.spec.networks = Object.keys(rest).length ? rest : undefined;
      scrubNetworkRefs(next, cmd.id);
      return next;
    }
    case "upsertVolume":
      next.spec.volumes = { ...next.spec.volumes, [cmd.id]: cmd.value };
      return next;
    case "removeVolume": {
      if (!next.spec.volumes) return next;
      const rest = omitKey(next.spec.volumes, cmd.id);
      next.spec.volumes = Object.keys(rest).length ? rest : undefined;
      scrubMountRefs(next, "volume", cmd.id);
      return next;
    }
    case "upsertResource":
      next.spec.resources = { ...next.spec.resources, [cmd.id]: cmd.value };
      return next;
    case "removeResource": {
      if (!next.spec.resources) return next;
      const rest = omitKey(next.spec.resources, cmd.id);
      next.spec.resources = Object.keys(rest).length ? rest : undefined;
      scrubMountRefs(next, "resource", cmd.id);
      return next;
    }
    case "upsertService":
      next.spec.services = { ...next.spec.services, [cmd.id]: cmd.value };
      return next;
    case "removeService": {
      next.spec.services = omitKey(next.spec.services, cmd.id);
      scrubRemovedEntity(next, cmd.id);
      return next;
    }
    case "upsertTask":
      next.spec.tasks = { ...next.spec.tasks, [cmd.id]: cmd.value };
      return next;
    case "removeTask": {
      if (!next.spec.tasks) return next;
      const rest = omitKey(next.spec.tasks, cmd.id);
      next.spec.tasks = Object.keys(rest).length ? rest : undefined;
      scrubRemovedEntity(next, cmd.id);
      return next;
    }
    case "upsertDatabase":
      next.spec.databases = { ...next.spec.databases, [cmd.id]: cmd.value };
      return next;
    case "removeDatabase": {
      if (!next.spec.databases) return next;
      const rest = omitKey(next.spec.databases, cmd.id);
      next.spec.databases = Object.keys(rest).length ? rest : undefined;
      return next;
    }
    case "setPackage":
      next.spec.package = cmd.value;
      return next;
    case "setXUI":
      next.spec["x-ui"] = cmd.value;
      return next;
    case "addDependency": {
      const depEnt = resolveEntity(next, cmd.service);
      const prereqEnt = resolveEntity(next, cmd.target);
      const condition =
        cmd.condition ?? (prereqEnt ? defaultCondition(prereqEnt) : "started");
      const err = validateDependency(next, cmd.service, cmd.target, condition);
      if (err) {
        lastGraphError = err;
        return state;
      }
      if (!depEnt) {
        lastGraphError = "unknown dependent";
        return state;
      }
      const existing = dependsOnOf(next, cmd.service);
      const entry: DependsOn = { target: cmd.target, condition };
      if (depEnt.kind === "service") {
        const svc = next.spec.services[cmd.service]!;
        next.spec.services[cmd.service] = { ...svc, dependsOn: [...existing, entry] };
      } else {
        const task = next.spec.tasks?.[cmd.service];
        if (!task) {
          lastGraphError = "unknown dependent task";
          return state;
        }
        next.spec.tasks = {
          ...next.spec.tasks,
          [cmd.service]: { ...task, dependsOn: [...existing, entry] },
        };
      }
      return next;
    }
    case "removeDependency": {
      const depEnt = resolveEntity(next, cmd.service);
      if (!depEnt) return next;
      const filtered = dependsOnOf(next, cmd.service).filter((d) => d.target !== cmd.target);
      patchDependsOn(next, cmd.service, filtered.length ? filtered : undefined);
      return next;
    }
    case "setDependencyCondition": {
      const existing = dependsOnOf(next, cmd.service);
      if (!existing.some((d) => d.target === cmd.target)) {
        lastGraphError = "dependency not found";
        return state;
      }
      const err = validateDependency(next, cmd.service, cmd.target, cmd.condition, {
        allowExisting: true,
      });
      if (err) {
        lastGraphError = err;
        return state;
      }
      patchDependsOn(
        next,
        cmd.service,
        existing.map((d) =>
          d.target === cmd.target ? { ...d, condition: cmd.condition } : d,
        ),
      );
      return next;
    }
    case "moveNode": {
      // x-ui only stores service/task positions (TZ §9.13); skip resources.
      if (!resolveEntity(next, cmd.id)) return next;
      const xui: XUI = { ...(next.spec["x-ui"] ?? {}), nodes: { ...(next.spec["x-ui"]?.nodes ?? {}) } };
      const prev = xui.nodes?.[cmd.id] ?? { x: 0, y: 0 };
      xui.nodes = { ...xui.nodes, [cmd.id]: { ...prev, x: cmd.x, y: cmd.y } };
      next.spec["x-ui"] = xui;
      return next;
    }
    case "setNodeCategory": {
      if (!resolveEntity(next, cmd.id)) return next;
      const category = cmd.category === "other" ? "custom" : cmd.category;
      const xui: XUI = { ...(next.spec["x-ui"] ?? {}), nodes: { ...(next.spec["x-ui"]?.nodes ?? {}) } };
      const prev = xui.nodes?.[cmd.id] ?? { x: 0, y: 0 };
      xui.nodes = { ...xui.nodes, [cmd.id]: { ...prev, category } };
      next.spec["x-ui"] = xui;
      return next;
    }
    case "setViewport": {
      next.spec["x-ui"] = { ...(next.spec["x-ui"] ?? {}), viewport: cmd.viewport };
      return next;
    }
    case "applyAutoLayout": {
      const positions = autoLayoutPositions(next);
      const nodes: NonNullable<XUI["nodes"]> = { ...(next.spec["x-ui"]?.nodes ?? {}) };
      for (const [id, pos] of Object.entries(positions)) {
        if (!resolveEntity(next, id)) continue;
        nodes[id] = { ...nodes[id], ...pos };
      }
      next.spec["x-ui"] = { ...(next.spec["x-ui"] ?? {}), nodes };
      return next;
    }
    case "patchUnmodeled": {
      // Preserve unknown nested fields under a map path (e.g. service extras).
      let cur: Record<string, unknown> = next as unknown as Record<string, unknown>;
      for (let i = 0; i < cmd.path.length - 1; i++) {
        const key = cmd.path[i];
        if (key === undefined) return next;
        const child = cur[key];
        if (!child || typeof child !== "object" || Array.isArray(child)) {
          cur[key] = {};
        }
        cur = cur[key] as Record<string, unknown>;
      }
      const last = cmd.path[cmd.path.length - 1];
      if (last !== undefined) cur[last] = cmd.value;
      return next;
    }
    default: {
      const _exhaustive: never = cmd;
      return _exhaustive;
    }
  }
}
