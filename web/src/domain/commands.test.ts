import { describe, expect, it } from "vitest";
import { applyCommand, applyCommands, reduceProject } from "./commands";
import { emptyProject } from "./project";

describe("reduceProject", () => {
  it("updates metadata without dropping services", () => {
    const p = emptyProject();
    const next = reduceProject(p, { type: "setMetadata", name: "shop", version: "1.0.0" });
    expect(next.metadata.name).toBe("shop");
    expect(Object.keys(next.spec.services)).toEqual(["web", "api"]);
  });

  it("preserves unmodeled service fields through upsert of known fields", () => {
    const p = emptyProject();
    p.spec.services.web = {
      ...p.spec.services.web!,
      image: "web",
      customExtra: "keep-me",
    };
    const next = reduceProject(p, {
      type: "upsertService",
      id: "web",
      value: { ...p.spec.services.web!, ports: [{ container: 80, host: 9090 }] },
    });
    expect(next.spec.services.web?.customExtra).toBe("keep-me");
    expect(next.spec.services.web?.ports?.[0]?.host).toBe(9090);
  });

  it("adds and removes a second service for two-service workflow", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertService",
      id: "cache",
      value: { image: "api", networks: ["frontend"] },
    });
    expect(Object.keys(p.spec.services).sort()).toEqual(["api", "cache", "web"]);
    p = reduceProject(p, { type: "removeService", id: "cache" });
    expect(Object.keys(p.spec.services).sort()).toEqual(["api", "web"]);
  });

  it("removeService scrubs inbound dependsOn and x-ui node", () => {
    let p = emptyProject();
    expect(p.spec.services.web?.dependsOn?.[0]?.target).toBe("api");
    p = reduceProject(p, { type: "moveNode", id: "api", x: 9, y: 9 });
    p = reduceProject(p, { type: "removeService", id: "api" });
    expect(p.spec.services.api).toBeUndefined();
    expect(p.spec.services.web?.dependsOn).toBeUndefined();
    expect(p.spec["x-ui"]?.nodes?.api).toBeUndefined();
  });

  it("removeNetwork scrubs service network refs", () => {
    let p = emptyProject();
    expect(p.spec.services.web?.networks).toContain("frontend");
    p = reduceProject(p, { type: "removeNetwork", id: "frontend" });
    expect(p.spec.networks?.frontend).toBeUndefined();
    expect(p.spec.services.web?.networks).toBeUndefined();
  });

  it("removeSecret scrubs service and task secret refs", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertSecret",
      id: "db",
      value: { description: "database password" },
    });
    p = reduceProject(p, {
      type: "upsertService",
      id: "api",
      value: {
        ...p.spec.services.api!,
        secrets: [
          { ref: "db", target: "/run/secrets/db" },
          { ref: "other", target: "/run/secrets/other" },
        ],
      },
    });
    p = reduceProject(p, {
      type: "upsertTask",
      id: "migrate",
      value: {
        phase: "preStart",
        image: "api",
        command: ["true"],
        rerun: "always",
        secrets: [{ ref: "db", target: "/run/secrets/db" }],
      },
    });
    p = reduceProject(p, { type: "removeSecret", id: "db" });
    expect(p.spec.secrets?.db).toBeUndefined();
    expect(p.spec.services.api?.secrets).toEqual([{ ref: "other", target: "/run/secrets/other" }]);
    expect(p.spec.tasks?.migrate?.secrets).toBeUndefined();
  });

  it("removeParameter scrubs environment, ports, and build args", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertParameter",
      id: "port",
      value: { phase: "deploy", type: "integer", default: 8080 },
    });
    p = reduceProject(p, {
      type: "upsertParameter",
      id: "tag",
      value: { phase: "build", type: "string", default: "latest" },
    });
    p = reduceProject(p, {
      type: "upsertService",
      id: "web",
      value: {
        ...p.spec.services.web!,
        environment: {
          KEEP: { literal: "1" },
          HOST_PORT: { parameter: "port" },
        },
        ports: [{ container: 80, host: { parameter: "port" }, protocol: "tcp" }],
      },
    });
    p = reduceProject(p, {
      type: "upsertImage",
      id: "web",
      value: {
        build: {
          context: ".",
          dockerfile: "Dockerfile",
          args: {
            APP_TAG: { parameter: "tag" },
            KEEP: { literal: "x" },
          },
        },
      },
    });
    p = reduceProject(p, { type: "removeParameter", id: "port" });
    expect(p.spec.parameters?.port).toBeUndefined();
    expect(p.spec.services.web?.environment).toEqual({ KEEP: { literal: "1" } });
    expect(p.spec.services.web?.ports?.[0]?.host).toBeUndefined();

    p = reduceProject(p, { type: "removeParameter", id: "tag" });
    expect(p.spec.parameters?.tag).toBeUndefined();
    const img = p.spec.images.web;
    expect(img && "build" in img ? img.build.args : undefined).toEqual({ KEEP: { literal: "x" } });
  });

  it("removeImage refuses while referenced", () => {
    const p = emptyProject();
    const r = applyCommand(p, { type: "removeImage", id: "web" });
    expect(r.error).toMatch(/used by/);
    expect(r.project.spec.images.web).toBeDefined();
  });

  it("setDependencyCondition updates existing edge", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertService",
      id: "api",
      value: {
        ...p.spec.services.api!,
        healthcheck: {
          command: ["true"],
          interval: "5s",
          timeout: "1s",
          retries: 1,
        },
      },
    });
    const r = applyCommand(p, {
      type: "setDependencyCondition",
      service: "web",
      target: "api",
      condition: "healthy",
    });
    expect(r.error).toBeUndefined();
    expect(r.project.spec.services.web?.dependsOn?.[0]?.condition).toBe("healthy");
  });

  it("applyCommands batches removes into one project", () => {
    let p = emptyProject();
    p = reduceProject(p, {
      type: "upsertService",
      id: "cache",
      value: { image: "api", networks: ["frontend"] },
    });
    const r = applyCommands(p, [
      { type: "removeService", id: "cache" },
      { type: "removeDependency", service: "web", target: "api" },
    ]);
    expect(r.error).toBeUndefined();
    expect(r.project.spec.services.cache).toBeUndefined();
    expect(r.project.spec.services.web?.dependsOn).toBeUndefined();
  });
});
