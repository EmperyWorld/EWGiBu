import { describe, expect, it } from "vitest";
import { dockerfilesFrom, splitDockerfilePath } from "../app/useDiscover";

describe("discover helpers", () => {
  it("splits dockerfile paths into context and name", () => {
    expect(splitDockerfilePath("app/Dockerfile")).toEqual({ context: "app", dockerfile: "Dockerfile" });
    expect(splitDockerfilePath("Dockerfile")).toEqual({ context: ".", dockerfile: "Dockerfile" });
    expect(splitDockerfilePath("./web/Dockerfile.dev")).toEqual({
      context: "web",
      dockerfile: "Dockerfile.dev",
    });
  });

  it("filters dockerfile suggestions", () => {
    expect(
      dockerfilesFrom([
        { path: "app/Dockerfile", kind: "dockerfile" },
        { path: "compose.yaml", kind: "compose" },
      ]),
    ).toEqual([{ path: "app/Dockerfile", kind: "dockerfile" }]);
  });
});
