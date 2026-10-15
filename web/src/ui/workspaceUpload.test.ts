import { describe, expect, it } from "vitest";
import { treeMemberPath } from "./workspaceUpload";

describe("treeMemberPath", () => {
  it("drops the picked folder name so files land in the target directory", () => {
    const file = new File(["x"], "postgresql.conf");
    Object.defineProperty(file, "webkitRelativePath", { value: "postgresql/postgresql.conf" });
    expect(treeMemberPath(file, true)).toBe("postgresql.conf");
    expect(treeMemberPath(file, false)).toBe("postgresql/postgresql.conf");
  });
});
