import type { ImportDiagnosticRow } from "./ImportDiagnosticsPanel";

export type ImportWizardStep = "sources" | "review" | "paths" | "onboard" | "accept";

export const IMPORT_WIZARD_STEPS: ImportWizardStep[] = [
  "sources",
  "review",
  "paths",
  "onboard",
  "accept",
];

/** Comma/newline-separated Compose profile names (COMPOSE_PROFILES / --profile). */
export function parseActiveProfilesInput(raw: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of raw.split(/[\n,]/)) {
    const p = part.trim();
    if (!p || seen.has(p)) continue;
    seen.add(p);
    out.push(p);
  }
  return out.sort();
}

export function countImportErrors(diags: ImportDiagnosticRow[]): number {
  return diags.filter((d) => (d.severity ?? "").toLowerCase() === "error").length;
}

export function countImportPathWarnings(diags: ImportDiagnosticRow[]): number {
  return diags.filter((d) => {
    const code = d.code ?? "";
    return code.includes("IMPORT") && code.includes("PATH");
  }).length;
}

export function acceptBlockReason(opts: {
  hasProject: boolean;
  services: number;
  proposeOk: boolean;
  errorMessage: string | null;
  errorDiags: number;
  missingPathWarnings: number;
  unfilledParameterGaps: number;
}): string | null {
  if (!opts.hasProject) return "import.acceptBlock.noDraft";
  if (opts.services === 0) return "import.acceptBlock.zeroServices";
  if (opts.errorMessage) return "import.acceptBlock.error";
  if (opts.errorDiags > 0) return "import.acceptBlock.diagnostics";
  if (!opts.proposeOk) return "import.acceptBlock.partial";
  if (opts.missingPathWarnings > 0) return "import.acceptBlock.missingPaths";
  return null;
}

export function countUnfilledParameterGaps(
  gaps: { id: string }[] | undefined,
  defaults: Record<string, string>,
): number {
  if (!gaps?.length) return 0;
  let n = 0;
  for (const g of gaps) {
    if (!defaults[g.id]?.trim()) n++;
  }
  return n;
}

export function nextWizardStep(
  current: ImportWizardStep,
  opts: { showPathsStep: boolean },
): ImportWizardStep | null {
  const order = opts.showPathsStep
    ? IMPORT_WIZARD_STEPS
    : IMPORT_WIZARD_STEPS.filter((s) => s !== "paths");
  const i = order.indexOf(current);
  if (i < 0 || i >= order.length - 1) return null;
  return order[i + 1] ?? null;
}

export function prevWizardStep(
  current: ImportWizardStep,
  opts: { showPathsStep: boolean },
): ImportWizardStep | null {
  const order = opts.showPathsStep
    ? IMPORT_WIZARD_STEPS
    : IMPORT_WIZARD_STEPS.filter((s) => s !== "paths");
  const i = order.indexOf(current);
  if (i <= 0) return null;
  return order[i - 1] ?? null;
}

export function diagWantsPrepare(d: ImportDiagnosticRow): boolean {
  const code = d.code ?? "";
  return code.includes("IMPORT") && code.includes("PATH");
}

export function diagIsRebaseHint(d: ImportDiagnosticRow): boolean {
  return (d.message ?? "").includes("rebased to workspace");
}
