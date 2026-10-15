import { describe, expect, it } from "vitest";
import { argvText } from "./project";

describe("argvText", () => {
  it("renders parameters, secrets, empty literals, and YAML booleans", () => {
    expect(
      argvText([
        "--requirepass",
        true,
        { secret: "redis-password" },
        { literal: "" },
        { parts: ["/models/", { parameter: "llama-chat-file" }] },
        false,
      ]),
    ).toBe("--requirepass true ${secret:redis-password}  /models/${llama-chat-file} false");
  });
});
