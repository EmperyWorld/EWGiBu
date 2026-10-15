export type BindPlanItem = {
  resourceId?: string;
  imageId?: string;
  missingPath: string;
  kind: string;
  source?: string;
  include?: string;
  status: string;
};

export function bindItemKey(item: BindPlanItem): string {
  return item.resourceId ?? (item.imageId ? `img:${item.imageId}` : item.missingPath);
}

export type BindChoiceState = {
  mode: "stub" | "existing";
  path: string;
};

export function buildBindChoicesPayload(choices: Record<string, BindChoiceState>): {
  resourceId?: string;
  imageId?: string;
  mode: string;
  path?: string;
}[] {
  const out: { resourceId?: string; imageId?: string; mode: string; path?: string }[] = [];
  for (const [key, ch] of Object.entries(choices)) {
    if (ch.mode !== "existing" || !ch.path.trim()) continue;
    out.push({
      resourceId: key.startsWith("img:") ? undefined : key,
      imageId: key.startsWith("img:") ? key.slice(4) : undefined,
      mode: "existing",
      path: ch.path.trim(),
    });
  }
  return out;
}
