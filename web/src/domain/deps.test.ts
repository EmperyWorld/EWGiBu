import { describe, expect, it } from "vitest";
import {
  allowedPrerequisiteIds,
  defaultCondition,
  validateDependency,
  wouldCreateCycle,
} from "./deps";
import { applyCommand, reduceProject } from "./commands";
import { emptyProject } from "./project";

function withTasks() {
  let p = emptyProject();
  p = reduceProject(p, {
    type: "upsertTask",
    id: "warmup",
    value: { phase: "preStart", image: "web", command: ["true"], rerun: "always" },
  });
  p = reduceProject(p, {
    type: "upsertTask",
    id: "migrate",
    value: { phase: "migration", image: "web", command: ["true"], rerun: "oncePerDeployment" },
  });
  return p;
}

describe("deps phase matrix", () => {
  it("allows service → preStart task and rejects service → migration", () => {
    const p = withTasks();
    expect(validateDependency(p, "web", "warmup", "completedSuccessfully")).toBeNull();
    expect(validateDependency(p, "web", "migrate", "completedSuccessfully")).toMatch(/migration/);
  });

  it("rejects preStart task → service", () => {
    const p = withTasks();
    expect(validateDependency(p, "warmup", "web", "started")).toMatch(/preStart/);
  });

  it("allows migration → service with started", () => {
    const p = withTasks();
    expect(validateDependency(p, "migrate", "api", "started")).toBeNull();
  });

  it("lists allowed prerequisites for a service including preStart tasks", () => {
    const p = withTasks();
    expect(allowedPrerequisiteIds(p, "web").sort()).toEqual(["api", "warmup"]);
  });

  it("defaults condition by prerequisite kind", () => {
    expect(defaultCondition({ id: "api", kind: "service" })).toBe("started");
    expect(defaultCondition({ id: "warmup", kind: "task", phase: "preStart" })).toBe(
      "completedSuccessfully",
    );
  });
});

describe("deps cycle across tasks", () => {
  it("detects cycles involving tasks", () => {
    let p = withTasks();
    p = applyCommand(p, {
      type: "addDependency",
      service: "migrate",
      target: "api",
      condition: "started",
    }).project;
    // migrate → api exists; api → migrate would cycle
    expect(wouldCreateCycle(p, "migrate", "api")).toBe(true);
    expect(wouldCreateCycle(p, "api", "web")).toBe(false);
  });
});

describe("addDependency for tasks", () => {
  it("writes dependsOn on the task", () => {
    const p = withTasks();
    const r = applyCommand(p, {
      type: "addDependency",
      service: "migrate",
      target: "api",
      condition: "started",
    });
    expect(r.error).toBeUndefined();
    expect(r.project.spec.tasks?.migrate?.dependsOn).toEqual([
      { target: "api", condition: "started" },
    ]);
  });

  it("rejects phase-invalid edge with announcement error", () => {
    const p = withTasks();
    const r = applyCommand(p, {
      type: "addDependency",
      service: "warmup",
      target: "web",
      condition: "started",
    });
    expect(r.error).toMatch(/preStart/);
    expect(r.project.spec.tasks?.warmup?.dependsOn).toBeUndefined();
  });
});
