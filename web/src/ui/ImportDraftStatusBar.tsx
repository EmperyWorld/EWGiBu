import { useLocale } from "../i18n/useLocale";

type Props = {
  inMemory: boolean;
  onDisk: boolean;
  stubsPrepared: boolean;
  missingBefore: number | null;
  missingAfter: number | null;
};

export function ImportDraftStatusBar({
  inMemory,
  onDisk,
  stubsPrepared,
  missingBefore,
  missingAfter,
}: Props) {
  const { t } = useLocale();
  if (!inMemory) return null;

  return (
    <div className="import-draft-status" data-testid="import-draft-status">
      <span className="import-status-chip active">{t("import.status.memory")}</span>
      {onDisk ? (
        <span className="import-status-chip ok">{t("import.status.onDisk")}</span>
      ) : (
        <span className="import-status-chip muted">{t("import.status.notOnDisk")}</span>
      )}
      {stubsPrepared ? (
        <span className="import-status-chip ok">{t("import.status.stubs")}</span>
      ) : (
        <span className="import-status-chip muted">{t("import.status.noStubs")}</span>
      )}
      {missingBefore !== null && missingAfter !== null && missingBefore !== missingAfter ? (
        <span className="import-status-chip" data-testid="import-paths-delta">
          {t("import.status.pathsDelta")
            .replace("{before}", String(missingBefore))
            .replace("{after}", String(missingAfter))}
        </span>
      ) : null}
    </div>
  );
}
