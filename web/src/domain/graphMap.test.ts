import { describe, expect, it } from "vitest";
import { applyCommand, reduceProject } from "./commands";
import { autoLayoutPositions, mapProjectGraph, wouldCreateCycle } from "./graphMap";
import { emptyProject } from "./project";

describe("graphMap", () => {
  it("maps services, tasks, and dependency edges prerequisite -> dependent", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertTask",
      id: "warmup",
      value: { phase: "preStart", image: "web", command: ["true"], rerun: "always" },
    });
    p = applyCommand(p, {
      type: "addDependency",
      service: "web",
      target: "warmup",
      condition: "completedSuccessfully",
    }).project;
    const g = mapProjectGraph(p, { informational: false });
    expect(g.nodes.map((n) => n.id).sort()).toEqual([
      "service:api",
      "service:web",
      "task:warmup",
    ]);
    expect(g.edges.every((e) => e.kind === "dependency")).toBe(true);
    expect(g.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "dependency",
          source: "service:api",
          target: "service:web",
          condition: "started",
        }),
        expect.objectContaining({
          kind: "dependency",
          source: "task:warmup",
          target: "service:web",
          condition: "completedSuccessfully",
        }),
      ]),
    );
  });

  it("maps informational network edges when enabled", () => {
    const p = emptyProject();
    const g = mapProjectGraph(p, { informational: true });
    expect(g.nodes.map((n) => n.id).sort()).toContain("network:frontend");
    expect(g.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "network",
          source: "network:frontend",
          target: "service:web",
        }),
      ]),
    );
  });

  it("detects cycles for client UX", () => {
    const p = emptyProject();
    // web -> api already; adding api -> web cycles
    expect(wouldCreateCycle(p, "web", "api")).toBe(true);
    expect(wouldCreateCycle(p, "api", "web")).toBe(false); // already exists direction
  });

  it("places mounts beside their service and networks to the left of that group", () => {
    const p = emptyProject();
    p.spec.resources = { cfg: { source: "cfg" } };
    p.spec.services.web = {
      ...p.spec.services.web!,
      mounts: [{ resource: "cfg", target: "/cfg", readOnly: true }],
    };
    const g = mapProjectGraph(p, { informational: true });
    const web = g.nodes.find((n) => n.id === "service:web");
    const res = g.nodes.find((n) => n.id === "resource:cfg");
    const net = g.nodes.find((n) => n.id === "network:frontend");
    expect(web && res && net).toBeTruthy();
    if (!web || !res || !net) return;
    expect(res.x).toBeGreaterThan(web.x);
    expect(Math.abs(res.y - web.y)).toBeLessThan(40);
    expect(net.x).toBeLessThan(web.x);
    expect(Math.abs(net.y - web.y)).toBeLessThan(200);
  });

  it("auto-layout is deterministic", () => {
    const p = emptyProject();
    expect(autoLayoutPositions(p)).toEqual(autoLayoutPositions(structuredClone(p)));
  });
});

describe("dependency commands", () => {
  it("adds and removes exactly one dependsOn entry", () => {
    let p = emptyProject();
    p = reduceProject(p, { type: "removeDependency", service: "web", target: "api" });
    expect(p.spec.services.web?.dependsOn ?? []).toEqual([]);
    const added = applyCommand(p, {
      type: "addDependency",
      service: "web",
      target: "api",
      condition: "started",
    });
    expect(added.error).toBeUndefined();
    expect(added.project.spec.services.web?.dependsOn).toEqual([{ target: "api", condition: "started" }]);
  });

  it("rejects self, duplicate, and cycle edges", () => {
    const p = emptyProject();
    expect(applyCommand(p, { type: "addDependency", service: "web", target: "web" }).error).toMatch(/self/);
    expect(applyCommand(p, { type: "addDependency", service: "web", target: "api" }).error).toMatch(/duplicate/);
    expect(applyCommand(p, { type: "addDependency", service: "api", target: "web" }).error).toMatch(/cycle/);
  });

  it("moveNode only touches x-ui", () => {
    const p = emptyProject();
    const beforeServices = JSON.stringify(p.spec.services);
    const next = reduceProject(p, { type: "moveNode", id: "web", x: 11, y: 22 });
    expect(JSON.stringify(next.spec.services)).toBe(beforeServices);
    expect(next.spec["x-ui"]?.nodes?.web).toEqual(expect.objectContaining({ x: 11, y: 22 }));
  });
});
