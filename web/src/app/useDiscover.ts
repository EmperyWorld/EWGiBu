import { useEffect, useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";

export type DiscoverSuggestion = {
  path: string;
  kind: string;
};

type DiscoverResult = {
  suggestions?: Array<{ path?: unknown; kind?: unknown }>;
};

export function useDiscoverSuggestions(): {
  suggestions: DiscoverSuggestion[];
  loading: boolean;
  reload: () => void;
} {
  const [suggestions, setSuggestions] = useState<DiscoverSuggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
    void client
      .discoverWorkspace()
      .then(async (res) => {
        if (!res.ok || cancelled) return;
        const body = (await res.json()) as { result?: DiscoverResult };
        const items = body.result?.suggestions ?? [];
        const parsed: DiscoverSuggestion[] = [];
        for (const item of items) {
          if (typeof item.path === "string" && typeof item.kind === "string") {
            parsed.push({ path: item.path, kind: item.kind });
          }
        }
        if (!cancelled) setSuggestions(parsed);
      })
      .catch(() => {
        if (!cancelled) setSuggestions([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  return {
    suggestions,
    loading,
    reload: () => {
      setLoading(true);
      setTick((n) => n + 1);
    },
  };
}

export function dockerfilesFrom(suggestions: DiscoverSuggestion[]): DiscoverSuggestion[] {
  return suggestions.filter((s) => s.kind === "dockerfile");
}

/** Split workspace-relative Dockerfile path into context dir + dockerfile name. */
export function splitDockerfilePath(full: string): { context: string; dockerfile: string } {
  const normalized = full.replace(/\\/g, "/").replace(/^\.\//, "");
  const slash = normalized.lastIndexOf("/");
  if (slash <= 0) return { context: ".", dockerfile: normalized || "Dockerfile" };
  return {
    context: normalized.slice(0, slash) || ".",
    dockerfile: normalized.slice(slash + 1) || "Dockerfile",
  };
}
