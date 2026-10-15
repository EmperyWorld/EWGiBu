import type { StubPathGroups } from "./ImportStubPathLists";

export function stubPathCreateCount(groups: StubPathGroups | null | undefined): number {
  if (!groups) return 0;
  return groups.dirs.length + groups.files.length + groups.buildContexts.length;
}

export function stubPathHasAny(groups: StubPathGroups | null | undefined): boolean {
  if (!groups) return false;
  return stubPathCreateCount(groups) > 0 || (groups.skipped?.length ?? 0) > 0;
}
