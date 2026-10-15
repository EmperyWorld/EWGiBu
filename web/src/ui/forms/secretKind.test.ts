import { describe, expect, it } from "vitest";
import { emptyProject } from "../../domain/project";
import { classifySecret, secretHints } from "./secretKind";

describe("classifySecret", () => {
  it("prefers port, then password, then name", () => {
    expect(classifySecret(["db-port"])).toEqual({ kind: "port", confident: true });
    expect(classifySecret(["db-user-password"])).toEqual({ kind: "password", confident: true });
    expect(classifySecret(["db-user"])).toEqual({ kind: "name", confident: true });
    expect(classifySecret(["api-key"])).toEqual({ kind: "password", confident: true });
    expect(classifySecret(["keycloak-admin"])).toEqual({ kind: "name", confident: true });
    expect(classifySecret(["app"])).toEqual({ kind: "password", confident: false });
  });

  it("reads the environment key that references the secret", () => {
    const project = emptyProject();
    project.spec.secrets = { "db-password": { required: true, delivery: "file" } };
    project.spec.services.app = {
      image: "api",
      environment: { POSTGRES_PASSWORD: { secret: "db-password" } },
      secrets: [{ ref: "db-password", target: "/run/secrets/db-password" }],
    };
    expect(classifySecret(secretHints(project, "db-password")).kind).toBe("password");
  });
});
