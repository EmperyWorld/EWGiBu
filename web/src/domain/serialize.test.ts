import { describe, expect, it } from "vitest";
import { emptyProject } from "./project";
import { parseProjectYaml, projectToYaml } from "./serialize";

describe("serialize", () => {
  it("round-trips empty two-service project", () => {
    const p = emptyProject();
    p.spec.services.web = { ...p.spec.services.web!, customExtra: "keep" };
    const yaml = projectToYaml(p);
    const parsed = parseProjectYaml(yaml);
    expect(parsed.ok).toBe(true);
    if (!parsed.ok) return;
    expect(parsed.project.spec.services.web?.customExtra).toBe("keep");
    expect(Object.keys(parsed.project.spec.services)).toEqual(["web", "api"]);
  });

  it("quotes command text that contains brackets", () => {
    const p = emptyProject();
    p.spec.services.web = {
      ...p.spec.services.web!,
      healthcheck: {
        command: ["/bin/sh", "-c", "pgrep -f '[c]elery.*beat' >/dev/null || exit 1"],
      },
    };
    const yaml = projectToYaml(p);
    expect(yaml).toContain('"pgrep -f \'[c]elery.*beat\' >/dev/null || exit 1"');
    expect(yaml).not.toMatch(/^\s+- pgrep /m);
  });

  it("rejects bad kind", () => {
    const r = parseProjectYaml("apiVersion: x\nkind: Nope\n");
    expect(r.ok).toBe(false);
  });
});
