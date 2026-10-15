import { useState, type ReactNode } from "react";
import { suggestId, validateEntityId } from "../../domain/ids";
import { useLocale } from "../../i18n/useLocale";

type Props = {
  title: string;
  ids: string[];
  testId: string;
  onAdd: (id: string) => void;
  onRemove: (id: string) => void;
  render: (id: string) => ReactNode;
  placeholder?: string;
  suggestPrefix?: string;
};

export function NamedMap({
  title,
  ids,
  testId,
  onAdd,
  onRemove,
  render,
  placeholder,
  suggestPrefix,
}: Props) {
  const { t } = useLocale();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  const submit = () => {
    const errKey = validateEntityId(draft, ids);
    if (errKey) {
      setError(t(errKey));
      return;
    }
    onAdd(draft.trim());
    setDraft("");
    setError(null);
  };

  return (
    <section className="named-map" aria-label={title} data-testid={testId}>
      <div className="section-head">
        <h2>{title}</h2>
      </div>
      <div className="add-row">
        <label className="add-row-label">
          <span className="sr-only">{t("forms.addId")}</span>
          <input
            data-testid={`${testId}-add-id`}
            value={draft}
            placeholder={placeholder ?? t("forms.addId.placeholder")}
            onChange={(e) => {
              setDraft(e.target.value);
              if (error) setError(null);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                submit();
              }
            }}
          />
        </label>
        {suggestPrefix ? (
          <button
            type="button"
            className="linkish"
            data-testid={`${testId}-suggest`}
            onClick={() => {
              setDraft(suggestId(suggestPrefix, ids));
              setError(null);
            }}
          >
            {t("forms.suggestId")}
          </button>
        ) : null}
        <button type="button" data-testid={`${testId}-add`} onClick={submit}>
          {t("forms.add")}
        </button>
      </div>
      {error ? (
        <p className="error" role="alert" data-testid={`${testId}-add-error`}>
          {error}
        </p>
      ) : null}
      {ids.length === 0 && <p className="muted">{t("forms.none")}</p>}
      {ids.map((id) => (
        <details key={id} open className="cardish">
          <summary>
            <strong>{id}</strong>
          </summary>
          <button
            type="button"
            className="linkish cardish-remove"
            data-testid={`${testId}-remove-${id}`}
            onClick={() => onRemove(id)}
          >
            {t("forms.remove")}
          </button>
          {render(id)}
        </details>
      ))}
    </section>
  );
}
