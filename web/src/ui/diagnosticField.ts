import type { Diagnostic } from "../app/types";

/** Prefer explicit field/path; fall back to API `source.pointer` (CLI/API envelope). */
export function diagnosticField(d: Diagnostic): string {
  if (typeof d.field === "string" && d.field.trim()) return d.field.trim();
  if (typeof d.path === "string" && d.path.trim()) return d.path.trim();
  const src = d.source;
  if (src && typeof src === "object" && !Array.isArray(src)) {
    const pointer = (src as { pointer?: unknown }).pointer;
    if (typeof pointer === "string" && pointer.trim()) return pointer.trim();
  }
  return "";
}
