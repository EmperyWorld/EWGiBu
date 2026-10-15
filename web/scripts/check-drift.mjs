import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const openapi = readFileSync(join(root, "openapi", "v1", "openapi.yaml"), "utf8");
const client = readFileSync(join(root, "web", "src", "api", "generated", "client.ts"), "utf8");

const pathRe = /^\s{2}(\/api\/v1\/[^\s:]+):$/gm;
const paths = new Set();
let m;
while ((m = pathRe.exec(openapi)) !== null) {
  paths.add(m[1]);
}
if (paths.size === 0) {
  console.error("no OpenAPI paths found");
  process.exit(1);
}
let failed = false;
for (const p of paths) {
  if (!client.includes(p)) {
    console.error(`client missing path ${p}`);
    failed = true;
  }
}
const ops = [
  "getHealth",
  "getWorkspaceInfo",
  "listProjects",
  "createProject",
  "selectProject",
  "getManifest",
  "putManifest",
  "validateManifest",
  "inspectGraph",
  "previewCompose",
  "previewArtifacts",
  "lockManifest",
  "startBuild",
  "getBuild",
  "cancelBuild",
  "listInputs",
  "exportManifest",
  "saveAsManifest",
  "workspaceUpload",
  "listWorkspaceFiles",
  "discoverWorkspace",
  "importStubPreview",
  "importPrepare",
  "importWriteDraft",
  "importResourceStubs",
];
for (const op of ops) {
  if (!client.includes(op)) {
    console.error(`client missing operation ${op}`);
    failed = true;
  }
}
if (failed) process.exit(1);
console.log(`drift ok (${paths.size} paths)`);
