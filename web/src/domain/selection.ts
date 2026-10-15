/** Canvas selection: React Flow node ids (`service:x`) and dependency edge ids (`dep:…`). */
export type GraphSelection = {
  nodeIds: string[];
  edgeIds: string[];
};

export const emptySelection = (): GraphSelection => ({ nodeIds: [], edgeIds: [] });

export function selectionEquals(a: GraphSelection, b: GraphSelection): boolean {
  if (a.nodeIds.length !== b.nodeIds.length || a.edgeIds.length !== b.edgeIds.length) return false;
  return (
    a.nodeIds.every((id, i) => id === b.nodeIds[i]) &&
    a.edgeIds.every((id, i) => id === b.edgeIds[i])
  );
}

export function primaryNodeId(sel: GraphSelection): string | null {
  return sel.nodeIds[0] ?? null;
}
