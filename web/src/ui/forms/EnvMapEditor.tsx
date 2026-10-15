import { useState } from "react";
import { useLocale } from "../../i18n/useLocale";
import type { ValueSource } from "../../domain/project";

type Props = {
  value: Record<string, ValueSource> | undefined;
  parameterIds: string[];
  onChange: (next: Record<string, ValueSource> | undefined) => void;
  testId?: string;
  label: string;
  hint: string;
};

type Row = { key: string; mode: "literal" | "parameter" | "secret"; text: string };

function sourceToRaw(v: ValueSource | undefined): { mode: "literal" | "parameter" | "secret"; text: string } {
  if (!v) return { mode: "literal", text: "" };
  if ("secret" in v) return { mode: "secret", text: v.secret };
  if ("parameter" in v) return { mode: "parameter", text: v.parameter };
  return { mode: "literal", text: String(v.literal) };
}

function rowsFrom(env: Record<string, ValueSource> | undefined): Row[] {
  if (!env) return [];
  return Object.keys(env)
    .sort()
    .map((key) => {
      const raw = sourceToRaw(env[key]);
      return { key, mode: raw.mode, text: raw.text };
    });
}

function toEnv(rows: Row[]): Record<string, ValueSource> | undefined {
  const out: Record<string, ValueSource> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (!key) continue;
    if (row.mode === "parameter") {
      const p = row.text.trim();
      if (!p) continue;
      out[key] = { parameter: p };
    } else if (row.mode === "secret") {
      const id = row.text.trim();
      if (!id) continue;
      out[key] = { secret: id };
    } else {
      out[key] = { literal: row.text };
    }
  }
  return Object.keys(out).length ? out : undefined;
}

function committedFingerprint(rows: Row[]): string {
  const env = toEnv(rows) ?? {};
  const keys = Object.keys(env).sort();
  return JSON.stringify(keys.map((k) => [k, env[k]]));
}

function sameCommitted(a: Row[], b: Row[]): boolean {
  return committedFingerprint(a) === committedFingerprint(b);
}

/** Row editor for environment / build-args style KEY → literal|parameter maps. */
export function EnvMapEditor({ value, parameterIds, onChange, testId, label, hint }: Props) {
  const { t } = useLocale();
  const params = [...parameterIds].sort();
  const [rows, setRowsState] = useState<Row[]>(() => rowsFrom(value));
  const incoming = rowsFrom(value);
  if (!sameCommitted(rows, incoming)) {
    setRowsState(incoming);
  }

  const commit = (next: Row[]) => {
    setRowsState(next);
    onChange(toEnv(next));
  };

  return (
    <div className="span2 service-list" data-testid={testId}>
      <div className="section-head">
        <strong>{label}</strong>
        <button
          type="button"
          data-testid={testId ? `${testId}-add` : undefined}
          onClick={() => commit([...rows, { key: "", mode: "literal", text: "" }])}
        >
          {t("env.add")}
        </button>
      </div>
      <p className="muted">{hint}</p>
      {rows.length === 0 ? <p className="muted">{t("svc.none")}</p> : null}
      {rows.map((row, index) => (
        <div
          className="grid2 service-row"
          key={`env-${String(index)}`}
          data-testid={testId ? `${testId}-row-${String(index)}` : undefined}
        >
          <label>
            {t("env.key")}
            <input
              value={row.key}
              onChange={(e) => {
                const next = [...rows];
                next[index] = { ...row, key: e.target.value };
                commit(next);
              }}
            />
          </label>
          <label>
            {t("env.source")}
            <select
              value={row.mode}
              onChange={(e) => {
                const mode = e.target.value as "literal" | "parameter" | "secret";
                const next = [...rows];
                next[index] = {
                  ...row,
                  mode,
                  text: mode === "parameter" ? (params[0] ?? "") : row.text,
                };
                commit(next);
              }}
            >
              <option value="literal">{t("env.literal")}</option>
              <option value="secret">{t("env.secret")}</option>
              <option value="parameter" disabled={params.length === 0}>
                {t("env.parameter")}
              </option>
            </select>
          </label>
          {row.mode === "parameter" ? (
            <label className="span2">
              {t("env.parameter")}
              <select
                value={row.text}
                onChange={(e) => {
                  const next = [...rows];
                  next[index] = { ...row, text: e.target.value };
                  commit(next);
                }}
              >
                {params.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <label className="span2">
              {t("env.value")}
              <input
                value={row.text}
                onChange={(e) => {
                  const next = [...rows];
                  next[index] = { ...row, text: e.target.value };
                  commit(next);
                }}
              />
            </label>
          )}
          <button
            type="button"
            className="linkish"
            onClick={() => commit(rows.filter((_, i) => i !== index))}
          >
            {t("forms.remove")}
          </button>
        </div>
      ))}
    </div>
  );
}
