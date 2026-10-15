export function readCsrf(): string {
  const meta = document.querySelector('meta[name="iacpack-csrf"]');
  const content = meta?.getAttribute("content") ?? "";
  if (!content || content === "__IACPACK_CSRF__") {
    throw new Error("CSRF token missing; reload from iacpack serve");
  }
  return content;
}
