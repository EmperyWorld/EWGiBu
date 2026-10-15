import type { Project, ValueSource } from "../../domain/project";

export type SecretKind = "password" | "port" | "name";

const PASSWORD = new Set([
  "password",
  "passwd",
  "pass",
  "secret",
  "token",
  "jwt",
  "credential",
  "credentials",
  "apikey",
  "key",
  "private",
]);
const PORT = new Set(["port", "ports"]);
const NAME = new Set(["user", "username", "name", "login", "dbuser", "dbname", "admin"]);

export function classifySecret(hints: string[]): { kind: SecretKind; confident: boolean } {
  const tokens = hints.flatMap(hintTokens);
  if (tokens.some((token) => PORT.has(token))) return { kind: "port", confident: true };
  if (tokens.some((token) => PASSWORD.has(token))) return { kind: "password", confident: true };
  if (tokens.some((token) => NAME.has(token))) return { kind: "name", confident: true };
  return { kind: "password", confident: false };
}

export function secretHints(project: Project, id: string): string[] {
  const hints = [id];
  const description = project.spec.secrets?.[id]?.description;
  if (description) hints.push(description);
  const holders = [
    ...Object.values(project.spec.services ?? {}),
    ...Object.values(project.spec.tasks ?? {}),
  ];
  for (const holder of holders) {
    const env = holder.environment;
    if (env) {
      for (const [key, source] of Object.entries(env)) {
        if (valueUsesSecret(source, id)) hints.push(key);
      }
    }
    for (const mount of holder.secrets ?? []) {
      if (mount.ref === id) hints.push(mount.target);
    }
  }
  return hints;
}

function hintTokens(value: string): string[] {
  return value
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter((token) => token.length > 0);
}

function valueUsesSecret(source: ValueSource | unknown, id: string): boolean {
  if (!source || typeof source !== "object") return false;
  const record = source as { secret?: unknown; parts?: unknown };
  if (record.secret === id) return true;
  if (!Array.isArray(record.parts)) return false;
  return record.parts.some((part) => valueUsesSecret(part, id));
}
