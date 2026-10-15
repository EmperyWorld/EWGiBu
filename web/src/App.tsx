import { useCallback, useEffect, useRef, useState } from "react";
import { useWorkspace } from "./app/useWorkspace";
import { emptySelection, selectionEquals, type GraphSelection } from "./domain/selection";
import { DesktopShell } from "./desktop/DesktopShell";
import type { WindowId } from "./desktop/WindowFrame";
import { useLocale } from "./i18n/useLocale";
import { diagnosticField } from "./ui/diagnosticField";
import { DiagnosticsList } from "./ui/DiagnosticsList";
import { fieldNavFromPointer, focusFieldPath } from "./ui/fieldPath";
import { ProjectForms } from "./ui/forms/ProjectForms";
import { DependencyTable } from "./ui/graph/DependencyTable";
import { GraphCanvas } from "./ui/graph/GraphCanvas";
import { PropertyPanel } from "./ui/graph/PropertyPanel";
import { PreviewPanel } from "./ui/PreviewPanel";
import { buildStatusText } from "./ui/statusText";
import { SyncBanner } from "./ui/SyncBanner";
import { Toolbar } from "./ui/Toolbar";
import { ImportDraftPanel } from "./ui/ImportDraftPanel";
import { ReleaseTemplatesPanel } from "./ui/ReleaseTemplatesPanel";
import { RuntimeGuidePanel } from "./ui/RuntimeGuidePanel";
import { SaveAsDialog } from "./ui/SaveAsDialog";
import { consumeImportNextSteps, peekImportNextSteps } from "./app/importNextSteps";
import { AuthorWelcome } from "./ui/AuthorWelcome";
import { ImportNextStepsBanner } from "./ui/ImportNextStepsBanner";
import type { Project } from "./domain/project";

export function App() {
  const ws = useWorkspace();
  const { state, undo, redo, canUndo, canRedo } = ws;
  const { t } = useLocale();
  const [selection, setSelection] = useState<GraphSelection>(emptySelection);
  const [raise, setRaise] = useState<{ id: WindowId; token: number } | null>(null);
  const [seenAttention, setSeenAttention] = useState(0);
  const [importOpen, setImportOpen] = useState(false);
  const [releaseOpen, setReleaseOpen] = useState(false);
  const [runtimeOpen, setRuntimeOpen] = useState(false);
  const [runtimeEmphasis, setRuntimeEmphasis] = useState(false);
  const [saveAsOpen, setSaveAsOpen] = useState(false);
  const [importNextStepsOpen, setImportNextStepsOpen] = useState(() => peekImportNextSteps());
  const prevSaveStatus = useRef(state.save.status);
  const focusedAttention = useRef(0);
  if (state.diagnosticsAttention > seenAttention) {
    setSeenAttention(state.diagnosticsAttention);
    if (state.diagnostics.length > 0) {
      setRaise({ id: "diagnostics", token: state.diagnosticsAttention });
    }
  }

  const serverGraphStatus =
    state.serverGraph.status === "idle"
      ? "idle"
      : state.serverGraph.status === "loading"
        ? "loading"
        : state.serverGraph.status === "pending"
          ? "pending"
          : state.serverGraph.status === "invalid"
            ? "invalid"
            : "current";

  const applySelection = useCallback((sel: GraphSelection) => {
    setSelection((prev) => {
      if (selectionEquals(prev, sel)) return prev;
      if (sel.nodeIds.length > 0) {
        queueMicrotask(() => setRaise({ id: "inspector", token: Date.now() }));
      }
      return sel;
    });
  }, []);

  const selectNode = useCallback(
    (id: string | null) => {
      applySelection(id ? { nodeIds: [id], edgeIds: [] } : emptySelection());
    },
    [applySelection],
  );

  const navigateField = useCallback(
    (field: string) => {
      const nav = fieldNavFromPointer(field);
      if (!nav) return;
      setRaise({ id: nav.window, token: Date.now() });
      if (nav.selectNodeId) {
        applySelection({ nodeIds: [nav.selectNodeId], edgeIds: [] });
        queueMicrotask(() => setRaise({ id: "inspector", token: Date.now() + 1 }));
      }
      window.setTimeout(() => {
        focusFieldPath(nav.selectors);
      }, 80);
    },
    [applySelection],
  );

  useEffect(() => {
    if (state.diagnosticsAttention <= focusedAttention.current) return;
    focusedAttention.current = state.diagnosticsAttention;
    if (state.diagnostics.length === 0) return;
    const firstField = state.diagnostics.map(diagnosticField).find((f) => f.length > 0);
    if (firstField) {
      window.setTimeout(() => navigateField(firstField), 120);
    }
  }, [state.diagnosticsAttention, state.diagnostics, navigateField]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      const key = e.key.toLowerCase();
      if (key === "z" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        undo();
        return;
      }
      if ((key === "z" && e.shiftKey) || key === "y") {
        e.preventDefault();
        redo();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [undo, redo]);

  useEffect(() => {
    const prev = prevSaveStatus.current;
    prevSaveStatus.current = state.save.status;
    if (prev === "saving" && state.save.status === "saved") {
      setRuntimeEmphasis(true);
      setRuntimeOpen(true);
    }
  }, [state.save.status]);

  const openRuntime = () => {
    setRuntimeEmphasis(false);
    setRuntimeOpen(true);
  };

  const toolbar = (
    <Toolbar
      state={state}
      saveBlocked={ws.saveBlocked}
      canUndo={canUndo}
      canRedo={canRedo}
      onReload={() => {
        void ws.load();
      }}
      onSave={() => {
        void ws.save();
      }}
      onSaveAs={() => setSaveAsOpen(true)}
      onUndo={() => {
        undo();
      }}
      onRedo={() => {
        redo();
      }}
      onValidate={() => {
        void ws.validate();
      }}
      onPreview={() => {
        void ws.refreshPreview();
        setRaise({ id: "preview", token: Date.now() });
      }}
      onExport={() => {
        void ws.exportCanonical();
      }}
      onBuild={() => {
        void ws.startBuild();
      }}
      onCancelBuild={() => {
        void ws.cancelBuild();
      }}
      onImport={() => setImportOpen(true)}
      onReleaseTemplates={() => setReleaseOpen(true)}
      onRuntimeGuide={openRuntime}
      onProjectSwitched={() => {
        setSelection(emptySelection());
        void ws.load();
      }}
    />
  );

  const syncBanner = (
    <SyncBanner
      state={state}
      onReload={() => {
        void ws.load();
      }}
      onDiscardLocal={() => {
        void ws.load();
      }}
      onExportLocal={() => {
        ws.exportLocalYaml();
      }}
      onArmOverwrite={() => {
        ws.armOverwrite();
      }}
      onRecoverOverwrite={() => {
        void ws.recoverOverwrite();
      }}
      onImport={() => setImportOpen(true)}
    />
  );

  const designerPane = (
    <div className="pane-designer">
      {state.phase === "loading" && <p role="status">{t("loading")}</p>}
      {state.phase === "disconnected" && (
        <p role="alert" className="error">
          {t("disconnected")}
        </p>
      )}
      {state.project ? (
        <>
          <GraphCanvas
            project={state.project}
            selection={selection}
            onSelectionChange={applySelection}
            dispatch={ws.dispatch}
            onAnnunciate={ws.announce}
            serverGraphStatus={serverGraphStatus}
          />
          <DependencyTable
            project={state.project}
            onRemove={(dependent, target) =>
              ws.dispatch({ type: "removeDependency", service: dependent, target })
            }
            onConditionChange={(dependent, target, condition) =>
              ws.dispatch({ type: "setDependencyCondition", service: dependent, target, condition })
            }
            onSelect={(nodeId) => selectNode(nodeId)}
          />
        </>
      ) : null}
      {!state.project && state.phase !== "loading" && (
        <AuthorWelcome
          onImport={() => setImportOpen(true)}
          onRuntimeGuide={openRuntime}
        />
      )}
      {!state.project && state.phase === "loading" && (
        <p className="muted">{t("no.project")}</p>
      )}
    </div>
  );

  const formsPane = (
    <div className="pane-forms">
      {state.phase === "loading" && <p role="status">{t("loading")}</p>}
      {state.project ? (
        <ProjectForms project={state.project} dispatch={ws.dispatch} />
      ) : (
        <p className="muted">{t("no.project")}</p>
      )}
    </div>
  );

  return (
    <>
      <ImportNextStepsBanner
        open={importNextStepsOpen && Boolean(state.project)}
        onSave={() => {
          void ws.save();
        }}
        onSaveAs={() => setSaveAsOpen(true)}
        onValidate={() => {
          void ws.validate();
        }}
        onRuntimeGuide={openRuntime}
        onDismiss={() => {
          consumeImportNextSteps();
          setImportNextStepsOpen(false);
        }}
      />
      <DesktopShell
        toolbar={toolbar}
        syncBanner={syncBanner}
        statusText={buildStatusText(state, t)}
        raise={raise}
        panes={{
          designer: designerPane,
          forms: formsPane,
          preview: <PreviewPanel preview={state.preview} />,
          diagnostics: (
            <DiagnosticsList
              diagnostics={
                state.serverGraph.status === "invalid"
                  ? state.serverGraph.diagnostics
                  : state.diagnostics
              }
              onFieldNavigate={navigateField}
            />
          ),
          inspector: state.project ? (
            <PropertyPanel
              project={state.project}
              selection={selection}
              announce={state.graphAnnounce}
              dispatch={ws.dispatch}
            />
          ) : (
            <p className="muted">{t("no.project")}</p>
          ),
        }}
      />
      <ImportDraftPanel
        open={importOpen}
        onClose={() => setImportOpen(false)}
        onAccept={(project: Project) => {
          ws.dispatch({ type: "replaceProject", project });
          setSelection(emptySelection());
          ws.announce(t("import.accept"));
          setImportNextStepsOpen(true);
          setRaise({ id: "designer", token: Date.now() });
        }}
      />
      <ReleaseTemplatesPanel open={releaseOpen} onClose={() => setReleaseOpen(false)} />
      <RuntimeGuidePanel
        open={runtimeOpen}
        projectName={state.project?.metadata.name}
        emphasizeSave={runtimeEmphasis}
        onClose={() => {
          setRuntimeOpen(false);
          setRuntimeEmphasis(false);
        }}
      />
      <SaveAsDialog
        open={saveAsOpen}
        busy={state.save.status === "saving"}
        onClose={() => setSaveAsOpen(false)}
        onSubmit={(path) => {
          void ws.saveAs(path);
          setSaveAsOpen(false);
        }}
      />
    </>
  );
}
