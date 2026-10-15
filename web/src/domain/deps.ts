import type { DependsOn, Project, Task } from "./project";

export type EntityKind = "service" | "task";

export type GraphEntity = {
  id: string;
  kind: EntityKind;
  phase?: Task["phase"];
};

export type DepRow = {
  /** Dependent entity id (service or task). */
  dependent: string;
  dependentKind: EntityKind;
  /** Prerequisite entity id. */
  target: string;
  targetKind: EntityKind;
  condition: NonNullable<DependsOn["condition"]>;
};

const PHASE_ORDER: Record<Task["phase"], number> = {
  preStart: 0,
  migration: 1,
  postDeploy: 2,
};

export function resolveEntity(project: Project, id: string): GraphEntity | null {
  if (project.spec.services[id]) return { id, kind: "service" };
  const task = project.spec.tasks?.[id];
  if (task) return { id, kind: "task", phase: task.phase };
  return null;
}

export function allEntityIds(project: Project): string[] {
  return [...Object.keys(project.spec.services), ...Object.keys(project.spec.tasks ?? {})].sort();
}

/** Default condition for a new edge given the prerequisite kind. */
export function defaultCondition(prereq: GraphEntity): DependsOn["condition"] {
  return prereq.kind === "task" ? "completedSuccessfully" : "started";
}

/**
 * Allowed prerequisite ids for a dependent, per TZ §9.11 phase partial order.
 * Does not filter cycles/self — callers still validate those on connect.
 */
export function allowedPrerequisiteIds(project: Project, dependentId: string): string[] {
  const dep = resolveEntity(project, dependentId);
  if (!dep) return [];
  const out: string[] = [];
  for (const id of allEntityIds(project)) {
    if (id === dependentId) continue;
    const prereq = resolveEntity(project, id);
    if (!prereq) continue;
    if (phaseAllows(dep, prereq) === null) out.push(id);
  }
  return out;
}

/** Returns an error message if the phase matrix forbids this edge; null if ok. */
export function phaseAllows(dependent: GraphEntity, prereq: GraphEntity): string | null {
  if (dependent.kind === "service") {
    if (prereq.kind === "task" && (prereq.phase === "migration" || prereq.phase === "postDeploy")) {
      return `service cannot depend on ${prereq.phase} task`;
    }
    return null;
  }
  // dependent is task
  const phase = dependent.phase ?? "preStart";
  switch (phase) {
    case "preStart":
      if (prereq.kind === "service") return "preStart task cannot depend on a service";
      if (prereq.phase !== "preStart") return "preStart task may only depend on other preStart tasks";
      return null;
    case "migration":
      if (prereq.kind === "task" && prereq.phase === "postDeploy") {
        return "migration task cannot depend on postDeploy";
      }
      if (prereq.kind === "task" && prereq.phase === "preStart") {
        return "migration task cannot depend on preStart task (use services)";
      }
      return null;
    case "postDeploy":
      if (prereq.kind === "task" && prereq.phase === "preStart") {
        return "postDeploy task cannot depend on preStart task";
      }
      return null;
    default:
      return null;
  }
}

/** Returns error if condition is invalid for the given prerequisite; null if ok. */
export function conditionAllows(
  project: Project,
  prereq: GraphEntity,
  condition: DependsOn["condition"],
): string | null {
  const cond = condition ?? "started";
  if (cond === "healthy") {
    if (prereq.kind !== "service") return "healthy condition requires a service target";
    if (!project.spec.services[prereq.id]?.healthcheck) {
      return "healthy condition requires a healthcheck on the target";
    }
    return null;
  }
  if (cond === "completedSuccessfully") {
    if (prereq.kind !== "task") return "completedSuccessfully requires a task target";
    return null;
  }
  // started — allowed for service; also allowed for task per compile graph
  return null;
}

/** Full client-side validation mirroring TZ §9.11 (UX gate; server remains authoritative). */
export function validateDependency(
  project: Project,
  dependentId: string,
  prereqId: string,
  condition?: DependsOn["condition"],
  opts?: { allowExisting?: boolean },
): string | null {
  const dependent = resolveEntity(project, dependentId);
  if (!dependent) return "unknown dependent";
  const prereq = resolveEntity(project, prereqId);
  if (!prereq) return "unknown prerequisite";
  if (dependentId === prereqId) return "self-dependency is not allowed";

  const phaseErr = phaseAllows(dependent, prereq);
  if (phaseErr) return phaseErr;

  const cond = condition ?? defaultCondition(prereq);
  const condErr = conditionAllows(project, prereq, cond);
  if (condErr) return condErr;

  if (cond === "completedSuccessfully" && dependent.kind === "task" && prereq.kind === "task") {
    const depPhase = dependent.phase ?? "preStart";
    const prePhase = prereq.phase ?? "preStart";
    if (PHASE_ORDER[prePhase] > PHASE_ORDER[depPhase]) {
      return "task cannot require completedSuccessfully of a later-phase task";
    }
  }

  const existing = dependsOnOf(project, dependentId);
  if (!opts?.allowExisting && existing.some((d) => d.target === prereqId)) {
    return "duplicate dependency edge";
  }

  if (!opts?.allowExisting && wouldCreateCycle(project, prereqId, dependentId)) {
    return "dependency would create a cycle";
  }
  // When updating an existing edge, the edge is already in the graph — cycle check
  // would see the existing edge; skip unless we are adding.
  if (opts?.allowExisting) {
    // still forbid introducing a cycle only if somehow the edge didn't exist
    // (no-op for updates).
  }
  return null;
}

export function dependsOnOf(project: Project, id: string): DependsOn[] {
  return project.spec.services[id]?.dependsOn ?? project.spec.tasks?.[id]?.dependsOn ?? [];
}

/** Adjacency: prerequisite id → list of dependent ids. */
export function dependencyAdjacency(project: Project): Map<string, string[]> {
  const adj = new Map<string, string[]>();
  for (const id of allEntityIds(project)) adj.set(id, []);
  for (const id of allEntityIds(project)) {
    for (const dep of dependsOnOf(project, id)) {
      const list = adj.get(dep.target) ?? [];
      list.push(id);
      adj.set(dep.target, list);
    }
  }
  return adj;
}

/** True if adding edge prerequisite → dependent would create a cycle. */
export function wouldCreateCycle(project: Project, prerequisite: string, dependent: string): boolean {
  if (prerequisite === dependent) return true;
  const adj = dependencyAdjacency(project);
  const list = adj.get(prerequisite) ?? [];
  adj.set(prerequisite, [...list, dependent]);
  const seen = new Set<string>();
  const stack = [dependent];
  while (stack.length) {
    const cur = stack.pop();
    if (cur === undefined) break;
    if (cur === prerequisite) return true;
    if (seen.has(cur)) continue;
    seen.add(cur);
    for (const n of adj.get(cur) ?? []) stack.push(n);
  }
  return false;
}

export function listDependencies(project: Project): DepRow[] {
  const rows: DepRow[] = [];
  for (const id of Object.keys(project.spec.services).sort()) {
    const svc = project.spec.services[id];
    if (!svc) continue;
    for (const d of svc.dependsOn ?? []) {
      const target = resolveEntity(project, d.target);
      rows.push({
        dependent: id,
        dependentKind: "service",
        target: d.target,
        targetKind: target?.kind ?? "service",
        condition: d.condition ?? "started",
      });
    }
  }
  for (const id of Object.keys(project.spec.tasks ?? {}).sort()) {
    const task = project.spec.tasks?.[id];
    if (!task) continue;
    for (const d of task.dependsOn ?? []) {
      const target = resolveEntity(project, d.target);
      rows.push({
        dependent: id,
        dependentKind: "task",
        target: d.target,
        targetKind: target?.kind ?? "service",
        condition: d.condition ?? "started",
      });
    }
  }
  return rows;
}
