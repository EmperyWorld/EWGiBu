import { describe, expect, it } from "vitest";
import { applyCommand } from "./commands";
import { suggestId, validateEntityId } from "./ids";
import { emptyProject } from "./project";

describe("entity ids", () => {
  it("accepts kebab-case and rejects duplicates", () => {
    expect(validateEntityId("my-service", [])).toBeNull();
    expect(validateEntityId("Bad_Id", [])).toBe("id.kebab");
    expect(validateEntityId("web", ["web"])).toBe("id.exists");
  });

  it("suggests unique ids", () => {
    expect(suggestId("service", [])).toBe("service");
    expect(suggestId("service", ["service"])).toBe("service-2");
  });
});

describe("resource commands", () => {
  it("adds and removes resources", () => {
    let p = emptyProject();
    p = applyCommand(p, {
      type: "upsertResource",
      id: "assets",
      value: { source: "./static", include: ["**"] },
    }).project;
    expect(p.spec.resources?.assets?.source).toBe("./static");
    p = applyCommand(p, { type: "removeResource", id: "assets" }).project;
    expect(p.spec.resources).toBeUndefined();
  });
});
