import type { AppState } from "../app/types";
import { useLocale } from "../i18n/useLocale";
import { ProjectSwitcher } from "./ProjectSwitcher";

type Props = {
  state: AppState;
  saveBlocked: boolean;
  canUndo: boolean;
  canRedo: boolean;
  onReload: () => void;
  onSave: () => void;
  onSaveAs: () => void;
  onUndo: () => void;
  onRedo: () => void;
  onValidate: () => void;
  onPreview: () => void;
  onExport: () => void;
  onBuild: () => void;
  onCancelBuild: () => void;
  onImport: () => void;
  onReleaseTemplates: () => void;
  onRuntimeGuide: () => void;
  onProjectSwitched: () => void;
};

export function Toolbar({
  state,
  saveBlocked,
  canUndo,
  canRedo,
  onReload,
  onSave,
  onSaveAs,
  onUndo,
  onRedo,
  onValidate,
  onPreview,
  onExport,
  onBuild,
  onCancelBuild,
  onImport,
  onReleaseTemplates,
  onRuntimeGuide,
  onProjectSwitched,
}: Props) {
  const { t } = useLocale();
  const busy = state.save.status === "saving" || state.validating || state.build.status === "running";
  const saveDisabled = busy || !state.project || saveBlocked;
  return (
    <div className="toolbar" role="toolbar" aria-label={t("toolbar.aria")}>
      <ProjectSwitcher disabled={busy} onSwitched={onProjectSwitched} />
      <button type="button" onClick={onReload} disabled={busy}>
        {t("toolbar.reload")}
      </button>
      <button
        type="button"
        onClick={onImport}
        disabled={busy}
        data-testid="import-open"
        title={t("toolbar.importHint")}
      >
        {t("toolbar.import")}
      </button>
      <button
        type="button"
        onClick={onReleaseTemplates}
        disabled={busy}
        data-testid="release-templates-open"
        title={t("toolbar.releaseHint")}
      >
        {t("toolbar.release")}
      </button>
      <button
        type="button"
        onClick={onRuntimeGuide}
        disabled={busy}
        data-testid="runtime-open"
        title={t("toolbar.runtimeHint")}
      >
        {t("toolbar.runtime")}
      </button>
      <button
        type="button"
        onClick={onSave}
        disabled={saveDisabled}
        data-testid="save"
        title={saveBlocked ? t("toolbar.saveBlocked") : t("toolbar.saveHint")}
      >
        {t("toolbar.save")}
      </button>
      <button type="button" onClick={onSaveAs} disabled={busy || !state.project} data-testid="save-as-open">
        {t("toolbar.saveAs")}
      </button>
      <button
        type="button"
        onClick={onUndo}
        disabled={busy || !canUndo}
        data-testid="undo"
        title={t("toolbar.undoHint")}
      >
        {t("toolbar.undo")}
      </button>
      <button
        type="button"
        onClick={onRedo}
        disabled={busy || !canRedo}
        data-testid="redo"
        title={t("toolbar.redoHint")}
      >
        {t("toolbar.redo")}
      </button>
      <button
        type="button"
        onClick={onValidate}
        disabled={busy || state.dirty}
        title={state.dirty ? t("toolbar.validateDirty") : t("toolbar.validateOk")}
        data-testid="validate"
      >
        {t("toolbar.validate")}
      </button>
      <button
        type="button"
        onClick={onPreview}
        disabled={busy || state.dirty}
        title={state.dirty ? t("toolbar.previewDirty") : t("toolbar.previewOk")}
        data-testid="preview"
      >
        {t("toolbar.preview")}
      </button>
      <button type="button" onClick={onExport} disabled={busy || state.dirty}>
        {t("toolbar.export")}
      </button>
      <button type="button" onClick={onBuild} disabled={busy || state.dirty} data-testid="build">
        {t("toolbar.build")}
      </button>
      <button
        type="button"
        onClick={onCancelBuild}
        disabled={state.build.status !== "running"}
        data-testid="cancel-build"
      >
        {t("toolbar.cancelBuild")}
      </button>
    </div>
  );
}
