import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { NodeKind } from "../../domain/graphMap";

export type ServiceNodeData = {
  label: string;
  kind: NodeKind;
  category: string;
  semantic: boolean;
};

export type IacpackNode = Node<ServiceNodeData, "iacpack">;

export function ServiceNode({ data, selected }: NodeProps<IacpackNode>) {
  const connectable = data.semantic;
  return (
    <div
      className={`flow-node kind-${data.kind} cat-${data.category}${selected ? " selected" : ""}${
        data.semantic ? "" : " info-node"
      }`}
      role="group"
      aria-label={`${data.kind} ${data.label}`}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable={connectable}
        tabIndex={connectable ? 0 : -1}
        aria-label="dependency target"
      />
      <div className="flow-node-title">{data.label}</div>
      <div className="flow-node-meta">
        {data.kind}
        {data.semantic && data.category !== "custom" ? ` · ${data.category}` : ""}
      </div>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={connectable}
        tabIndex={connectable ? 0 : -1}
        aria-label="dependency source"
      />
    </div>
  );
}
