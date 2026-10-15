import { useLocale } from "../../i18n/useLocale";

type Props = {
  label: string;
  ids: string[];
  selected: string[];
  onChange: (next: string[]) => void;
  testId?: string;
  emptyHint?: string;
};

/** Checkbox multi-select bound to stack entity ids (sorted display). */
export function IdChecklist({ label, ids, selected, onChange, testId, emptyHint }: Props) {
  const { t } = useLocale();
  const selectedSet = new Set(selected);
  const sorted = [...ids].sort();

  const toggle = (id: string) => {
    if (selectedSet.has(id)) {
      onChange(selected.filter((x) => x !== id));
    } else {
      onChange([...selected, id].sort());
    }
  };

  return (
    <fieldset className="id-checklist span2" data-testid={testId}>
      <legend>{label}</legend>
      {sorted.length === 0 ? (
        <p className="muted">{emptyHint ?? t("stack.empty")}</p>
      ) : (
        <div className="id-checklist-grid">
          {sorted.map((id) => (
            <label key={id} className="id-checklist-item">
              <input
                type="checkbox"
                checked={selectedSet.has(id)}
                onChange={() => toggle(id)}
              />
              <span>{id}</span>
            </label>
          ))}
        </div>
      )}
    </fieldset>
  );
}
