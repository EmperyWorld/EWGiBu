import { describe, expect, it } from "vitest";
import { diagnosticField } from "./diagnosticField";

describe("diagnosticField", () => {
  it("prefers field then path then source.pointer", () => {
    expect(diagnosticField({ field: "/a" })).toBe("/a");
    expect(diagnosticField({ path: "/b" })).toBe("/b");
    expect(diagnosticField({ source: { pointer: "/metadata/name" } })).toBe("/metadata/name");
    expect(diagnosticField({ message: "x" })).toBe("");
  });
});
