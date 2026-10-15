import { describe, expect, it } from "vitest";
import { emptySelection, primaryNodeId, selectionEquals } from "./selection";

describe("selection", () => {
  it("compares node and edge ids", () => {
    expect(selectionEquals(emptySelection(), { nodeIds: [], edgeIds: [] })).toBe(true);
    expect(
      selectionEquals(
        { nodeIds: ["service:a", "service:b"], edgeIds: [] },
        { nodeIds: ["service:a", "service:b"], edgeIds: [] },
      ),
    ).toBe(true);
    expect(
      selectionEquals({ nodeIds: ["service:a"], edgeIds: [] }, { nodeIds: ["service:b"], edgeIds: [] }),
    ).toBe(false);
  });

  it("primaryNodeId is first selected node", () => {
    expect(primaryNodeId(emptySelection())).toBeNull();
    expect(primaryNodeId({ nodeIds: ["service:a", "task:t"], edgeIds: ["dep:x"] })).toBe("service:a");
  });
});
