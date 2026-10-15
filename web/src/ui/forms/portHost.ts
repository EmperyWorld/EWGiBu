import type { Port, ValueSource } from "../../domain/project";

export function hostIsParameter(host: Port["host"]): host is { parameter: string } {
  return Boolean(host && typeof host === "object" && "parameter" in host);
}

export function hostLiteralText(host: Port["host"]): string {
  if (host === undefined || host === null) return "";
  if (typeof host === "number") return String(host);
  if (typeof host === "object" && "literal" in host) return String(host.literal);
  return "";
}

export function parseHostLiteral(text: string): number | undefined {
  if (text.trim() === "") return undefined;
  const n = Number(text);
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : undefined;
}

export function hostFromParameter(parameterId: string): ValueSource {
  return { parameter: parameterId };
}
