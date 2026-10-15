import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type OnSelectionChangeParams,
  type Viewport,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { applyCommands, type Command } from "../../domain/commands";
import {
  allEntityIds,
  allowedPrerequisiteIds,
  defaultCondition,
  resolveEntity,
  validateDependency,
} from "../../domain/deps";
import { suggestId } from "../../domain/ids";
import { isSemanticKind, mapProjectGraph, parseNodeId } from "../../domain/graphMap";
import type { GraphSelection } from "../../domain/selection";
import type { DependsOn, Project, Service } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";
import { fitViewDuration, prefersReducedMotion } from "../motion";
import { ServiceNode, type ServiceNodeData } from "./ServiceNode";

const nodeTypes = { iacpack: ServiceNode };

type Props = {
  project: Project;
  selection: GraphSelection;
  onSelectionChange: (sel: GraphSelection) => void;
  dispatch: (cmd: Command) => void;
  onAnnunciate: (msg: string) => void;
  serverGraphStatus: "idle" | "loading" | "current" | "invalid" | "pending";
};

function GraphCanvasInner({
  project,
  selection,
  onSelectionChange,
  dispatch,
  onAnnunciate,
  serverGraphStatus,
}: Props) {
  const { t } = useLocale();
  const { fitView } = useReactFlow();
  const [showInfo, setShowInfo] = useState(false);
  const mapped = useMemo(
    () => mapProjectGraph(project, { informational: showInfo }),
    [project, showInfo],
  );
  const entityIds = useMemo(() => allEntityIds(project), [project]);
  const [depFrom, setDepFrom] = useState("");
  const [depTo, setDepTo] = useState("");
  const [condition, setCondition] = useState<NonNullable<DependsOn["condition"]>>("started");

  // Derive selection against the current project — never setState during render
  // (import Accept / window switches were crashing on entity id churn).
  const effectiveFrom =
    !depFrom || !entityIds.includes(depFrom) ? (entityIds[0] ?? "") : depFrom;
  const prereqOptions = useMemo(
    () =>
      effectiveFrom
        ? allowedPrerequisiteIds(project, effectiveFrom)
        : entityIds.filter((id) => id !== effectiveFrom),
    [project, effectiveFrom, entityIds],
  );
  const effectiveTo =
    !depTo || !prereqOptions.includes(depTo) ? (prereqOptions[0] ?? "") : depTo;
  const prereq = effectiveTo ? resolveEntity(project, effectiveTo) : null;
  const effectiveCondition = prereq ? (defaultCondition(prereq) ?? "started") : condition;

  const graphNodes: Node<ServiceNodeData>[] = useMemo(
    () =>
      mapped.nodes.map((n) => ({
        id: n.id,
        type: "iacpack",
        position: { x: n.x, y: n.y },
        data: { label: n.label, kind: n.kind, category: n.category, semantic: n.semantic },
        draggable: n.semantic,
        connectable: n.semantic,
        deletable: n.semantic,
      })),
    [mapped.nodes],
  );

  const graphEdges: Edge[] = useMemo(() => {
    const reduceMotion = prefersReducedMotion();
    return mapped.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      label: e.label,
      className: `edge-${e.kind}`,
      animated: e.kind === "dependency" && !reduceMotion,
      deletable: e.kind === "dependency",
      selectable: e.kind === "dependency",
      focusable: e.kind === "dependency",
    }));
  }, [mapped.edges]);

  const [nodes, setNodes, onNodesChange] = useNodesState(graphNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(graphEdges);
  const suppressSelect = useRef(false);

  useEffect(() => {
    suppressSelect.current = true;
    const nodeSet = new Set(selection.nodeIds);
    const edgeSet = new Set(selection.edgeIds);
    setNodes(graphNodes.map((n) => ({ ...n, selected: nodeSet.has(n.id) })));
    setEdges(graphEdges.map((e) => ({ ...e, selected: edgeSet.has(e.id) })));
    queueMicrotask(() => {
      suppressSelect.current = false;
    });
  }, [graphNodes, graphEdges, setNodes, setEdges]); // eslint-disable-line react-hooks/exhaustive-deps -- selection applied below

  const fittedInfo = useRef(false);
  useEffect(() => {
    if (!showInfo) {
      fittedInfo.current = false;
      return;
    }
    if (fittedInfo.current) return;
    fittedInfo.current = true;
    const id = requestAnimationFrame(() => {
      void fitView({ padding: 0.2, duration: fitViewDuration() });
    });
    return () => cancelAnimationFrame(id);
  }, [showInfo, graphEdges, fitView]);

  useEffect(() => {
    suppressSelect.current = true;
    const nodeSet = new Set(selection.nodeIds);
    const edgeSet = new Set(selection.edgeIds);
    setNodes((nds) => nds.map((n) => ({ ...n, selected: nodeSet.has(n.id) })));
    setEdges((eds) => eds.map((e) => ({ ...e, selected: edgeSet.has(e.id) })));
    queueMicrotask(() => {
      suppressSelect.current = false;
    });
  }, [selection.nodeIds, selection.edgeIds, setNodes, setEdges]);

  const selectPrimary = useCallback(
    (id: string | null) => {
      onSelectionChange(id ? { nodeIds: [id], edgeIds: [] } : { nodeIds: [], edgeIds: [] });
    },
    [onSelectionChange],
  );

  const deleteSelected = useCallback(() => {
    const cmds: Command[] = [];
    const removedNames: string[] = [];
    let blockedResource = false;
    for (const id of selection.nodeIds) {
      const parsed = parseNodeId(id);
      if (!parsed) continue;
      if (!isSemanticKind(parsed.kind)) {
        blockedResource = true;
        continue;
      }
      cmds.push(
        parsed.kind === "service"
          ? { type: "removeService", id: parsed.name }
          : { type: "removeTask", id: parsed.name },
      );
      removedNames.push(parsed.name);
    }
    for (const e of edges) {
      if (!selection.edgeIds.includes(e.id) || !e.id.startsWith("dep:")) continue;
      const src = parseNodeId(e.source);
      const tgt = parseNodeId(e.target);
      if (!src || !tgt || !isSemanticKind(src.kind) || !isSemanticKind(tgt.kind)) continue;
      cmds.push({ type: "removeDependency", service: tgt.name, target: src.name });
    }
    if (cmds.length === 0) {
      onAnnunciate(blockedResource ? t("graph.delete.resource") : t("graph.delete.none"));
      return;
    }
    const result = applyCommands(project, cmds);
    if (result.error) {
      onAnnunciate(result.error);
      return;
    }
    dispatch({ type: "replaceProject", project: result.project });
    onSelectionChange({ nodeIds: [], edgeIds: [] });
    const label =
      removedNames.length > 0
        ? `${t("graph.deleted")}: ${removedNames.join(", ")}`
        : t("graph.deletedEdges");
    onAnnunciate(label);
  }, [dispatch, edges, onAnnunciate, onSelectionChange, project, selection.edgeIds, selection.nodeIds, t]);

  const isValidConnection = useCallback(
    (c: Connection | Edge) => {
      if (!c.source || !c.target) return false;
      const src = parseNodeId(c.source);
      const tgt = parseNodeId(c.target);
      if (!src || !tgt || !isSemanticKind(src.kind) || !isSemanticKind(tgt.kind)) return false;
      const prereq = resolveEntity(project, src.name);
      const cond = prereq ? defaultCondition(prereq) : "started";
      return validateDependency(project, tgt.name, src.name, cond) === null;
    },
    [project],
  );

  const onConnect = useCallback(
    (c: Connection) => {
      if (!c.source || !c.target) return;
      const src = parseNodeId(c.source);
      const tgt = parseNodeId(c.target);
      if (!src || !tgt || !isSemanticKind(src.kind) || !isSemanticKind(tgt.kind)) {
        onAnnunciate(t("graph.edge.depOnly"));
        return;
      }
      const prereq = resolveEntity(project, src.name);
      const cond = prereq ? defaultCondition(prereq) : "started";
      const err = validateDependency(project, tgt.name, src.name, cond);
      if (err) {
        onAnnunciate(err);
        return;
      }
      dispatch({
        type: "addDependency",
        service: tgt.name,
        target: src.name,
        condition: cond,
      });
    },
    [dispatch, onAnnunciate, project, t],
  );

  const onNodeDragStop = useCallback(
    (_: MouseEvent | TouchEvent, node: Node) => {
      const parsed = parseNodeId(node.id);
      if (!parsed || !isSemanticKind(parsed.kind)) return;
      dispatch({ type: "moveNode", id: parsed.name, x: node.position.x, y: node.position.y });
    },
    [dispatch],
  );

  const onRfSelectionChange = useCallback(
    (p: OnSelectionChangeParams) => {
      if (suppressSelect.current) return;
      const nodeIds = p.nodes.map((n) => n.id);
      const edgeIds = p.edges.filter((e) => e.id.startsWith("dep:")).map((e) => e.id);
      // Pane click clears RF selection; keep app selection unless Escape clears it.
      if (nodeIds.length === 0 && edgeIds.length === 0) return;
      onSelectionChange({ nodeIds, edgeIds });
    },
    [onSelectionChange],
  );

  const clearSelection = useCallback(() => {
    if (selection.nodeIds.length === 0 && selection.edgeIds.length === 0) return;
    suppressSelect.current = true;
    setNodes((nds) => nds.map((n) => ({ ...n, selected: false })));
    setEdges((eds) => eds.map((e) => ({ ...e, selected: false })));
    onSelectionChange({ nodeIds: [], edgeIds: [] });
    onAnnunciate(t("graph.selectionCleared"));
    queueMicrotask(() => {
      suppressSelect.current = false;
    });
  }, [onAnnunciate, onSelectionChange, selection.edgeIds.length, selection.nodeIds.length, setEdges, setNodes, t]);

  const addService = useCallback(() => {
    const id = suggestId("service", Object.keys(project.spec.services));
    const firstImage = Object.keys(project.spec.images)[0] ?? "web";
    const value: Service = {
      image: firstImage,
      networks: Object.keys(project.spec.networks ?? {}),
    };
    dispatch({ type: "upsertService", id, value });
    dispatch({ type: "moveNode", id, x: 80 + Object.keys(project.spec.services).length * 40, y: 120 });
    selectPrimary(`service:${id}`);
  }, [dispatch, selectPrimary, project.spec.images, project.spec.networks, project.spec.services]);

  const addTask = useCallback(() => {
    const existing = Object.keys(project.spec.tasks ?? {});
    const id = suggestId("task", existing);
    const firstImage = Object.keys(project.spec.images)[0] ?? "web";
    dispatch({
      type: "upsertTask",
      id,
      value: { phase: "preStart", image: firstImage, command: ["true"], rerun: "always" },
    });
    dispatch({ type: "moveNode", id, x: 80 + existing.length * 40, y: 280 });
    selectPrimary(`task:${id}`);
  }, [dispatch, selectPrimary, project.spec.images, project.spec.tasks]);

  const onGraphKeyDown = useCallback(
    (e: ReactKeyboardEvent<HTMLElement>) => {
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName ?? "";
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target?.isContentEditable) {
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        clearSelection();
        return;
      }
      if ((e.key === "Delete" || e.key === "Backspace") && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        deleteSelected();
        return;
      }
      if (e.key === "a" && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        addService();
        return;
      }
      if (e.key === "t" && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        addTask();
        return;
      }
      if (e.key === "f" && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        void fitView({ padding: 0.2, duration: fitViewDuration() });
        onAnnunciate(t("graph.fitView.done"));
        return;
      }
      if (e.key === "l" && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        dispatch({ type: "applyAutoLayout" });
      }
    },
    [addService, addTask, clearSelection, deleteSelected, dispatch, fitView, onAnnunciate, t],
  );

  const onMoveEnd = useCallback(
    (_: MouseEvent | TouchEvent | null, vp: Viewport) => {
      dispatch({
        type: "setViewport",
        viewport: { x: vp.x, y: vp.y, zoom: vp.zoom },
      });
    },
    [dispatch],
  );

  const connectSelected = () => {
    if (!effectiveFrom || !effectiveTo) return;
    const err = validateDependency(project, effectiveFrom, effectiveTo, effectiveCondition);
    if (err) {
      onAnnunciate(err);
      return;
    }
    dispatch({
      type: "addDependency",
      service: effectiveFrom,
      target: effectiveTo,
      condition: effectiveCondition,
    });
  };

  const entityLabel = (id: string) => {
    const ent = resolveEntity(project, id);
    if (!ent) return id;
    return ent.kind === "task" ? `${id} (task)` : id;
  };

  const prereqEnt = effectiveTo ? resolveEntity(project, effectiveTo) : null;
  const conditionOptions: Array<NonNullable<DependsOn["condition"]>> =
    prereqEnt?.kind === "task"
      ? ["completedSuccessfully", "started"]
      : prereqEnt?.kind === "service"
        ? project.spec.services[effectiveTo]?.healthcheck
          ? ["started", "healthy"]
          : ["started"]
        : ["started"];

  const defaultViewport = project.spec["x-ui"]?.viewport;
  const canDeleteSelected =
    selection.edgeIds.some((id) => id.startsWith("dep:")) ||
    selection.nodeIds.some((id) => {
      const p = parseNodeId(id);
      return Boolean(p && isSemanticKind(p.kind));
    });

  return (
    <section
      className="graph-shell"
      aria-labelledby="graph-h"
      data-testid="graph-shell"
      tabIndex={0}
      onKeyDown={onGraphKeyDown}
    >
      <div className="graph-toolbar">
        <h2 id="graph-h">{t("graph.title")}</h2>
        <span className={`graph-server graph-${serverGraphStatus}`} data-testid="server-graph-status">
          {t("graph.server")} {serverGraphStatus}
        </span>
        <button type="button" data-testid="graph-add-service" onClick={addService}>
          {t("graph.addService")}
        </button>
        <button type="button" data-testid="graph-add-task" onClick={addTask}>
          {t("graph.addTask")}
        </button>
        <button type="button" data-testid="graph-auto-layout" onClick={() => dispatch({ type: "applyAutoLayout" })}>
          {t("graph.autoLayout")}
        </button>
        <button
          type="button"
          data-testid="graph-fit-view"
          onClick={() => {
            void fitView({ padding: 0.2, duration: fitViewDuration() });
            onAnnunciate(t("graph.fitView.done"));
          }}
        >
          {t("graph.fitView")}
        </button>
        <button
          type="button"
          data-testid="graph-delete-selected"
          disabled={!canDeleteSelected}
          onClick={deleteSelected}
        >
          {t("graph.deleteSelected")}
        </button>
        <label className="graph-toggle">
          <input
            type="checkbox"
            data-testid="graph-show-info"
            checked={showInfo}
            onChange={(e) => setShowInfo(e.target.checked)}
          />{" "}
          {t("graph.showInfo")}
        </label>
      </div>
      <div className="graph-connect-row">
        <form
          className="graph-connect-form"
          data-testid="graph-connect-form"
          onSubmit={(e) => {
            e.preventDefault();
            connectSelected();
          }}
        >
          <label>
            {t("graph.dependent")}
            <select
              data-testid="graph-dep-from"
              value={effectiveFrom}
              onChange={(e) => setDepFrom(e.target.value)}
            >
              {entityIds.map((id) => (
                <option key={id} value={id}>
                  {entityLabel(id)}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t("graph.prereq")}
            <select
              data-testid="graph-dep-to"
              value={effectiveTo}
              onChange={(e) => setDepTo(e.target.value)}
            >
              {prereqOptions.map((id) => (
                <option key={id} value={id}>
                  {entityLabel(id)}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t("deps.condition")}
            <select
              data-testid="graph-dep-condition"
              value={effectiveCondition}
              onChange={(e) => setCondition(e.target.value as NonNullable<DependsOn["condition"]>)}
            >
              {conditionOptions.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
          <button
            type="submit"
            data-testid="graph-connect"
            disabled={!effectiveFrom || !effectiveTo}
            onClick={(e) => {
              e.preventDefault();
              connectSelected();
            }}
          >
            {t("graph.connect")}
          </button>
        </form>
      </div>
      <div className="graph-canvas" data-testid="graph-canvas">
        <p className="graph-overlay-hint muted">
          <span className="graph-hint">{t("graph.hint")}</span>
          <span className="graph-shortcuts" data-testid="graph-shortcuts">
            {t("graph.shortcuts")}
          </span>
        </p>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          isValidConnection={isValidConnection}
          onNodeDragStop={onNodeDragStop}
          onSelectionChange={onRfSelectionChange}
          onMoveEnd={onMoveEnd}
          nodeTypes={nodeTypes}
          fitView={!defaultViewport}
          defaultViewport={defaultViewport}
          deleteKeyCode={null}
          multiSelectionKeyCode="Shift"
          onNodesDelete={(nds) => {
            const cmds: Command[] = [];
            for (const n of nds) {
              const parsed = parseNodeId(n.id);
              if (!parsed || !isSemanticKind(parsed.kind)) continue;
              cmds.push(
                parsed.kind === "service"
                  ? { type: "removeService", id: parsed.name }
                  : { type: "removeTask", id: parsed.name },
              );
            }
            if (!cmds.length) return;
            const result = applyCommands(project, cmds);
            if (result.error) {
              onAnnunciate(result.error);
              return;
            }
            dispatch({ type: "replaceProject", project: result.project });
            onSelectionChange({ nodeIds: [], edgeIds: [] });
          }}
          onEdgesDelete={(eds) => {
            for (const e of eds) {
              if (!e.id.startsWith("dep:")) continue;
              const src = parseNodeId(e.source);
              const tgt = parseNodeId(e.target);
              if (src && tgt && isSemanticKind(src.kind) && isSemanticKind(tgt.kind)) {
                dispatch({ type: "removeDependency", service: tgt.name, target: src.name });
              }
            }
          }}
          aria-label={t("graph.aria")}
        >
          <Background />
          <Controls />
          <MiniMap pannable zoomable />
        </ReactFlow>
      </div>
    </section>
  );
}

export function GraphCanvas(props: Props) {
  return (
    <ReactFlowProvider>
      <GraphCanvasInner {...props} />
    </ReactFlowProvider>
  );
}
