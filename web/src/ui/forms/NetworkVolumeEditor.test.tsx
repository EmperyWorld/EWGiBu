import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { emptyProject } from "../../domain/project";
import { LocaleProvider } from "../../i18n/LocaleProvider";
import { NetworkEditor } from "./NetworkEditor";
import { VolumeEditor } from "./VolumeEditor";

describe("NetworkEditor", () => {
  it("marks network external and records external name", () => {
    const dispatch = vi.fn();
    const project = emptyProject();
    render(
      <LocaleProvider>
        <NetworkEditor
          project={project}
          networkId="frontend"
          network={project.spec.networks?.frontend ?? {}}
          dispatch={dispatch}
        />
      </LocaleProvider>,
    );
    expect(screen.getByTestId("network-stack-frontend")).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("network-external-frontend"), { target: { value: "yes" } });
    expect(dispatch).toHaveBeenCalledWith({
      type: "upsertNetwork",
      id: "frontend",
      value: { external: { name: "frontend" } },
    });
  });
});

describe("VolumeEditor", () => {
  it("updates retention for managed volumes", () => {
    const dispatch = vi.fn();
    const project = emptyProject();
    project.spec.volumes = { data: { retention: "retain" } };
    render(
      <LocaleProvider>
        <VolumeEditor
          project={project}
          volumeId="data"
          volume={project.spec.volumes.data!}
          dispatch={dispatch}
        />
      </LocaleProvider>,
    );
    fireEvent.change(screen.getByTestId("volume-retention-data"), { target: { value: "delete" } });
    expect(dispatch).toHaveBeenCalledWith({
      type: "upsertVolume",
      id: "data",
      value: { retention: "delete" },
    });
  });
});
