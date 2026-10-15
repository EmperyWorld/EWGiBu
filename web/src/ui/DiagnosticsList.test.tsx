import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n/LocaleProvider";
import { DiagnosticsList } from "./DiagnosticsList";

describe("DiagnosticsList", () => {
  it("links field and hint for screen readers", () => {
    render(
      <LocaleProvider>
        <DiagnosticsList
          diagnostics={[
            {
              severity: "error",
              code: "MANIFEST.INVALID",
              message: "bad port",
              field: "/spec/services/web/ports/0",
              hint: "use 1..65535",
            },
          ]}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText("bad port")).toBeInTheDocument();
    expect(screen.getByText("/spec/services/web/ports/0")).toBeInTheDocument();
    expect(screen.getByText(/Hint: use 1..65535/)).toBeInTheDocument();
  });

  it("invokes onFieldNavigate when field link is clicked", () => {
    const onFieldNavigate = vi.fn();
    render(
      <LocaleProvider>
        <DiagnosticsList
          onFieldNavigate={onFieldNavigate}
          diagnostics={[
            {
              severity: "error",
              code: "MANIFEST.INVALID",
              message: "bad port",
              field: "/spec/services/web/ports/0",
            },
          ]}
        />
      </LocaleProvider>,
    );
    fireEvent.click(screen.getByTestId("diag-field-0"));
    expect(onFieldNavigate).toHaveBeenCalledWith("/spec/services/web/ports/0");
  });

  it("reads source.pointer when field/path are absent", () => {
    const onFieldNavigate = vi.fn();
    render(
      <LocaleProvider>
        <DiagnosticsList
          onFieldNavigate={onFieldNavigate}
          diagnostics={[
            {
              severity: "error",
              code: "SCHEMA.INVALID_VALUE",
              message: "name must be a non-empty string",
              source: { pointer: "/metadata/name", line: 4, column: 9 },
            },
          ]}
        />
      </LocaleProvider>,
    );
    fireEvent.click(screen.getByTestId("diag-field-0"));
    expect(onFieldNavigate).toHaveBeenCalledWith("/metadata/name");
  });
});
