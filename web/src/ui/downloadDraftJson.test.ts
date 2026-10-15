import { describe, expect, it } from "vitest";
import { draftJsonDownloadFilename } from "./downloadDraftJson";

describe("draftJsonDownloadFilename", () => {
  it("sanitizes project name", () => {
    expect(draftJsonDownloadFilename("stack4-stubs")).toBe("stack4-stubs.json");
    expect(draftJsonDownloadFilename("  WEIRD!! name  ")).toBe("weird-name.json");
  });
});
