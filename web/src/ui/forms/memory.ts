/** Human-friendly memory helpers (schema stores integer bytes). */

export const MIB = 1024 * 1024;

export function bytesToMib(bytes: number | undefined): string {
  if (bytes === undefined || !Number.isFinite(bytes)) return "";
  const mib = bytes / MIB;
  return Number.isInteger(mib) ? String(mib) : mib.toFixed(3).replace(/\.?0+$/, "");
}

export function mibToBytes(text: string): number | undefined {
  const trimmed = text.trim();
  if (!trimmed) return undefined;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n <= 0) return undefined;
  return Math.round(n * MIB);
}
