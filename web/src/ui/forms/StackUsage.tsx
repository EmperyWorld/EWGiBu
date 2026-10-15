type Props = {
  title: string;
  lines: Array<{ label: string; ids: string[] }>;
  noneLabel: string;
  testId?: string;
};

/** Compact “who uses this stack entity” read-out. */
export function StackUsage({ title, lines, noneLabel, testId }: Props) {
  return (
    <div className="stack-affinity" data-testid={testId}>
      <strong>{title}</strong>
      {lines.map((line) => (
        <p key={line.label}>
          {line.label}: {line.ids.length ? line.ids.join(", ") : noneLabel}
        </p>
      ))}
    </div>
  );
}
