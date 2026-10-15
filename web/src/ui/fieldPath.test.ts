import { describe, expect, it } from "vitest";
import { fieldNavFromPointer } from "../ui/fieldPath";
import { mergeUnmodeled, pickUnmodeled, KNOWN_SERVICE } from "../ui/forms/unmodeled";

describe("unmodeled helpers", () => {
  it("picks and merges extras without touching known keys", () => {
    const obj = { image: "web", ports: [], customExtra: 1, other: true };
    expect(pickUnmodeled(obj, KNOWN_SERVICE)).toEqual({ customExtra: 1, other: true });
    expect(mergeUnmodeled(obj, KNOWN_SERVICE, { customExtra: 2 })).toEqual({
      image: "web",
      ports: [],
      customExtra: 2,
    });
  });
});

describe("fieldNavFromPointer", () => {
  it("maps service port paths to forms selectors", () => {
    const nav = fieldNavFromPointer("/spec/services/web/ports/0");
    expect(nav?.window).toBe("forms");
    expect(nav?.selectNodeId).toBe("service:web");
    expect(nav?.selectors[0]).toContain("/spec/services/web/ports");
  });

  it("maps metadata name", () => {
    const nav = fieldNavFromPointer("/metadata/name");
    expect(nav?.selectors[0]).toBe('[data-field-path="/metadata/name"]');
  });

  it("maps network and volume pointers to canvas node ids", () => {
    expect(fieldNavFromPointer("/spec/networks/frontend")?.selectNodeId).toBe("network:frontend");
    expect(fieldNavFromPointer("/spec/volumes/data")?.selectNodeId).toBe("volume:data");
  });
});
