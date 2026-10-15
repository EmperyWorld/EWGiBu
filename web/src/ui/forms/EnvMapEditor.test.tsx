import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n/LocaleProvider";
import { EnvMapEditor } from "./EnvMapEditor";

describe("EnvMapEditor", () => {
  it("keeps an empty draft row after Add until a key is typed", () => {
    const onChange = vi.fn();
    render(
      <LocaleProvider>
        <EnvMapEditor
          label="Environment"
          hint="hint"
          testId="env"
          value={undefined}
          parameterIds={["db-host"]}
          onChange={onChange}
        />
      </LocaleProvider>,
    );

    fireEvent.click(screen.getByTestId("env-add"));
    expect(screen.getByTestId("env-row-0")).toBeInTheDocument();
    expect(onChange).toHaveBeenLastCalledWith(undefined);

    const keyInput = screen.getByTestId("env-row-0").querySelector("input");
    expect(keyInput).toBeTruthy();
    fireEvent.change(keyInput!, { target: { value: "LOG_LEVEL" } });
    expect(onChange).toHaveBeenLastCalledWith({ LOG_LEVEL: { literal: "" } });
  });
});
