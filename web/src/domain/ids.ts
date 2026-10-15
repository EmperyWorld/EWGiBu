import type { MessageKey } from "../i18n/messages";

const KEBAB = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;

export function validateEntityId(
  id: string,
  existing: Iterable<string>,
): MessageKey | null {
  const trimmed = id.trim();
  if (!trimmed) return "id.required";
  if (!KEBAB.test(trimmed)) return "id.kebab";
  if (trimmed.length > 63) return "id.tooLong";
  for (const e of existing) {
    if (e === trimmed) return "id.exists";
  }
  return null;
}

export function suggestId(prefix: string, existing: Iterable<string>): string {
  const set = new Set(existing);
  if (!set.has(prefix)) return prefix;
  for (let i = 2; i < 1000; i++) {
    const id = `${prefix}-${String(i)}`;
    if (!set.has(id)) return id;
  }
  return `${prefix}-${String(Date.now())}`;
}
