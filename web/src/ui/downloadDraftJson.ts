/** Browser download of draft Project JSON (no server write). */

export function draftJsonDownloadFilename(name?: string): string {
  const stem = (name?.trim() || "import-draft")
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return (stem || "import-draft") + ".json";
}

export function downloadProjectJson(project: unknown, filename: string): void {
  const text = `${JSON.stringify(project, null, 2)}\n`;
  const blob = new Blob([text], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.rel = "noopener";
  anchor.click();
  URL.revokeObjectURL(url);
}
