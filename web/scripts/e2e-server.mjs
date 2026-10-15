import { spawn } from "node:child_process";
import { cpSync, existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const bin = join(root, "bin", "iacpack");
const fixture = join(root, "testdata", "ui", "two-service");
const ws = mkdtempSync(join(tmpdir(), "iacpack-e2e-"));
const marker = join(root, "web", ".e2e-workspace");

if (!existsSync(bin)) {
  console.error("build iacpack first: make build");
  process.exit(1);
}

cpSync(fixture, ws, { recursive: true });
writeFileSync(marker, ws, "utf8");

const child = spawn(bin, ["serve", "--workspace", ws, "--addr", "127.0.0.1:7432"], {
  stdio: "inherit",
});

const cleanup = () => {
  child.kill("SIGTERM");
  rmSync(ws, { recursive: true, force: true });
  try {
    rmSync(marker, { force: true });
  } catch {
    /* ignore */
  }
};
process.on("exit", cleanup);
process.on("SIGINT", () => {
  cleanup();
  process.exit(130);
});
