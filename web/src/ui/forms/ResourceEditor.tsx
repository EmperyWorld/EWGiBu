import type { Command } from "../../domain/commands";
import type { Project, Resource } from "../../domain/project";
import { splitDockerfilePath, useDiscoverSuggestions } from "../../app/useDiscover";
import { useLocale } from "../../i18n/useLocale";
import { GlobChipRow } from "./GlobChipRow";
import { UnmodeledFields } from "./UnmodeledFields";
import { KNOWN_RESOURCE, mergeUnmodeled } from "./unmodeled";

type Props = {
  project: Project;
  resourceId: string;
  resource: Resource;
  dispatch: (cmd: Command) => void;
};

const INCLUDE_PRESETS = ["**", "**/*", "src/**", "app/**"];
const EXCLUDE_PRESETS = [".git/**", ".env", "dist/**", "node_modules/**", "**/*.md"];

export function ResourceEditor({ resourceId, resource: r, dispatch }: Props) {
  const { t } = useLocale();
  const { suggestions, loading, reload } = useDiscoverSuggestions();
  const contexts = [
    ".",
    ...new Set(
      suggestions
        .filter((s) => s.kind === "dockerfile")
        .map((s) => splitDockerfilePath(s.path).context),
    ),
  ].sort();

  const patch = (next: Partial<Resource>) => {
    dispatch({
      type: "upsertResource",
      id: resourceId,
      value: { ...r, ...next },
    });
  };

  return (
    <div
      className="grid2 resource-editor"
      data-testid={`resource-${resourceId}`}
      data-field-path={`/spec/resources/${resourceId}`}
    >
      <label className="span2">
        {t("forms.source")}
        <div className="picker-row">
          <input
            data-testid={`resource-source-${resourceId}`}
            value={r.source ?? "."}
            list={`resource-source-list-${resourceId}`}
            onChange={(e) => patch({ source: e.target.value })}
          />
          <select
            data-testid={`resource-source-pick-${resourceId}`}
            value=""
            disabled={loading}
            onChange={(e) => {
              if (e.target.value) patch({ source: e.target.value });
            }}
          >
            <option value="">{loading ? t("img.discoverLoading") : t("res.pickSource")}</option>
            {contexts.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <button type="button" className="linkish" onClick={reload}>
            {t("img.discoverReload")}
          </button>
        </div>
        <datalist id={`resource-source-list-${resourceId}`}>
          {contexts.map((c) => (
            <option key={c} value={c} />
          ))}
        </datalist>
      </label>
      <label>
        {t("res.destination")}
        <input
          value={r.destination ?? ""}
          onChange={(e) => patch({ destination: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("res.mode")}
        <input
          value={r.mode ?? ""}
          placeholder="0644"
          onChange={(e) => patch({ mode: e.target.value || undefined })}
        />
      </label>
      <label>
        {t("res.allowEmpty")}
        <select
          value={r.allowEmpty ? "yes" : "no"}
          onChange={(e) => patch({ allowEmpty: e.target.value === "yes" })}
        >
          <option value="no">{t("forms.no")}</option>
          <option value="yes">{t("forms.yes")}</option>
        </select>
      </label>
      <label className="span2">
        {t("forms.include")}
        <input
          data-testid={`resource-include-${resourceId}`}
          value={(r.include ?? ["**"]).join(", ")}
          onChange={(e) =>
            patch({
              include: e.target.value
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean),
            })
          }
        />
        <GlobChipRow
          label={t("glob.presets")}
          presets={INCLUDE_PRESETS}
          onPick={(g) => {
            const cur = r.include ?? [];
            if (cur.includes(g)) return;
            patch({ include: [...cur, g] });
          }}
        />
      </label>
      <label className="span2">
        {t("img.exclude")}
        <input
          value={(r.exclude ?? []).join(", ")}
          onChange={(e) => {
            const exclude = e.target.value
              .split(",")
              .map((s) => s.trim())
              .filter(Boolean);
            patch({ exclude: exclude.length ? exclude : undefined });
          }}
        />
        <GlobChipRow
          label={t("glob.presets")}
          presets={EXCLUDE_PRESETS}
          onPick={(g) => {
            const cur = r.exclude ?? [];
            if (cur.includes(g)) return;
            patch({ exclude: [...cur, g] });
          }}
        />
      </label>
      <UnmodeledFields
        testId={`resource-unmodeled-${resourceId}`}
        value={r as Record<string, unknown>}
        known={KNOWN_RESOURCE}
        onApply={(extra) =>
          dispatch({
            type: "upsertResource",
            id: resourceId,
            value: mergeUnmodeled(r as Record<string, unknown>, KNOWN_RESOURCE, extra) as Resource,
          })
        }
      />
    </div>
  );
}
