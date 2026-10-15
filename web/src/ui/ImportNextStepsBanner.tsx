import { useLocale } from "../i18n/useLocale";

type Props = {
  open: boolean;
  onSave: () => void;
  onSaveAs: () => void;
  onValidate: () => void;
  onRuntimeGuide: () => void;
  onDismiss: () => void;
};

export function ImportNextStepsBanner({
  open,
  onSave,
  onSaveAs,
  onValidate,
  onRuntimeGuide,
  onDismiss,
}: Props) {
  const { t } = useLocale();
  if (!open) return null;

  return (
    <aside className="import-next-steps" role="status" data-testid="import-next-steps">
      <div className="import-next-steps-inner">
        <strong>{t("import.nextSteps.title")}</strong>
        <p className="muted">{t("import.nextSteps.body")}</p>
        <ol>
          <li>{t("import.nextSteps.step1")}</li>
          <li>{t("import.nextSteps.step2")}</li>
          <li>{t("import.nextSteps.step3")}</li>
        </ol>
        <div className="import-next-steps-actions">
          <button type="button" data-testid="import-next-save" onClick={onSave}>
            {t("toolbar.save")}
          </button>
          <button type="button" data-testid="import-next-save-as" onClick={onSaveAs}>
            {t("toolbar.saveAs")}
          </button>
          <button type="button" data-testid="import-next-validate" onClick={onValidate}>
            {t("toolbar.validate")}
          </button>
          <button type="button" data-testid="import-next-runtime" onClick={onRuntimeGuide}>
            {t("toolbar.runtime")}
          </button>
          <button type="button" className="import-next-dismiss" data-testid="import-next-dismiss" onClick={onDismiss}>
            {t("import.nextSteps.dismiss")}
          </button>
        </div>
      </div>
    </aside>
  );
}
