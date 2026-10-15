import { IacpackClient } from "../../api/generated/client";
import { readCsrf } from "../../app/csrf";
import type { SecretKind } from "./secretKind";

export type SecretWriteItem = { id: string; kind: SecretKind };

export type SecretWriteResult = {
  wrote: string[];
  existed: string[];
  failed: string[];
};

export async function writeSecretFiles(items: SecretWriteItem[], replace: boolean): Promise<SecretWriteResult> {
  const client = new IacpackClient({ baseUrl: "", csrf: readCsrf() });
  const wrote: string[] = [];
  const existed: string[] = [];
  const failed: string[] = [];
  for (const item of items) {
    const res = await client.generateSecretFile({ id: item.id, kind: item.kind, replace });
    if (res.status === 409 && !replace) {
      existed.push(item.id);
      continue;
    }
    if (!res.ok) {
      failed.push(item.id);
      continue;
    }
    const body = (await res.json().catch(() => null)) as { result?: { path?: unknown } } | null;
    if (body?.result?.path !== `secrets/${item.id}`) {
      failed.push(item.id);
      continue;
    }
    wrote.push(item.id);
  }
  return { wrote, existed, failed };
}

export async function generateAllSecrets(
  items: SecretWriteItem[],
  askReplace: (count: number) => boolean,
): Promise<{ wrote: number; kept: number; failed: number }> {
  const first = await writeSecretFiles(items, false);
  let wrote = first.wrote.length;
  let kept = first.existed.length;
  let failed = first.failed.length;
  if (first.existed.length > 0 && askReplace(first.existed.length)) {
    const again = await writeSecretFiles(
      items.filter((item) => first.existed.includes(item.id)),
      true,
    );
    wrote += again.wrote.length;
    kept = again.existed.length;
    failed += again.failed.length;
  }
  return { wrote, kept, failed };
}
