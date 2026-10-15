import { describe, expect, it } from "vitest";
import { bytesToMib, mibToBytes, MIB } from "./memory";

describe("memory helpers", () => {
  it("converts MiB ↔ bytes", () => {
    expect(mibToBytes("256")).toBe(256 * MIB);
    expect(bytesToMib(268435456)).toBe("256");
    expect(mibToBytes("")).toBeUndefined();
  });
});
