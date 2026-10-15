import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { AppState } from "../app/types";
import { emptyHistory } from "../app/editHistory";
import { emptyProject } from "../domain/project";
import { LocaleProvider } from "../i18n/LocaleProvider";
import { SyncBanner } from "./SyncBanner";

function baseState(over: Partial<AppState> = {}): AppState {
  return {
    phase: "loaded",
    project: emptyProject(),
    lastValid: emptyProject(),
    etag: "1",
    dirty: false,
    validating: false,
    valid: null,
    diagnostics: [],
    preview: { status: "idle" },
    build: { status: "idle" },
    save: { status: "idle" },
    sync: { status: "idle" },
    allowDiskOverwrite: false,
    statusMessage: "",
    serverGraph: { status: "idle" },
    graphAnnounce: "",
    history: emptyHistory(),
    diagnosticsAttention: 0,
    ...over,
  };
}

const noop = vi.fn();

describe("SyncBanner focus", () => {
  it("moves focus into recovery actions when conflict appears and restores on dismiss", async () => {
    function Harness() {
      const [state, setState] = useState(baseState({ dirty: true }));
      return (
        <LocaleProvider>
          <input data-testid="prior-focus" defaultValue="x" />
          <button
            type="button"
            data-testid="trigger-conflict"
            onClick={() =>
              setState(
                baseState({
                  dirty: true,
                  sync: { status: "conflict", revision: "r1", reason: "external-while-dirty" },
                }),
              )
            }
          >
            conflict
          </button>
          <button
            type="button"
            data-testid="clear-conflict"
            onClick={() => setState(baseState({ dirty: false }))}
          >
            clear
          </button>
          <SyncBanner
            state={state}
            onReload={noop}
            onDiscardLocal={noop}
            onExportLocal={noop}
            onArmOverwrite={noop}
            onRecoverOverwrite={noop}
          />
        </LocaleProvider>
      );
    }

    render(<Harness />);
    const prior = screen.getByTestId("prior-focus");
    prior.focus();
    expect(prior).toHaveFocus();

    await act(async () => {
      fireEvent.click(screen.getByTestId("trigger-conflict"));
      await new Promise((r) => setTimeout(r, 0));
    });
    await waitFor(() => {
      expect(screen.getByTestId("sync-discard")).toHaveFocus();
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("clear-conflict"));
      await new Promise((r) => setTimeout(r, 0));
    });
    await waitFor(() => {
      expect(screen.queryByTestId("sync-banner")).toBeNull();
      expect(prior).toHaveFocus();
    });
  });
});
