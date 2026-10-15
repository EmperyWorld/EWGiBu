type Props = {
  label: string;
  presets: string[];
  onPick: (value: string) => void;
};

export function GlobChipRow({ label, presets, onPick }: Props) {
  return (
    <div className="glob-chips" role="group" aria-label={label}>
      <span className="muted glob-chips-label">{label}</span>
      {presets.map((p) => (
        <button key={p} type="button" className="glob-chip" onClick={() => onPick(p)}>
          {p}
        </button>
      ))}
    </div>
  );
}
