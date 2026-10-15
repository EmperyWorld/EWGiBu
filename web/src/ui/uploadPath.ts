/** Helpers for import file browse / drag-drop uploads into the workspace. */

export function basenameUpload(name: string): string {
  const n = name.replace(/\\/g, "/").split("/").pop() ?? name;
  return n.replace(/^\.+/, "") || "upload.txt";
}

export function safeUploadRelPath(name: string): string {
  const base = basenameUpload(name).replace(/[^\w.\-+=@]+/g, "_");
  if (!base || base === "." || base === "..") return "upload.txt";
  const lower = base.toLowerCase();
  if (lower === ".env" || lower.startsWith(".env.") || lower.endsWith(".env")) {
    return "upload.env.example";
  }
  return base;
}

export function classifyUploadName(name: string): "compose" | "dockerfile" {
  const base = basenameUpload(name);
  const lower = base.toLowerCase();
  if (lower === "dockerfile" || lower.startsWith("dockerfile.") || lower.endsWith(".dockerfile")) {
    return "dockerfile";
  }
  return "compose";
}
