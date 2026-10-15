import { useState } from "react";
import { useLocale } from "../../i18n/useLocale";
import { pickUnmodeled } from "./unmodeled";

type Props = {
  value: Record<string, unknown>;
  known: Set<string>;
  onApply: (extra: Record<string, unknown>) => void;
  testId?: string;
};

/** Safe JSON editor for schema-known-but-unmodeled fields. */
export function UnmodeledFields({ value, known, onApply, testId }: Props) {
  const { t } = useLocale();
  const extra = pickUnmodeled(value, known);
  const serialized = JSON.stringify(extra, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState<string | null>(null);
  const [synced, setSynced] = useState(serialized);
  if (serialized !== synced) {
    setSynced(serialized);
    setText(serialized);
    setError(null);
  }

  return (
    <label className="span2">
      {t("forms.unmodeled")}
      <textarea
        data-testid={testId}
        rows={3}
        value={text}
        spellCheck={false}
        onChange={(e) => {
          setText(e.target.value);
          setError(null);
        }}
        onBlur={() => {
          try {
            const parsed = JSON.parse(text === "" ? "{}" : text) as unknown;
            if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
              setError(t("forms.unmodeled.invalid"));
              return;
            }
            const record = parsed as Record<string, unknown>;
            for (const k of Object.keys(record)) {
              if (known.has(k)) {
                setError(t("forms.unmodeled.knownKey"));
                return;
              }
            }
            setError(null);
            onApply(record);
          } catch {
            setError(t("forms.unmodeled.invalid"));
          }
        }}
      />
      {error ? (
        <span className="error" role="alert">
          {error}
        </span>
      ) : null}
    </label>
  );
}
