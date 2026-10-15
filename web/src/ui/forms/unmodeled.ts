/** Known schema-modeled keys per authoring object; extras stay as unmodeled JSON. */

export const KNOWN_SERVICE = new Set([
  "image",
  "user",
  "command",
  "entrypoint",
  "environment",
  "environmentFrom",
  "secrets",
  "ports",
  "mounts",
  "networks",
  "profiles",
  "dependsOn",
  "healthcheck",
  "restart",
  "resources",
]);

export const KNOWN_IMAGE_PULL = new Set(["kind", "ref", "delivery"]);
export const KNOWN_IMAGE_BUILD = new Set(["kind", "build"]);

export const KNOWN_RESOURCE = new Set([
  "source",
  "include",
  "exclude",
  "destination",
  "mode",
  "allowEmpty",
]);

export const KNOWN_TASK = new Set([
  "phase",
  "image",
  "user",
  "command",
  "entrypoint",
  "workdir",
  "dependsOn",
  "timeout",
  "retry",
  "rerun",
  "mounts",
  "networks",
  "secrets",
]);

export function pickUnmodeled(
  obj: Record<string, unknown>,
  known: Set<string>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (!known.has(k)) out[k] = v;
  }
  return out;
}

export function stripUnmodeled<T extends Record<string, unknown>>(
  obj: T,
  known: Set<string>,
): T {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (known.has(k)) out[k] = v;
  }
  return out as T;
}

export function mergeUnmodeled<T extends Record<string, unknown>>(
  obj: T,
  known: Set<string>,
  extra: Record<string, unknown>,
): T {
  return { ...stripUnmodeled(obj, known), ...extra } as T;
}
