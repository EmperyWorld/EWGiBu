import { useLocale } from "../i18n/useLocale";

type Props = {
  onImport: () => void;
  onRuntimeGuide: () => void;
};

export function AuthorWelcome({ onImport, onRuntimeGuide }: Props) {
  const { t } = useLocale();
  return (
    <div className="author-welcome" data-testid="author-welcome">
      <h2>{t("welcome.title")}</h2>
      <p>{t("welcome.body")}</p>
      <ol className="welcome-steps">
        <li>{t("welcome.stepImport")}</li>
        <li>{t("welcome.stepEdit")}</li>
        <li>{t("welcome.stepSave")}</li>
        <li>{t("welcome.stepCli")}</li>
      </ol>
      <div className="welcome-actions">
        <button type="button" data-testid="welcome-import" onClick={onImport}>
          {t("toolbar.import")}
        </button>
        <button type="button" data-testid="welcome-runtime" onClick={onRuntimeGuide}>
          {t("toolbar.runtime")}
        </button>
      </div>
    </div>
  );
}
