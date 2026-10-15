import type { Mount } from "../../domain/project";
import { useLocale } from "../../i18n/useLocale";

type Props = {
  mounts: Mount[];
  volumeIds: string[];
  resourceIds: string[];
  testIdPrefix: string;
  onChange: (next: Mount[] | undefined) => void;
};

export function MountListEditor({
  mounts,
  volumeIds,
  resourceIds,
  testIdPrefix,
  onChange,
}: Props) {
  const { t } = useLocale();

  const setMount = (index: number, mount: Mount) => {
    const next = [...mounts];
    next[index] = mount;
    onChange(next);
  };

  const addVolumeMount = () => {
    const volume = volumeIds[0];
    if (!volume) return;
    onChange([...mounts, { volume, target: "/data", readOnly: false }]);
  };

  const addResourceMount = () => {
    const resource = resourceIds[0];
    if (!resource) return;
    onChange([...mounts, { resource, target: `/mnt/${resource}`, readOnly: true }]);
  };

  const removeMount = (index: number) => {
    const next = mounts.filter((_, i) => i !== index);
    onChange(next.length ? next : undefined);
  };

  return (
    <div className="span2 service-list">
      <div className="section-head">
        <strong>{t("svc.mounts")}</strong>
        <div className="picker-row">
          <button
            type="button"
            data-testid={`${testIdPrefix}-add-mount`}
            onClick={addVolumeMount}
            disabled={volumeIds.length === 0}
          >
            {t("svc.addMount")}
          </button>
          <button
            type="button"
            data-testid={`${testIdPrefix}-add-resource-mount`}
            onClick={addResourceMount}
            disabled={resourceIds.length === 0}
          >
            {t("svc.addResourceMount")}
          </button>
        </div>
      </div>
      {mounts.length === 0 ? <p className="muted">{t("svc.none")}</p> : null}
      {mounts.map((mount, index) => {
        const isResource = Boolean(mount.resource);
        return (
          <div
            className="grid2 service-row"
            key={`mount-${String(index)}`}
            data-testid={`${testIdPrefix}-mount-row-${String(index)}`}
          >
            <label>
              {t("svc.mountKind")}
              <select
                value={isResource ? "resource" : "volume"}
                onChange={(e) => {
                  if (e.target.value === "resource") {
                    setMount(index, {
                      resource: resourceIds[0] ?? "",
                      target: mount.target || "/mnt/data",
                      readOnly: true,
                    });
                  } else {
                    setMount(index, {
                      volume: volumeIds[0] ?? "",
                      target: mount.target || "/data",
                      readOnly: false,
                    });
                  }
                }}
              >
                <option value="volume" disabled={volumeIds.length === 0}>
                  {t("svc.mountVolume")}
                </option>
                <option value="resource" disabled={resourceIds.length === 0}>
                  {t("svc.mountResource")}
                </option>
              </select>
            </label>
            {isResource ? (
              <label>
                {t("svc.mountResource")}
                <select
                  value={mount.resource ?? ""}
                  onChange={(e) =>
                    setMount(index, {
                      resource: e.target.value,
                      target: mount.target,
                      source: mount.source,
                      readOnly: mount.readOnly ?? true,
                    })
                  }
                >
                  {resourceIds.map((r) => (
                    <option key={r} value={r}>
                      {r}
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              <label>
                {t("svc.mountVolume")}
                <select
                  value={mount.volume ?? ""}
                  onChange={(e) =>
                    setMount(index, {
                      volume: e.target.value,
                      target: mount.target,
                      readOnly: mount.readOnly,
                    })
                  }
                >
                  {volumeIds.map((v) => (
                    <option key={v} value={v}>
                      {v}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              {t("svc.mountTarget")}
              <input
                value={mount.target}
                onChange={(e) => setMount(index, { ...mount, target: e.target.value })}
              />
            </label>
            {isResource ? (
              <label>
                {t("svc.mountSource")}
                <input
                  value={mount.source ?? ""}
                  onChange={(e) =>
                    setMount(index, { ...mount, source: e.target.value || undefined })
                  }
                />
              </label>
            ) : null}
            <label>
              {t("svc.mountRo")}
              <select
                value={mount.readOnly ? "yes" : "no"}
                onChange={(e) => setMount(index, { ...mount, readOnly: e.target.value === "yes" })}
              >
                <option value="no">{t("forms.no")}</option>
                <option value="yes">{t("forms.yes")}</option>
              </select>
            </label>
            <button type="button" className="linkish" onClick={() => removeMount(index)}>
              {t("svc.removeMount")}
            </button>
          </div>
        );
      })}
    </div>
  );
}
