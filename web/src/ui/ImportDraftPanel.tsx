import { useRef, useState } from "react";
import { IacpackClient } from "../api/generated/client";
import { readCsrf } from "../app/csrf";
import { useDiscoverSuggestions } from "../app/useDiscover";
import { markImportNextSteps } from "../app/importNextSteps";
import {
  buildImportStubsCliScript,
  defaultImportDraftPath,
  useWorkspaceInfo,
} from "../app/useWorkspaceInfo";
import type { Project } from "../domain/project";
import type { MessageKey } from "../i18n/messages";
import { useLocale } from "../i18n/useLocale";
import { FileDropUpload } from "./FileDropUpload";
import { draftJsonDownloadFilename, downloadProjectJson } from "./downloadDraftJson";
import { ImportDiagnosticsPanel } from "./ImportDiagnosticsPanel";
import { ImportBindPathChooser } from "./ImportBindPathChooser";
import type { BindChoiceState, BindPlanItem } from "./ImportBindPathChooser.types";
import { buildBindChoicesPayload } from "./ImportBindPathChooser.types";
import { ImportStubPathLists } from "./ImportStubPathLists";
import { stubPathCreateCount, stubPathHasAny } from "./importStubCounts";
import { importServiceRows } from "./importServiceRows";
import { ImportDraftStatusBar } from "./ImportDraftStatusBar";
import { ImportWizardNav } from "./ImportWizardNav";
import { SecretFileTools } from "./forms/SecretFileTools";
import { generateAllSecrets } from "./forms/generateSecrets";
import { classifySecret, type SecretKind } from "./forms/secretKind";
import {
  acceptBlockReason,
  countImportErrors,
  countImportPathWarnings,
  countUnfilledParameterGaps,
  nextWizardStep,
  parseActiveProfilesInput,
  prevWizardStep,
  type ImportWizardStep,
} from "./importWizard";

export type SecretSuggestion = {
  id: string;
  source: string;
  path: string;
  pointer?: string;
  envFile?: string;
  envKey?: string;
  service?: string;
  target?: string;
  description?: string;
};

export type ParameterGap = {
  id: string;
  phase: string;
  type: string;
  reason: string;
};

export type ImportProposeResult = {
  project: Project;
  summary: {
    services: number;
    images: number;
    networks: number;
    volumes: number;
    resources: number;
    parameters: number;
  };
  sources: Record<string, unknown>;
  refusals?: Array<{
    path: string;
    pointer?: string;
    code: string;
    category: string;
    message: string;
  }>;
  parameterGaps?: ParameterGap[];
  secretSuggestions?: SecretSuggestion[];
  envExample?: string;
};

type DiagnosticRow = {
  code?: string;
  severity?: string;
  message?: string;
  hint?: string;
};

type Props = {
  open: boolean;
  onClose: () => void;
  onAccept: (project: Project) => void;
};

const REFUSAL_ORDER = ["policy", "collision", "secret", "path", "unsupported"] as const;
const UNSUPPORTED_REFUSAL_PREVIEW = 6;

function defaultProjectNameFromCompose(path: string): string {
  const base = path.split("/").pop() ?? path;
  const stem = base.replace(/\.(ya?ml)$/i, "").replace(/^\d+-/, "");
  const cleaned = stem.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "");
  return cleaned || "imported";
}

function suggestionHints(s: SecretSuggestion): string[] {
  return [s.id, s.envKey, s.source, s.target, s.description, s.service, s.envFile].filter(
    (v): v is string => Boolean(v),
  );
}

export function ImportDraftPanel({ open, onClose, onAccept }: Props) {
  const { t } = useLocale();
  const { info: workspaceInfo } = useWorkspaceInfo();
  const { suggestions, loading, reload } = useDiscoverSuggestions();
  const composeHits = suggestions.filter((s) => s.kind === "compose");
  const dockerfileHits = suggestions.filter((s) => s.kind === "dockerfile");
  const [selectedCompose, setSelectedCompose] = useState<string[]>([]);
  const [composeExtra, setComposeExtra] = useState("");
  const [selectedDockerfiles, setSelectedDockerfiles] = useState<string[]>([]);
  const [dockerfileExtra, setDockerfileExtra] = useState("");
  const [name, setName] = useState("imported");
  const [activeProfilesInput, setActiveProfilesInput] = useState("");
  const [importStrict, setImportStrict] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [diags, setDiags] = useState<DiagnosticRow[]>([]);
  const [showAllUnsupported, setShowAllUnsupported] = useState(false);
  const [draft, setDraft] = useState<ImportProposeResult | null>(null);
  const [selectedSecrets, setSelectedSecrets] = useState<string[]>([]);
  const [secretKinds, setSecretKinds] = useState<Record<string, SecretKind>>({});
  const [secretNotes, setSecretNotes] = useState<Record<string, string>>({});
  const [secretGenBusy, setSecretGenBusy] = useState(false);
  const [secretBatchNote, setSecretBatchNote] = useState("");
  const [paramDefaults, setParamDefaults] = useState<Record<string, string>>({});
  const [writeEnvExample, setWriteEnvExample] = useState(false);
  const [stubReport, setStubReport] = useState<{
    dirs: string[];
    files: string[];
    buildContexts: string[];
    skipped: string[];
  } | null>(null);
  const [stubPreview, setStubPreview] = useState<{
    dirs: string[];
    files: string[];
    buildContexts: string[];
    skipped?: string[];
    bindItems?: BindPlanItem[];
  } | null>(null);
  const [bindChoices, setBindChoices] = useState<Record<string, BindChoiceState>>({});
  const [stubPreviewBusy, setStubPreviewBusy] = useState(false);
  const [draftCliPath, setDraftCliPath] = useState(defaultImportDraftPath);
  const [cliCopied, setCliCopied] = useState(false);
  const [wizardStep, setWizardStep] = useState<ImportWizardStep>("sources");
  const [proposeOk, setProposeOk] = useState(true);
  const [draftOnDisk, setDraftOnDisk] = useState(false);
  const [stubsPrepared, setStubsPrepared] = useState(false);
  const [missingPathsBefore, setMissingPathsBefore] = useState<number | null>(null);
  const [missingPathsAfter, setMissingPathsAfter] = useState<number | null>(null);
  const [expertMode, setExpertMode] = useState(false);
  const wizardNextRef = useRef<HTMLButtonElement>(null);

  if (!open) return null;

  const closePanel = () => {
    setWizardStep(draft ? "review" : "sources");
    onClose();
  };

  const saveDraftJsonForCli = async (): Promise<boolean> => {
    if (!draft?.project) return false;
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.importWriteDraft({
        project: draft.project,
        path: draftCliPath,
        replace: true,
      });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: { path?: string };
        detail?: string;
        diagnostics?: DiagnosticRow[];
      };
      if (!body.ok) {
        setError(body.detail ?? body.diagnostics?.[0]?.message ?? t("import.saveDraftFailed"));
        return false;
      }
      const path = body.result?.path ?? draftCliPath;
      setDraftCliPath(path);
      setDraftOnDisk(true);
      setNotice(t("import.draftSaved").replace("{path}", path));
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("import.saveDraftFailed"));
      return false;
    }
  };

  const downloadDraftJson = () => {
    if (!draft?.project) return;
    const filename = draftJsonDownloadFilename(name);
    downloadProjectJson(draft.project, filename);
    setNotice(t("import.draftDownloaded").replace("{file}", filename));
  };

  const applyResourceStubs = async (opts?: { quiet?: boolean }): Promise<boolean> => {
    if (!draft?.project) return false;
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.importResourceStubs({ project: draft.project });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: {
          createdDirs?: string[];
          createdFiles?: string[];
          createdBuildContexts?: string[];
          skipped?: string[];
        };
        detail?: string;
        diagnostics?: DiagnosticRow[];
      };
      if (body.diagnostics?.length) {
        setDiags(body.diagnostics);
      }
      if (!body.ok) {
        setError(body.detail ?? body.diagnostics?.[0]?.message ?? t("import.stubsFailed"));
        return false;
      }
      const report = {
        dirs: body.result?.createdDirs ?? [],
        files: body.result?.createdFiles ?? [],
        buildContexts: body.result?.createdBuildContexts ?? [],
        skipped: body.result?.skipped ?? [],
      };
      setStubReport(report);
      if (report.dirs.length + report.files.length + report.buildContexts.length > 0) {
        setStubsPrepared(true);
      }
      const createdCount = report.dirs.length + report.files.length + report.buildContexts.length;
      if (!opts?.quiet) {
        if (createdCount === 0) {
          setNotice(t("import.stubsNothing"));
        } else {
          setNotice(t("import.stubsOk"));
        }
      }
      await runPropose({ resetSecrets: false, preserveWizardStep: true });
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("import.stubsFailed"));
      return false;
    }
  };

  const prepareWorkspace = async () => {
    if (!draft?.project) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const projectForPrepare =
        (await refreshStubPreview(draft.project, bindChoices)) ?? draft.project;
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const res = await client.importPrepare({
        project: projectForPrepare,
        path: draftCliPath,
        replace: true,
      });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: {
          draftPath?: string;
          stubs?: {
            createdDirs?: string[];
            createdFiles?: string[];
            createdBuildContexts?: string[];
            skipped?: string[];
          };
        };
        detail?: string;
        diagnostics?: DiagnosticRow[];
      };
      if (body.diagnostics?.length) {
        setDiags(body.diagnostics);
      }
      if (!body.ok || !body.result) {
        setError(body.detail ?? body.diagnostics?.[0]?.message ?? t("import.prepareFailed"));
        return;
      }
      const path = body.result.draftPath ?? draftCliPath;
      setDraftCliPath(path);
      const report = {
        dirs: body.result.stubs?.createdDirs ?? [],
        files: body.result.stubs?.createdFiles ?? [],
        buildContexts: body.result.stubs?.createdBuildContexts ?? [],
        skipped: body.result.stubs?.skipped ?? [],
      };
      setStubReport(report);
      setDraftOnDisk(true);
      setStubsPrepared(true);
      setMissingPathsAfter(countImportPathWarnings(body.diagnostics ?? []));
      const created = report.dirs.length + report.files.length + report.buildContexts.length;
      await runPropose({ resetSecrets: false, preserveWizardStep: true });
      setNotice(
        t("import.prepareWorkspaceOk")
          .replace("{path}", path)
          .replace("{n}", String(created)),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : t("import.prepareFailed"));
    } finally {
      setBusy(false);
    }
  };

  const copyCliRecipe = async () => {
    if (!workspaceInfo) return;
    const text = buildImportStubsCliScript(workspaceInfo, draftCliPath);
    try {
      await navigator.clipboard.writeText(text);
      setCliCopied(true);
      window.setTimeout(() => setCliCopied(false), 2000);
    } catch {
      setError(t("import.copyCliFailed"));
    }
  };

  const mergePathDiags = (pathRows: DiagnosticRow[]) => {
    setDiags((prev) => {
      const rest = prev.filter(
        (d) => !(d.code?.includes("IMPORT") && d.code?.includes("PATH")),
      );
      return [...rest, ...pathRows];
    });
    setMissingPathsAfter(countImportPathWarnings(pathRows));
  };

  const refreshStubPreview = async (
    project: Project,
    choices: Record<string, BindChoiceState> = bindChoices,
  ): Promise<Project | null> => {
    setStubPreviewBusy(true);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const payload = buildBindChoicesPayload(choices);
      const res = await client.importStubPreview({
        project,
        bindChoices: payload.length ? payload : undefined,
      });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: {
          plannedDirs?: string[];
          plannedFiles?: string[];
          plannedBuildContexts?: string[];
          skipped?: string[];
          bindItems?: BindPlanItem[];
          project?: Project;
        };
        pathDiagnostics?: DiagnosticRow[];
      };
      if (!body.ok || !body.result) {
        setStubPreview(null);
        return null;
      }
      const nextProject = body.result.project ?? project;
      if (body.result.project) {
        setDraft((prev) => (prev ? { ...prev, project: nextProject } : prev));
      }
      if (body.pathDiagnostics) {
        mergePathDiags(body.pathDiagnostics);
      }
      setStubPreview({
        dirs: body.result.plannedDirs ?? [],
        files: body.result.plannedFiles ?? [],
        buildContexts: body.result.plannedBuildContexts ?? [],
        skipped: body.result.skipped ?? [],
        bindItems: body.result.bindItems ?? [],
      });
      return nextProject;
    } catch {
      setStubPreview(null);
      return null;
    } finally {
      setStubPreviewBusy(false);
    }
  };

  const toggle = (list: string[], path: string, set: (next: string[]) => void) => {
    set(list.includes(path) ? list.filter((p) => p !== path) : [...list, path].sort());
  };

  const composePaths = (() => {
    const extra = composeExtra
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean);
    const all = [...selectedCompose, ...extra];
    return [...new Set(all)];
  })();
  const dockerfilePaths = (() => {
    const extra = dockerfileExtra
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean);
    return [...new Set([...selectedDockerfiles, ...extra])];
  })();

  const uniqueSecretSuggestions = (() => {
    const seen = new Set<string>();
    const out: SecretSuggestion[] = [];
    for (const s of draft?.secretSuggestions ?? []) {
      if (seen.has(s.id)) continue;
      seen.add(s.id);
      out.push(s);
    }
    return out;
  })();

  function kindLabel(kind: SecretKind): string {
    const key: MessageKey =
      kind === "port"
        ? "forms.secret.kind.port"
        : kind === "name"
          ? "forms.secret.kind.name"
          : "forms.secret.kind.password";
    return t(key);
  }

  function detectImportSecret(s: SecretSuggestion) {
    const found = classifySecret(suggestionHints(s));
    setSecretKinds((prev) => ({ ...prev, [s.id]: found.kind }));
    setSecretNotes((prev) => ({
      ...prev,
      [s.id]: found.confident
        ? t("forms.secret.detected").replace("{kind}", kindLabel(found.kind))
        : t("forms.secret.unsure"),
    }));
  }

  async function generateImportSecrets() {
    setSecretGenBusy(true);
    try {
      const result = await generateAllSecrets(
        uniqueSecretSuggestions.map((s) => ({
          id: s.id,
          kind: secretKinds[s.id] ?? classifySecret(suggestionHints(s)).kind,
        })),
        (count) => window.confirm(t("forms.secret.replaceAllAsk").replace("{n}", String(count))),
      );
      setSecretBatchNote(
        t("forms.secret.generateAllDone")
          .replace("{wrote}", String(result.wrote))
          .replace("{kept}", String(result.kept))
          .replace("{failed}", String(result.failed)),
      );
    } catch {
      setSecretBatchNote(t("forms.secret.failed"));
    } finally {
      setSecretGenBusy(false);
    }
  }

  const applyProposeResponse = (
    body: {
      ok?: boolean;
      result?: ImportProposeResult;
      diagnostics?: DiagnosticRow[];
      detail?: string;
    },
    opts: { resetSecrets: boolean; preserveWizardStep?: boolean },
  ) => {
    const nextDiags = body.diagnostics ?? [];
    setDiags(nextDiags);
    if (!body.result?.project) {
      setError(body.detail ?? t("import.noDraft"));
      return false;
    }
    setDraft(body.result);
    setProposeOk(body.ok !== false);
    const pathWarns = countImportPathWarnings(nextDiags);
    setMissingPathsBefore((prev) => (prev === null ? pathWarns : prev));
    setMissingPathsAfter(pathWarns);
    if (!opts.preserveWizardStep) {
      setWizardStep("review");
    }
    if (body.result.project) {
      void refreshStubPreview(body.result.project);
    }
    if (opts.resetSecrets) {
      const ids = [...new Set((body.result.secretSuggestions ?? []).map((s) => s.id))].sort();
      setSelectedSecrets(ids);
    }
    setError(null);
    if (!body.ok) {
      setError(t("import.partial"));
    }
    return true;
  };

  const runPropose = async (opts: { resetSecrets: boolean; preserveWizardStep?: boolean }) => {
    const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
    const activeProfiles = parseActiveProfilesInput(activeProfilesInput);
    const res = await client.importPropose({
      composes: composePaths.length ? composePaths : undefined,
      dockerfiles: dockerfilePaths.length ? dockerfilePaths : undefined,
      name: name.trim() || undefined,
      activeProfiles: activeProfiles.length ? activeProfiles : undefined,
      strict: importStrict || undefined,
      existingProject: draft?.project,
    });
    const body = (await res.json()) as {
      ok?: boolean;
      result?: ImportProposeResult;
      diagnostics?: DiagnosticRow[];
      detail?: string;
    };
    return applyProposeResponse(body, opts);
  };

  const propose = async () => {
    setBusy(true);
    setError(null);
    setNotice(null);
    setStubReport(null);
    setStubPreview(null);
    setBindChoices({});
    setDraft(null);
    setProposeOk(true);
    setDraftOnDisk(false);
    setStubsPrepared(false);
    setMissingPathsBefore(null);
    setMissingPathsAfter(null);
    setWizardStep("sources");
    setDiags([]);
    setShowAllUnsupported(false);
    setSelectedSecrets([]);
    setParamDefaults({});
    let proposed = false;
    try {
      proposed = await runPropose({ resetSecrets: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : t("import.failed"));
    } finally {
      setBusy(false);
      if (proposed) {
        requestAnimationFrame(() => wizardNextRef.current?.focus());
      }
    }
  };

  const createStubs = async () => {
    if (!draft?.project) return;
    setBusy(true);
    try {
      await applyResourceStubs();
    } finally {
      setBusy(false);
    }
  };

  const accept = async () => {
    if (!draft?.project) return;
    setBusy(true);
    setError(null);
    try {
      const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
      const defaults: Record<string, string | number | boolean> = {};
      for (const [id, raw] of Object.entries(paramDefaults)) {
        const v = raw.trim();
        if (!v) continue;
        defaults[id] = v;
      }
      const res = await client.importOnboard({
        project: draft.project,
        secretIds: selectedSecrets,
        suggestions: draft.secretSuggestions ?? [],
        paramDefaults: defaults,
        attachMounts: true,
        writeEnvExample,
      });
      const body = (await res.json()) as {
        ok?: boolean;
        result?: { project?: Project; envExample?: string };
        detail?: string;
      };
      if (!body.ok || !body.result?.project) {
        setError(body.detail ?? t("import.applyFailed"));
        return;
      }
      markImportNextSteps();
      onAccept(body.result.project);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("import.applyFailed"));
    } finally {
      setBusy(false);
    }
  };

  const missingWorkspacePaths = (() => {
    let n = 0;
    for (const d of diags) {
      const code = d.code ?? "";
      if (code.includes("IMPORT") && code.includes("PATH")) {
        n++;
      }
    }
    return n;
  })();

  const stubPreviewCreateCount = stubPathCreateCount(stubPreview);
  const stubPreviewVisible = stubPathHasAny(stubPreview);

  const showCreateStubs =
    draft &&
    ((draft.summary.resources ?? 0) > 0 ||
      missingWorkspacePaths > 0 ||
      stubPreviewVisible);

  const prepareWorkspaceLabel =
    stubPreviewCreateCount > 0
      ? t("import.prepareWorkspaceCount").replace("{n}", String(stubPreviewCreateCount))
      : t("import.prepareWorkspace");

  const unfilledGaps = countUnfilledParameterGaps(draft?.parameterGaps, paramDefaults);
  const acceptBlockKey = acceptBlockReason({
    hasProject: Boolean(draft?.project),
    services: draft?.summary.services ?? 0,
    proposeOk,
    errorMessage: error,
    errorDiags: countImportErrors(diags),
    missingPathWarnings: missingWorkspacePaths,
    unfilledParameterGaps: unfilledGaps,
  });
  const acceptBlockText = acceptBlockKey ? t(acceptBlockKey as Parameters<typeof t>[0]) : null;

  const foldersChecklistOk = missingWorkspacePaths === 0;
  const reviewChecklistOk = proposeOk && countImportErrors(diags) === 0 && !error;

  const wizardPaths = { showPathsStep: Boolean(showCreateStubs) };
  const goWizardNext = () => {
    const n = nextWizardStep(wizardStep, wizardPaths);
    if (n) setWizardStep(n);
  };
  const goWizardBack = () => {
    const p = prevWizardStep(wizardStep, wizardPaths);
    if (p) setWizardStep(p);
  };

  const refusalsByCat = (() => {
    const map = new Map<string, NonNullable<ImportProposeResult["refusals"]>>();
    for (const r of draft?.refusals ?? []) {
      const list = map.get(r.category) ?? [];
      list.push(r);
      map.set(r.category, list);
    }
    return map;
  })();

  return (
    <div className="import-backdrop import-draft-backdrop" role="presentation" data-testid="import-draft">
      <div
        className="import-panel import-panel-draft"
        role="dialog"
        aria-modal="true"
        aria-labelledby="import-title"
      >
        <header className="import-head">
          <h2 id="import-title">{t("import.title")}</h2>
          <button type="button" onClick={closePanel} data-testid="import-cancel">
            {t("import.close")}
          </button>
        </header>
        <p className="import-hint">{t("import.wizard.intro")}</p>
        <ImportWizardNav
          step={wizardStep}
          showPathsStep={wizardPaths.showPathsStep}
          draftReady={Boolean(draft)}
          onStep={setWizardStep}
        />
        <ImportDraftStatusBar
          inMemory={Boolean(draft)}
          onDisk={draftOnDisk}
          stubsPrepared={stubsPrepared}
          missingBefore={missingPathsBefore}
          missingAfter={missingPathsAfter}
        />

        {wizardStep === "sources" ? (
          <>
        <fieldset className="import-fieldset" data-testid="import-compose-list">
          <legend>{t("import.composeMulti")}</legend>
          <FileDropUpload
            kind="compose"
            testId="import-compose-drop"
            disabled={busy}
            onError={setError}
            onUploaded={(path) => {
              setName((prev) => (prev === "imported" ? defaultProjectNameFromCompose(path) : prev));
              setSelectedCompose((prev) => [...new Set([...prev, path])].sort());
              setComposeExtra((prev) => {
                const lines = prev
                  .split(/[\n,]/)
                  .map((s) => s.trim())
                  .filter(Boolean);
                if (lines.includes(path)) return prev;
                return [...lines, path].join("\n");
              });
              reload();
            }}
          />
          {composeHits.length === 0 ? (
            <p className="muted">{t("import.composeEmpty")}</p>
          ) : (
            <div className="import-check-list">
              {composeHits.map((s) => (
                <label key={s.path} className="import-check">
                  <input
                    type="checkbox"
                    checked={selectedCompose.includes(s.path)}
                    onChange={() => toggle(selectedCompose, s.path, setSelectedCompose)}
                    data-testid={"import-compose-check-" + s.path.replace(/[^\w.-]+/g, "_")}
                  />
                  <span>{s.path}</span>
                </label>
              ))}
            </div>
          )}
          <label>
            {t("import.composeExtra")}
            <textarea
              data-testid="import-compose"
              rows={2}
              value={composeExtra}
              onChange={(e) => setComposeExtra(e.target.value)}
              placeholder="stack9.yml"
            />
          </label>
        </fieldset>

        <fieldset className="import-fieldset" data-testid="import-dockerfile-list">
          <legend>{t("import.dockerfileMulti")}</legend>
          <FileDropUpload
            kind="dockerfile"
            testId="import-dockerfile-drop"
            disabled={busy}
            onError={setError}
            onUploaded={(path) => {
              setSelectedDockerfiles((prev) => [...new Set([...prev, path])].sort());
              setDockerfileExtra((prev) => {
                const parts = prev
                  .split(/[\n,]/)
                  .map((s) => s.trim())
                  .filter(Boolean);
                if (parts.includes(path)) return prev;
                return [...parts, path].join("\n");
              });
              reload();
            }}
          />
          {dockerfileHits.length > 0 ? (
            <div className="import-check-list">
              {dockerfileHits.map((s) => (
                <label key={s.path} className="import-check">
                  <input
                    type="checkbox"
                    checked={selectedDockerfiles.includes(s.path)}
                    onChange={() => toggle(selectedDockerfiles, s.path, setSelectedDockerfiles)}
                  />
                  <span>{s.path}</span>
                </label>
              ))}
            </div>
          ) : null}
          <label>
            {t("import.dockerfile")}
            <input
              data-testid="import-dockerfile"
              value={dockerfileExtra}
              onChange={(e) => setDockerfileExtra(e.target.value)}
              placeholder="app/Dockerfile"
            />
          </label>
        </fieldset>

        <label>
          {t("import.name")}
          <input data-testid="import-name" value={name} onChange={(e) => setName(e.target.value)} />
        </label>

        <label>
          {t("import.profiles")}
          <input
            data-testid="import-profiles"
            value={activeProfilesInput}
            onChange={(e) => setActiveProfilesInput(e.target.value)}
            placeholder="companion-tool-router, debug"
          />
          <span className="muted import-field-hint">{t("import.profilesHint")}</span>
        </label>

        {expertMode ? (
          <label className="import-check">
            <input
              type="checkbox"
              data-testid="import-strict"
              checked={importStrict}
              onChange={(e) => setImportStrict(e.target.checked)}
            />
            <span>{t("import.importStrict")}</span>
          </label>
        ) : null}

        <div className="import-actions">
          <button type="button" onClick={() => reload()} disabled={loading || busy}>
            {t("import.rescan")}
          </button>
          <button
            type="button"
            data-testid="import-propose"
            onClick={() => {
              void propose();
            }}
            disabled={busy || (composePaths.length === 0 && dockerfilePaths.length === 0)}
          >
            {t("import.propose")}
          </button>
        </div>
          </>
        ) : null}

        {error ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}
        {notice ? (
          <p className="import-notice" role="status" data-testid="import-notice">
            {notice}
          </p>
        ) : null}
        {stubReport ? (
          <div className="import-stub-report" data-testid="import-stub-report">
            <ImportStubPathLists groups={stubReport} />
          </div>
        ) : null}

        {draft && wizardStep === "review" ? (
          <div className="import-summary" data-testid="import-summary">
            <p data-testid="import-summary-text">
              {t("import.summaryPrefix")} {draft.summary.services} {t("import.services")},{" "}
              {draft.summary.images} {t("import.images")}, {draft.summary.networks}{" "}
              {t("import.networks")}, {draft.summary.volumes} {t("import.volumes")},{" "}
              {draft.summary.resources} {t("import.resources")}
            </p>
            {importServiceRows(draft.project.spec.services).length > 0 ? (
              <ul className="import-service-rows" data-testid="import-service-rows">
                {importServiceRows(draft.project.spec.services).map((row) => (
                  <li key={row.id}>
                    <code>{row.id}</code>
                    {row.user ? (
                      <>
                        {" "}
                        {t("import.serviceUser")} <code>{row.user}</code>
                      </>
                    ) : null}
                    {row.environmentFrom ? (
                      <>
                        {" "}
                        {t("import.serviceEnvFrom")} <code>{row.environmentFrom}</code>
                      </>
                    ) : null}
                  </li>
                ))}
              </ul>
            ) : null}
            {draft.summary.services === 0 ? (
              <div className="import-zero-services" data-testid="import-zero-services">
                <strong>{t("import.zeroServicesTitle")}</strong>
                <p className="muted">{t("import.zeroServicesHint")}</p>
              </div>
            ) : null}
            {(draft.refusals?.length ?? 0) > 0 ? (
              <p className="import-notice" data-testid="import-review-notes">
                {t("import.reviewNotes")}
              </p>
            ) : null}
            {(draft.summary.resources ?? 0) > 0 ? (
              <p className="import-review-hint muted" data-testid="import-review-folders-hint">
                {t("import.reviewFoldersHint")}
              </p>
            ) : null}
            <ImportDiagnosticsPanel
              diags={diags}
              onPrepare={() => {
                setWizardStep("paths");
                void prepareWorkspace();
              }}
              onGoToPaths={() => setWizardStep("paths")}
            />
          </div>
        ) : null}

        {draft && wizardStep === "paths" ? (
          <div className="import-step-paths" data-testid="import-step-paths">
            <p className="muted">{t("import.wizard.pathsIntro")}</p>
            {missingWorkspacePaths > 0 ? (
              <p className="import-missing-paths" data-testid="import-missing-paths">
                {missingWorkspacePaths} — {t("import.missingPathsHint")}
              </p>
            ) : null}
            {showCreateStubs && stubPreview && stubPreviewVisible && stubPreviewCreateCount === 0 ? (
              <p className="import-stub-all-ok" data-testid="import-stub-all-exist">
                {t("import.stubsAllExist")}
              </p>
            ) : null}
            {showCreateStubs && stubPreview && stubPreviewVisible ? (
              <div className="import-stub-preview" data-testid="import-stub-preview">
                <div className="import-stub-preview-head">
                  <strong>{t("import.stubsPreviewTitle")}</strong>
                  <button
                    type="button"
                    className="import-stub-preview-refresh"
                    data-testid="import-stub-preview-refresh"
                    disabled={busy || stubPreviewBusy || !draft?.project}
                    onClick={() => {
                      if (draft?.project) void refreshStubPreview(draft.project, bindChoices);
                    }}
                  >
                    {stubPreviewBusy ? t("import.stubsPreviewLoading") : t("import.refreshStubPreview")}
                  </button>
                </div>
                {stubPreview.bindItems?.length ? (
                  <ImportBindPathChooser
                    items={stubPreview.bindItems}
                    choices={bindChoices}
                    disabled={busy || stubPreviewBusy}
                    onChange={(key, next) => {
                      const nextChoices = { ...bindChoices, [key]: next };
                      setBindChoices(nextChoices);
                      if (draft?.project) {
                        void refreshStubPreview(draft.project, nextChoices);
                      }
                    }}
                    onUploaded={() => {
                      if (draft?.project) void refreshStubPreview(draft.project, bindChoices);
                    }}
                    onError={setError}
                  />
                ) : null}
                {stubPreviewCreateCount > 0 ? (
                  <ImportStubPathLists groups={stubPreview} testId="import-stub-path-summary" />
                ) : null}
              </div>
            ) : null}
            {showCreateStubs && stubPreviewBusy && !stubPreview ? (
              <p className="muted" data-testid="import-stub-preview-loading">
                {t("import.stubsPreviewLoading")}
              </p>
            ) : null}
            {expertMode && draft?.project ? (
              <div className="import-draft-cli" data-testid="import-draft-cli">
                <label className="import-draft-cli-path">
                  {t("import.draftCliPath")}
                  <input
                    data-testid="import-draft-cli-path"
                    value={draftCliPath}
                    disabled={busy}
                    onChange={(e) => setDraftCliPath(e.target.value.trim() || defaultImportDraftPath)}
                  />
                </label>
                <button
                  type="button"
                  data-testid="import-save-draft-json"
                  disabled={busy}
                  onClick={() => {
                    setBusy(true);
                    void saveDraftJsonForCli().finally(() => setBusy(false));
                  }}
                >
                  {t("import.saveDraftJson")}
                </button>
                <button
                  type="button"
                  data-testid="import-download-draft-json"
                  disabled={busy}
                  onClick={() => downloadDraftJson()}
                >
                  {t("import.downloadDraftJson")}
                </button>
              </div>
            ) : null}
            {expertMode && showCreateStubs && workspaceInfo ? (
              <details className="import-stubs-cli" data-testid="import-stubs-cli">
                <summary>{t("import.stubsCliTitle")}</summary>
                <p className="muted">{t("import.stubsCliHint")}</p>
                <div className="import-stubs-cli-actions">
                  <button
                    type="button"
                    data-testid="import-copy-cli"
                    disabled={busy}
                    onClick={() => {
                      void copyCliRecipe();
                    }}
                  >
                    {cliCopied ? t("import.copyCliDone") : t("import.copyCli")}
                  </button>
                </div>
                <pre>{buildImportStubsCliScript(workspaceInfo, draftCliPath)}</pre>
              </details>
            ) : null}
            {showCreateStubs ? (
              <div className="import-stubs-actions">
                <button
                  type="button"
                  className="import-stubs-btn import-prepare-btn"
                  data-testid="import-prepare-workspace"
                  title={t("import.prepareWorkspaceHint")}
                  disabled={busy}
                  onClick={() => {
                    void prepareWorkspace();
                  }}
                >
                  {prepareWorkspaceLabel}
                </button>
                <button
                  type="button"
                  className="import-stubs-btn"
                  data-testid="import-create-stubs"
                  title={t("import.createStubsHint")}
                  disabled={busy}
                  onClick={() => {
                    void createStubs();
                  }}
                >
                  {t("import.createStubs")}
                </button>
              </div>
            ) : (
              <p className="muted">{t("import.wizard.pathsSkip")}</p>
            )}
          </div>
        ) : null}

        {draft && wizardStep === "onboard" ? (
          <div className="import-step-onboard" data-testid="import-step-onboard">
            <p className="muted">{t("import.wizard.onboardIntro")}</p>
            {(draft.parameterGaps?.length ?? 0) > 0 ? (
              <div className="import-gaps" data-testid="import-parameter-gaps">
                <strong>{t("import.parameterGaps")}</strong>
                <ul>
                  {draft.parameterGaps!.map((g) => (
                    <li key={g.id}>
                      <label className="import-gap-row">
                        <span>
                          {g.id} ({g.phase}/{g.type}) — {g.reason}
                        </span>
                        <input
                          data-testid={"import-param-default-" + g.id}
                          placeholder={t("import.paramDefault")}
                          value={paramDefaults[g.id] ?? ""}
                          onChange={(e) =>
                            setParamDefaults({ ...paramDefaults, [g.id]: e.target.value })
                          }
                        />
                      </label>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            {uniqueSecretSuggestions.length > 0 ? (
              <div className="import-secrets" data-testid="import-secret-suggestions">
                <strong>{t("import.secretSuggestions")}</strong>
                <p className="muted">{t("import.secretHint")}</p>
                <p className="muted">{t("forms.secret.hint")}</p>
                <div className="secret-batch-actions">
                  <button
                    type="button"
                    data-testid="import-secret-detect-all"
                    disabled={secretGenBusy}
                    onClick={() => {
                      for (const s of uniqueSecretSuggestions) detectImportSecret(s);
                    }}
                  >
                    {t("forms.secret.detectAll")}
                  </button>
                  <button
                    type="button"
                    data-testid="import-secret-generate-all"
                    disabled={secretGenBusy}
                    onClick={() => {
                      void generateImportSecrets();
                    }}
                  >
                    {t("forms.secret.generateAll")}
                  </button>
                </div>
                {secretBatchNote ? (
                  <p className="muted" data-testid="import-secret-generate-all-status">
                    {secretBatchNote}
                  </p>
                ) : null}
                {uniqueSecretSuggestions.map((s) => (
                  <div key={s.id} className="import-secret-card">
                    <label className="import-check">
                      <input
                        type="checkbox"
                        checked={selectedSecrets.includes(s.id)}
                        onChange={() => toggle(selectedSecrets, s.id, setSelectedSecrets)}
                        data-testid={"import-secret-" + s.id}
                      />
                      <span>
                        {s.id} [{s.source}]
                        {s.envKey ? ` env:${s.envKey}` : ""}
                        {s.target ? ` → ${s.target}` : ""}
                      </span>
                    </label>
                    <SecretFileTools
                      id={s.id}
                      kind={secretKinds[s.id] ?? classifySecret(suggestionHints(s)).kind}
                      note={secretNotes[s.id]}
                      onKind={(kind) => {
                        setSecretKinds((prev) => ({ ...prev, [s.id]: kind }));
                        setSecretNotes((prev) => ({ ...prev, [s.id]: "" }));
                      }}
                      onDetect={() => detectImportSecret(s)}
                    />
                  </div>
                ))}
              </div>
            ) : null}

            {draft.envExample ? (
              <pre className="import-env-example" data-testid="import-env-example">
                {draft.envExample}
              </pre>
            ) : null}

            <label className="import-check">
              <input
                type="checkbox"
                checked={writeEnvExample}
                onChange={(e) => setWriteEnvExample(e.target.checked)}
                data-testid="import-write-env-example"
              />
              <span>{t("import.writeEnvExample")}</span>
            </label>
          </div>
        ) : null}

        {draft && wizardStep === "accept" ? (
          <div className="import-step-accept" data-testid="import-step-accept">
            <p>{t("import.wizard.acceptIntro")}</p>
            <p className="muted">{t("import.wizard.acceptBeforeSave")}</p>
            <ul className="import-accept-checklist import-accept-prep">
              <li className={foldersChecklistOk ? "done" : "pending"} data-testid="import-check-folders">
                {foldersChecklistOk ? "✓" : "○"} {t("import.wizard.checklist.folders")}
                {!foldersChecklistOk && missingWorkspacePaths > 0
                  ? ` (${String(missingWorkspacePaths)})`
                  : null}
              </li>
              <li className="note" data-testid="import-check-parameters">
                {unfilledGaps === 0
                  ? t("import.wizard.checklist.parametersDone")
                  : t("import.wizard.checklist.parameters").replace("{n}", String(unfilledGaps))}
              </li>
              <li className={reviewChecklistOk ? "done" : "pending"} data-testid="import-check-review">
                {reviewChecklistOk ? "✓" : "○"} {t("import.wizard.checklist.review")}
              </li>
            </ul>
            <ul className="import-accept-checklist">
              <li>{t("import.wizard.acceptMemory")}</li>
              <li>{t("import.wizard.acceptSave")}</li>
            </ul>
            {acceptBlockText ? (
              <p className="import-accept-blocked" role="alert" data-testid="import-accept-blocked">
                {acceptBlockText}
              </p>
            ) : (
              <p className="import-accept-ready" data-testid="import-accept-ready">
                {t("import.wizard.acceptReady")}
              </p>
            )}
            <button
              type="button"
              className="import-accept-btn"
              data-testid="import-accept"
              disabled={Boolean(acceptBlockKey) || busy}
              title={acceptBlockText ?? undefined}
              onClick={() => {
                void accept();
              }}
            >
              {t("import.accept")}
            </button>
          </div>
        ) : null}

        {draft && wizardStep === "review" && (draft.refusals?.length ?? 0) > 0 ? (
          <div className="import-refusals" data-testid="import-refusals">
            <strong>{t("import.refusals")}</strong>
            {REFUSAL_ORDER.map((cat) => {
              const list = refusalsByCat.get(cat);
              if (!list?.length) return null;
              const preview =
                cat === "unsupported" && !showAllUnsupported
                  ? list.slice(0, UNSUPPORTED_REFUSAL_PREVIEW)
                  : list;
              const hidden =
                cat === "unsupported" && !showAllUnsupported
                  ? Math.max(0, list.length - UNSUPPORTED_REFUSAL_PREVIEW)
                  : 0;
              return (
                <div key={cat} data-testid={"import-refusal-" + cat}>
                  <h3>{cat}</h3>
                  <ul>
                    {preview.map((r, i) => (
                      <li key={"r-" + cat + "-" + String(i)}>
                        [{r.code}] {r.path}
                        {r.pointer ? " " + r.pointer : ""} — {r.message}
                      </li>
                    ))}
                  </ul>
                  {hidden > 0 ? (
                    <button
                      type="button"
                      className="import-more-refusals"
                      data-testid="import-refusal-unsupported-more"
                      onClick={() => setShowAllUnsupported(true)}
                    >
                      +{hidden} …
                    </button>
                  ) : null}
                </div>
              );
            })}
          </div>
        ) : null}

        <footer className="import-wizard-footer">
          <button
            type="button"
            data-testid="import-wizard-back"
            disabled={busy || !prevWizardStep(wizardStep, wizardPaths)}
            onClick={goWizardBack}
          >
            {t("import.wizard.back")}
          </button>
          <button
            ref={wizardNextRef}
            type="button"
            data-testid="import-wizard-next"
            disabled={busy || !draft || !nextWizardStep(wizardStep, wizardPaths)}
            onClick={goWizardNext}
          >
            {t("import.wizard.next")}
          </button>
          <label className="import-expert-toggle import-check">
            <input
              type="checkbox"
              checked={expertMode}
              onChange={(e) => setExpertMode(e.target.checked)}
              data-testid="import-expert-mode"
            />
            <span>{t("import.wizard.expertMode")}</span>
          </label>
        </footer>
      </div>
    </div>
  );
}
