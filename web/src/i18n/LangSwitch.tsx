import { useLocale } from "./useLocale";
import type { Locale } from "../i18n/messages";

export function LangSwitch() {
  const { locale, setLocale, t } = useLocale();
  return (
    <div className="lang-switch" role="group" aria-label={t("lang.label")}>
      {(["en", "ru"] as Locale[]).map((code) => (
        <button
          key={code}
          type="button"
          className={locale === code ? "active" : ""}
          aria-pressed={locale === code}
          data-testid={`lang-${code}`}
          onClick={() => setLocale(code)}
        >
          {code.toUpperCase()}
        </button>
      ))}
    </div>
  );
}
