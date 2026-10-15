import { useLocale } from "../i18n/useLocale";

export type StubPathGroups = {
  dirs: string[];
  files: string[];
  buildContexts: string[];
  skipped?: string[];
};

type Props = {
  groups: StubPathGroups;
  testId?: string;
};

export function ImportStubPathLists({ groups, testId }: Props) {
  const { t } = useLocale();
  const hasAny =
    groups.dirs.length > 0 ||
    groups.files.length > 0 ||
    groups.buildContexts.length > 0 ||
    (groups.skipped?.length ?? 0) > 0;
  if (!hasAny) return null;

  return (
    <div className="import-stub-path-lists" data-testid={testId}>
      {groups.dirs.length > 0 ? (
        <div>
          <strong>{t("import.stubsDirs")}</strong>
          <ul>
            {groups.dirs.map((p) => (
              <li key={"d-" + p}>{p}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {groups.files.length > 0 ? (
        <div>
          <strong>{t("import.stubsFiles")}</strong>
          <ul>
            {groups.files.map((p) => (
              <li key={"f-" + p}>{p}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {groups.buildContexts.length > 0 ? (
        <div>
          <strong>{t("import.stubsBuildContexts")}</strong>
          <ul>
            {groups.buildContexts.map((p) => (
              <li key={"b-" + p}>{p}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {(groups.skipped?.length ?? 0) > 0 ? (
        <p className="muted" data-testid="import-stub-skipped">
          {t("import.stubsSkipped")}: {groups.skipped!.join(", ")}
        </p>
      ) : null}
    </div>
  );
}
