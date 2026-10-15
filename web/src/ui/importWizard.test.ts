import { describe, expect, it } from "vitest";
import {
  acceptBlockReason,
  countImportPathWarnings,
  countUnfilledParameterGaps,
  nextWizardStep,
  parseActiveProfilesInput,
} from "./importWizard";

describe("importWizard", () => {
  it("blocks accept when no services", () => {
    const key = acceptBlockReason({
      hasProject: true,
      services: 0,
      proposeOk: true,
      errorMessage: null,
      errorDiags: 0,
      missingPathWarnings: 0,
      unfilledParameterGaps: 0,
    });
    expect(key).toBe("import.acceptBlock.zeroServices");
  });

  it("allows accept when parameter defaults are still empty", () => {
    expect(countUnfilledParameterGaps([{ id: "postgres-db" }], {})).toBe(1);
    expect(
      acceptBlockReason({
        hasProject: true,
        services: 2,
        proposeOk: true,
        errorMessage: null,
        errorDiags: 0,
        missingPathWarnings: 0,
        unfilledParameterGaps: 1,
      }),
    ).toBeNull();
  });

  it("skips paths step in navigation when hidden", () => {
    expect(nextWizardStep("review", { showPathsStep: false })).toBe("onboard");
  });

  it("parses active profiles", () => {
    expect(parseActiveProfilesInput("a, b\na")).toEqual(["a", "b"]);
  });

  it("counts path warnings", () => {
    expect(
      countImportPathWarnings([{ code: "IMPORT.PATH_INVALID", severity: "warning", message: "x" }]),
    ).toBe(1);
  });
});
