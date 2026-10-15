import type { Command } from "../../domain/commands";
import {
  imageRefLabel,
  isImageBuild,
  type Image,
  type ImageDevBuild,
  type ImagePull,
  type Project,
} from "../../domain/project";
import { dockerfilesFrom, splitDockerfilePath, useDiscoverSuggestions } from "../../app/useDiscover";
import { useLocale } from "../../i18n/useLocale";
import { EnvMapEditor } from "./EnvMapEditor";
import { GlobChipRow } from "./GlobChipRow";
import { UnmodeledFields } from "./UnmodeledFields";
import { KNOWN_IMAGE_BUILD, KNOWN_IMAGE_PULL, mergeUnmodeled } from "./unmodeled";

type Props = {
  project: Project;
  imageId: string;
  image: Image;
  dispatch: (cmd: Command) => void;
};

const INCLUDE_PRESETS = ["**", "src/**", "app/**"];
const EXCLUDE_PRESETS = [".git/**", ".env", "dist/**", "node_modules/**"];

function commaList(v: string[] | undefined): string {
  return (v ?? []).join(", ");
}

function parseCommaList(text: string): string[] | undefined {
  const parts = text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  return parts.length ? parts : undefined;
}

function patchBuild(img: ImageDevBuild, patch: Partial<ImageDevBuild["build"]>): ImageDevBuild {
  return { build: { ...img.build, ...patch } };
}

export function ImageEditor({ project, imageId, image: img, dispatch }: Props) {
  const { t } = useLocale();
  const { suggestions, loading, reload } = useDiscoverSuggestions();
  const dockerfiles = dockerfilesFrom(suggestions);
  const buildParams = Object.entries(project.spec.parameters ?? {})
    .filter(([, p]) => p.phase === "build")
    .map(([id]) => id);

  const usedByServices = Object.keys(project.spec.services)
    .filter((sid) => project.spec.services[sid]?.image === imageId)
    .sort();
  const usedByTasks = Object.keys(project.spec.tasks ?? {})
    .filter((tid) => project.spec.tasks?.[tid]?.image === imageId)
    .sort();

  const kind: "build" | "pull" = isImageBuild(img) ? "build" : "pull";
  const buildAllowed = project.spec.mode === "development";

  const setKind = (next: "build" | "pull") => {
    if (next === kind) return;
    if (next === "build") {
      if (!buildAllowed) return;
      const first = dockerfiles[0];
      const split = first ? splitDockerfilePath(first.path) : { context: ".", dockerfile: "Dockerfile" };
      const value: ImageDevBuild = {
        build: {
          context: split.context,
          dockerfile: split.dockerfile,
          include: ["**"],
        },
      };
      dispatch({ type: "upsertImage", id: imageId, value });
      return;
    }
    const value: ImagePull = {
      ref: "docker.io/library/busybox@sha256:1111111111111111111111111111111111111111111111111111111111111111",
      delivery: "pull",
    };
    dispatch({ type: "upsertImage", id: imageId, value });
  };

  const applyDockerfilePick = (fullPath: string) => {
    if (!isImageBuild(img)) return;
    const split = splitDockerfilePath(fullPath);
    dispatch({
      type: "upsertImage",
      id: imageId,
      value: patchBuild(img, { context: split.context, dockerfile: split.dockerfile }),
    });
  };

  return (
    <div
      className="grid2 image-editor"
      data-testid={`image-${imageId}`}
      data-field-path={`/spec/images/${imageId}`}
    >
      <label>
        {t("img.kind")}
        <select
          data-testid={`image-kind-${imageId}`}
          value={kind}
          onChange={(e) => setKind(e.target.value as "build" | "pull")}
        >
          <option value="pull">{t("img.kind.pull")}</option>
          <option value="build" disabled={!buildAllowed}>
            {t("img.kind.build")}
          </option>
        </select>
      </label>
      {!buildAllowed ? (
        <p className="span2 muted" role="status">
          {kind === "build" ? t("img.build.releaseConflict") : t("img.build.devOnly")}
        </p>
      ) : null}
      <div className="span2 stack-affinity" data-testid={`image-stack-${imageId}`}>
        <strong>{t("stack.usage")}</strong>
        <p>
          {t("stack.services")}:{" "}
          {usedByServices.length ? usedByServices.join(", ") : t("svc.none")}
        </p>
        <p>
          {t("stack.tasks")}: {usedByTasks.length ? usedByTasks.join(", ") : t("svc.none")}
        </p>
      </div>

      {isImageBuild(img) ? (
        <>
          <label className="span2">
            {t("img.pickDockerfile")}
            <div className="picker-row">
              <select
                data-testid={`image-dockerfile-pick-${imageId}`}
                value=""
                disabled={loading || dockerfiles.length === 0}
                onChange={(e) => {
                  if (e.target.value) applyDockerfilePick(e.target.value);
                }}
              >
                <option value="">
                  {loading
                    ? t("img.discoverLoading")
                    : dockerfiles.length
                      ? t("img.pickDockerfile.placeholder")
                      : t("img.pickDockerfile.empty")}
                </option>
                {dockerfiles.map((d) => (
                  <option key={d.path} value={d.path}>
                    {d.path}
                  </option>
                ))}
              </select>
              <button type="button" className="linkish" onClick={reload}>
                {t("img.discoverReload")}
              </button>
            </div>
            <p className="muted">{t("img.pickDockerfile.hint")}</p>
          </label>
          <label>
            {t("forms.context")}
            <input
              data-testid={`image-context-${imageId}`}
              value={img.build.context}
              list={`image-context-list-${imageId}`}
              onChange={(e) =>
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { context: e.target.value }),
                })
              }
            />
            <datalist id={`image-context-list-${imageId}`}>
              {[...new Set(dockerfiles.map((d) => splitDockerfilePath(d.path).context))].map((c) => (
                <option key={c} value={c} />
              ))}
            </datalist>
          </label>
          <label>
            {t("forms.dockerfile")}
            <input
              data-testid={`image-dockerfile-${imageId}`}
              value={img.build.dockerfile ?? ""}
              list={`image-dockerfile-list-${imageId}`}
              onChange={(e) =>
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { dockerfile: e.target.value || undefined }),
                })
              }
            />
            <datalist id={`image-dockerfile-list-${imageId}`}>
              <option value="Dockerfile" />
              {[...new Set(dockerfiles.map((d) => splitDockerfilePath(d.path).dockerfile))].map(
                (name) => (
                  <option key={name} value={name} />
                ),
              )}
            </datalist>
          </label>
          <label className="span2">
            {t("forms.include")}
            <input
              value={commaList(img.build.include)}
              onChange={(e) =>
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { include: parseCommaList(e.target.value) }),
                })
              }
            />
            <GlobChipRow
              label={t("glob.presets")}
              presets={INCLUDE_PRESETS}
              onPick={(g) => {
                const cur = img.build.include ?? [];
                if (cur.includes(g)) return;
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { include: [...cur, g] }),
                });
              }}
            />
          </label>
          <label className="span2">
            {t("img.exclude")}
            <input
              value={commaList(img.build.exclude)}
              onChange={(e) =>
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { exclude: parseCommaList(e.target.value) }),
                })
              }
            />
            <GlobChipRow
              label={t("glob.presets")}
              presets={EXCLUDE_PRESETS}
              onPick={(g) => {
                const cur = img.build.exclude ?? [];
                if (cur.includes(g)) return;
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { exclude: [...cur, g] }),
                });
              }}
            />
          </label>
          <EnvMapEditor
            label={t("img.args")}
            hint={t("img.args.hint")}
            testId={`image-args-${imageId}`}
            value={img.build.args}
            parameterIds={buildParams}
            onChange={(args) =>
              dispatch({
                type: "upsertImage",
                id: imageId,
                value: patchBuild(img, { args }),
              })
            }
          />
          <label className="span2">
            {t("img.expectedId")}
            <input
              data-testid={`image-expected-${imageId}`}
              value={img.build.expectedImageID ?? ""}
              onChange={(e) =>
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: patchBuild(img, { expectedImageID: e.target.value || undefined }),
                })
              }
            />
          </label>
        </>
      ) : (
        <>
          <label className="span2">
            {t("forms.ref")}
            <input
              data-testid={`image-ref-${imageId}`}
              value={imageRefLabel(img.ref)}
              onChange={(e) => {
                const next = e.target.value;
                const ref =
                  typeof img.ref === "object" && next === img.ref.parameter ? img.ref : next;
                dispatch({
                  type: "upsertImage",
                  id: imageId,
                  value: { ...img, ref },
                });
              }}
            />
          </label>
          <label>
            {t("forms.delivery")}
            <input value="pull" readOnly disabled data-testid={`image-delivery-${imageId}`} />
            <span className="muted">{t("img.delivery.pullOnly")}</span>
          </label>
        </>
      )}
      <UnmodeledFields
        testId={`image-unmodeled-${imageId}`}
        value={img as Record<string, unknown>}
        known={isImageBuild(img) ? KNOWN_IMAGE_BUILD : KNOWN_IMAGE_PULL}
        onApply={(extra) =>
          dispatch({
            type: "upsertImage",
            id: imageId,
            value: mergeUnmodeled(
              img as Record<string, unknown>,
              isImageBuild(img) ? KNOWN_IMAGE_BUILD : KNOWN_IMAGE_PULL,
              extra,
            ) as Image,
          })
        }
      />
    </div>
  );
}
