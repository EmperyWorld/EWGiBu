import { describe, expect, it } from "vitest";
import { emptyProject } from "../domain/project";
import {
  emptyHistory,
  pushHistory,
  redoHistory,
  skipsHistory,
  undoHistory,
} from "./editHistory";

describe("editHistory", () => {
  it("skips viewport commands", () => {
    expect(skipsHistory({ type: "setViewport", viewport: { x: 0, y: 0, zoom: 1 } })).toBe(true);
    expect(skipsHistory({ type: "removeService", id: "web" })).toBe(false);
  });

  it("undo restores prior entry and redo restores current", () => {
    const a = { project: emptyProject(), dirty: false };
    const b = { project: emptyProject(), dirty: true };
    b.project.metadata.name = "b";
    const c = { project: emptyProject(), dirty: true };
    c.project.metadata.name = "c";

    let h = emptyHistory();
    h = pushHistory(h, a);
    h = pushHistory(h, b);

    const u1 = undoHistory(h, c);
    expect(u1).not.toBeNull();
    expect(u1!.entry.project.metadata.name).toBe("b");
    expect(u1!.history.past).toHaveLength(1);
    expect(u1!.history.future).toHaveLength(1);

    const r1 = redoHistory(u1!.history, u1!.entry);
    expect(r1).not.toBeNull();
    expect(r1!.entry.project.metadata.name).toBe("c");
    expect(r1!.history.future).toHaveLength(0);
  });

  it("new push clears future", () => {
    const a = { project: emptyProject(), dirty: false };
    const b = { project: emptyProject(), dirty: true };
    b.project.metadata.name = "b";
    let h = pushHistory(emptyHistory(), a);
    const u = undoHistory(h, b)!;
    expect(u.history.future).toHaveLength(1);
    h = pushHistory(u.history, u.entry);
    expect(h.future).toHaveLength(0);
  });
});
