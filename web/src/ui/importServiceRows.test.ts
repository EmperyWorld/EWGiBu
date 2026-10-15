import { describe, expect, it } from "vitest";
import { importServiceRows } from "./importServiceRows";

describe("importServiceRows", () => {
  it("lists numeric user and environmentFrom in id order", () => {
    const rows = importServiceRows({
      beat: { user: "0:0", environmentFrom: "api" },
      api: { user: "1000" },
      web: {},
    });
    expect(rows).toEqual([
      { id: "api", user: "1000", environmentFrom: "" },
      { id: "beat", user: "0:0", environmentFrom: "api" },
    ]);
  });
});
