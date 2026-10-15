import { useLocale } from "../i18n/useLocale";
import { IMPORT_WIZARD_STEPS, type ImportWizardStep } from "./importWizard";

type Props = {
  step: ImportWizardStep;
  showPathsStep: boolean;
  onStep: (step: ImportWizardStep) => void;
  draftReady: boolean;
};

const STEP_I18N: Record<ImportWizardStep, string> = {
  sources: "import.wizard.sources",
  review: "import.wizard.review",
  paths: "import.wizard.paths",
  onboard: "import.wizard.onboard",
  accept: "import.wizard.accept",
};

export function ImportWizardNav({ step, showPathsStep, onStep, draftReady }: Props) {
  const { t } = useLocale();
  const steps = showPathsStep
    ? IMPORT_WIZARD_STEPS
    : IMPORT_WIZARD_STEPS.filter((s) => s !== "paths");

  return (
    <nav className="import-wizard-nav" aria-label={t("import.wizard.nav")} data-testid="import-wizard-nav">
      <ol>
        {steps.map((s, idx) => {
          const active = s === step;
          const done =
            s === "sources"
              ? draftReady
              : steps.indexOf(s) < steps.indexOf(step);
          const disabled = s !== "sources" && !draftReady;
          return (
            <li key={s} className={active ? "active" : done ? "done" : ""}>
              <button
                type="button"
                disabled={disabled}
                data-testid={"import-wizard-tab-" + s}
                onClick={() => onStep(s)}
              >
                <span className="import-wizard-num">{idx + 1}</span>
                {t(STEP_I18N[s] as Parameters<typeof t>[0])}
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
