import { Document, Scalar, parse as parseYaml, visit } from "yaml";
import type { Project } from "./project";

export type ParseResult =
  | { ok: true; project: Project }
  | { ok: false; message: string };

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

export function parseProjectYaml(text: string): ParseResult {
  let doc: unknown;
  try {
    doc = parseYaml(text);
  } catch (e) {
    return { ok: false, message: e instanceof Error ? e.message : "YAML parse failed" };
  }
  if (!isRecord(doc)) {
    return { ok: false, message: "manifest root must be a mapping" };
  }
  if (doc.apiVersion !== "iacpack.io/v1alpha1" || doc.kind !== "Project") {
    return { ok: false, message: "expected apiVersion iacpack.io/v1alpha1 kind Project" };
  }
  if (!isRecord(doc.metadata) || !isRecord(doc.spec)) {
    return { ok: false, message: "metadata and spec are required" };
  }
  // Trust shape enough for authoring; server validation is authoritative.
  return { ok: true, project: doc as unknown as Project };
}

/** Quote strings the manifest subset rejects as flow collections (`[ ] { }`). */
export function projectToYaml(project: Project): string {
  const doc = new Document(project);
  visit(doc, {
    Scalar(_key, node) {
      if (typeof node.value === "string" && /[{}\[\]]/.test(node.value)) {
        node.type = Scalar.QUOTE_DOUBLE;
      }
    },
  });
  return doc.toString({
    lineWidth: 0,
    defaultKeyType: "PLAIN",
    defaultStringType: "PLAIN",
  });
}
