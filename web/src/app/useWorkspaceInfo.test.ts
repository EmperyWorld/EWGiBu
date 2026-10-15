import { describe, expect, it } from "vitest";
import { buildRuntimeCliScript } from "./useWorkspaceInfo";

describe("buildRuntimeCliScript", () => {
  it("writes a bundle file and the project name for status/destroy", () => {
    const script = buildRuntimeCliScript(
      { workspace: "/tmp/ws", manifest: "iacpack.yaml", outputRoot: ".iacpack-out" },
      "shop",
    );
    expect(script).toContain("--output '.iacpack-out/bundle.iacbundle'");
    expect(script).toContain("iacpack verify '.iacpack-out/bundle.iacbundle'");
    expect(script).toContain("iacpack status 'shop' --instance default");
    expect(script).toContain("--secrets-dir ./secrets");
    expect(script).toContain("--values values.json");
    expect(script).not.toContain("<project>");
  });
});
