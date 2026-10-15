/** Typed authoring model for schema-known Project fields (v1alpha1). */

export type Mode = "development" | "release";

export type ValueSource =
  | { literal: string | number | boolean }
  | { parameter: string }
  | { secret: string };

export type Parameter = {
  phase: "build" | "deploy";
  type: "string" | "integer" | "number" | "boolean" | "enum";
  default?: string | number | boolean;
  minimum?: number;
  maximum?: number;
  pattern?: string;
  choices?: Array<string | number | boolean>;
  description?: string;
};

export type SecretDecl = {
  required?: boolean;
  delivery?: "file";
  description?: string;
};

export type ImageBuild = {
  context: string;
  dockerfile?: string;
  include?: string[];
  exclude?: string[];
  args?: Record<string, ValueSource>;
  expectedImageID?: string;
};

export type ImageRef = string | { parameter: string };

export function imageRefLabel(ref: ImageRef): string {
  return typeof ref === "string" ? ref : ref.parameter;
}

export type ImagePull = {
  kind?: "pull";
  ref: ImageRef;
  delivery: "pull";
};

export type ImageDevBuild = {
  kind?: "build";
  build: ImageBuild;
};

export type Image = ImagePull | ImageDevBuild;

export function isImageBuild(img: Image): img is ImageDevBuild {
  return "build" in img && img.build !== undefined;
}

export type Resource = {
  type?: string;
  source?: string;
  include?: string[];
  exclude?: string[];
  destination?: string;
  mode?: string;
  allowEmpty?: boolean;
  [key: string]: unknown;
};

export type Network = {
  external?: { name: string };
  [key: string]: unknown;
};

export type Volume = {
  retention?: "retain" | "delete";
  external?: { name: string };
  [key: string]: unknown;
};

export type Port = {
  container: number;
  host?: number | ValueSource;
  bind?: string;
  protocol?: "tcp" | "udp";
};

export type Mount = {
  volume?: string;
  resource?: string;
  source?: string;
  host?: string | { parameter: string };
  target: string;
  readOnly?: boolean;
};

export type ArgvPart =
  | string
  | number
  | boolean
  | { parameter?: string; secret?: string; literal?: string; parts?: ArgvPart[] };

export type ArgvElement = ArgvPart;

export function argvText(command: readonly ArgvElement[] | undefined): string {
  return (command ?? []).map(argvElementText).join(" ");
}

function argvElementText(el: ArgvElement): string {
  if (typeof el === "string") return el;
  if (typeof el === "number" || typeof el === "boolean") return String(el);
  if (!el || typeof el !== "object") return "";
  if (Array.isArray(el.parts)) return el.parts.map(argvElementText).join("");
  if (typeof el.parameter === "string") return "${" + el.parameter + "}";
  if (typeof el.secret === "string") return "${secret:" + el.secret + "}";
  if ("literal" in el) return el.literal == null ? "" : String(el.literal);
  return "";
}

export type DependsOn = {
  target: string;
  condition?: "started" | "healthy" | "completedSuccessfully";
};

export type Healthcheck = {
  command: string[];
  interval?: string;
  timeout?: string;
  startPeriod?: string;
  retries?: number;
};

export type Service = {
  image: string;
  user?: string;
  command?: ArgvElement[];
  environmentFrom?: string;
  entrypoint?: ArgvElement[];
  environment?: Record<string, ValueSource>;
  secrets?: Array<{ ref: string; target: string }>;
  ports?: Port[];
  mounts?: Mount[];
  networks?: string[];
  dependsOn?: DependsOn[];
  healthcheck?: Healthcheck;
  restart?: "no" | "always" | "on-failure" | "unless-stopped";
  resources?: { memory?: number; cpus?: string; gpu?: number | "all" | "devices-only"; ldLibraryPath?: string };
  profiles?: string[];
  [key: string]: unknown;
};

export type Database = {
  on: string;
  name: string;
  ownerFrom: string;
  migrations: string;
  urlParameter: string;
};

export type Task = {
  phase: "preStart" | "migration" | "postDeploy";
  image: string;
  user?: string;
  command?: ArgvElement[];
  entrypoint?: ArgvElement[];
  workdir?: string;
  timeout?: string;
  retry?: {
    count?: number;
    initialBackoff?: string;
    multiplier?: number;
    maxBackoff?: string;
  };
  rerun?: "always" | "oncePerDeployment" | "manual";
  mounts?: Mount[];
  networks?: string[];
  secrets?: Array<{ ref: string; target: string }>;
  dependsOn?: DependsOn[];
  [key: string]: unknown;
};

export type PackageOpts = {
  generateReadme?: boolean;
  generateEnvExample?: boolean;
  includeRuntime?: boolean;
  embedImages?: boolean;
  [key: string]: unknown;
};

export type XUI = {
  nodes?: Record<string, { x: number; y: number; category?: string }>;
  viewport?: { x: number; y: number; zoom: number };
};

export type Project = {
  apiVersion: "iacpack.io/v1alpha1";
  kind: "Project";
  metadata: {
    name: string;
    version: string;
    description?: string;
  };
  spec: {
    target: { profile: "compose-docker/v1" };
    mode: Mode;
    parameters?: Record<string, Parameter>;
    secrets?: Record<string, SecretDecl>;
    images: Record<string, Image>;
    resources?: Record<string, Resource>;
    networks?: Record<string, Network>;
    volumes?: Record<string, Volume>;
    services: Record<string, Service>;
    tasks?: Record<string, Task>;
    databases?: Record<string, Database>;
    package?: PackageOpts;
    "x-ui"?: XUI;
  };
};

export function emptyProject(): Project {
  return {
    apiVersion: "iacpack.io/v1alpha1",
    kind: "Project",
    metadata: { name: "app", version: "0.1.0" },
    spec: {
      target: { profile: "compose-docker/v1" },
      mode: "development",
      images: {
        web: {
          ref: "docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001",
          delivery: "pull",
        },
        api: {
          ref: "docker.io/library/busybox@sha256:1111111111111111111111111111111111111111111111111111111111111111",
          delivery: "pull",
        },
      },
      networks: { frontend: {} },
      services: {
        web: {
          image: "web",
          ports: [{ container: 80, host: 8080, bind: "127.0.0.1", protocol: "tcp" }],
          networks: ["frontend"],
          dependsOn: [{ target: "api", condition: "started" }],
        },
        api: {
          image: "api",
          networks: ["frontend"],
        },
      },
      package: { generateReadme: true, generateEnvExample: true },
      "x-ui": {
        nodes: {
          web: { x: 240, y: 80, category: "proxy" },
          api: { x: 40, y: 80, category: "app" },
        },
        viewport: { x: 0, y: 0, zoom: 1 },
      },
    },
  };
}
