import { useEffect, useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";
import { useLocale } from "../i18n/useLocale";

export type ProjectRow = {
  id: string;
  name: string;
  dir: string;
  active: boolean;
};

type Props = {
  disabled?: boolean;
  onSwitched: () => void;
};

export function ProjectSwitcher({ disabled, onSwitched }: Props) {
  const { t } = useLocale();
  const [projects, setProjects] = useState<ProjectRow[]>([]);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancel = false;
    const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
    void client
      .listProjects()
      .then(async (res) => {
        const body = (await res.json()) as { result?: { projects?: ProjectRow[] } };
        if (cancel) return;
        if (!res.ok) {
          setError(t("project.listFailed"));
          setProjects([]);
          return;
        }
        setProjects(body.result?.projects ?? []);
      })
      .catch(() => {
        if (!cancel) {
          setProjects([]);
          setError(t("project.listFailed"));
        }
      });
    return () => {
      cancel = true;
    };
  }, []);

  const select = async (id: string) => {
    setBusy(true);
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.selectProject(id);
      const body = (await res.json()) as { ok?: boolean; detail?: string; result?: { projects?: ProjectRow[] } };
      if (!res.ok || body.ok === false) {
        setError(body.detail ?? t("project.switchFailed"));
        return;
      }
      setProjects(body.result?.projects ?? []);
      onSwitched();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("project.switchFailed"));
    } finally {
      setBusy(false);
    }
  };

  const create = async () => {
    const id = name.trim();
    setBusy(true);
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.createProject(id);
      const body = (await res.json()) as { ok?: boolean; detail?: string; result?: { projects?: ProjectRow[] } };
      if (!res.ok || body.ok === false) {
        setError(body.detail ?? t("project.createFailed"));
        return;
      }
      setProjects(body.result?.projects ?? []);
      setName("");
      setOpen(false);
      onSwitched();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("project.createFailed"));
    } finally {
      setBusy(false);
    }
  };

  const active = projects.find((p) => p.active) ?? projects[0];

  return (
    <div className="project-switcher" data-testid="project-switcher">
      <span className="project-switcher-label">{t("project.label")}</span>
      <select
        data-testid="project-select"
        aria-label={t("project.label")}
        title={projects.length <= 1 ? t("project.onlyHint") : t("project.switchHint")}
        value={active?.id ?? ""}
        disabled={disabled || busy || projects.length === 0}
        onChange={(e) => {
          if (e.target.value && e.target.value !== active?.id) void select(e.target.value);
        }}
      >
        {projects.length === 0 ? <option value="">{t("stack.empty")}</option> : null}
        {projects.map((p) => (
          <option key={p.id} value={p.id} data-testid={"project-tab-" + p.id}>
            {p.name || p.id}
          </option>
        ))}
      </select>
      <button
        type="button"
        data-testid="project-new"
        disabled={disabled || busy}
        title={t("project.onlyHint")}
        onClick={() => setOpen(true)}
      >
        {t("project.new")}
      </button>
      {open ? (
        <form
          className="project-new-form"
          data-testid="project-new-form"
          onSubmit={(e) => {
            e.preventDefault();
            void create();
          }}
        >
          <input
            data-testid="project-new-name"
            value={name}
            placeholder={t("project.namePlaceholder")}
            onChange={(e) => setName(e.target.value)}
            pattern="[a-z][a-z0-9-]{0,62}"
            required
          />
          <button type="submit" disabled={busy || !name.trim()}>
            {t("project.create")}
          </button>
        </form>
      ) : null}
      {error ? (
        <span className="error" role="alert">
          {error}
        </span>
      ) : null}
    </div>
  );
}
