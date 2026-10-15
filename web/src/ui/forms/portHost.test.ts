import { describe, expect, it } from "vitest";
import {
  hostFromParameter,
  hostIsParameter,
  hostLiteralText,
  parseHostLiteral,
} from "./portHost";

describe("portHost", () => {
  it("round-trips literal and parameter hosts", () => {
    expect(hostLiteralText(8080)).toBe("8080");
    expect(hostLiteralText({ literal: 9090 })).toBe("9090");
    expect(hostIsParameter({ parameter: "http-port" })).toBe(true);
    expect(hostIsParameter(8080)).toBe(false);
    expect(parseHostLiteral("443")).toBe(443);
    expect(parseHostLiteral("")).toBeUndefined();
    expect(hostFromParameter("http-port")).toEqual({ parameter: "http-port" });
  });
});
