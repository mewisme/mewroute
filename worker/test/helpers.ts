import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { build } from "../build/build.js";
import { handleRequest, type AssetFetcher } from "../src/handler.js";
import { compileTable, resolve, type Manifest } from "../src/resolve.js";

export async function tmp(): Promise<string> {
  return fs.mkdtemp(path.join(os.tmpdir(), "mewroute-"));
}

export async function writeTree(root: string, files: Record<string, string>): Promise<void> {
  for (const [rel, content] of Object.entries(files)) {
    const abs = path.join(root, rel);
    await fs.mkdir(path.dirname(abs), { recursive: true });
    await fs.writeFile(abs, content);
  }
}

export async function load(dataDir: string) {
  const out = await tmp();
  const m: Manifest = await build(dataDir, out);
  const table = compileTable(m);
  const assets: AssetFetcher = {
    async fetch(req) {
      const rel = decodeURIComponent(new URL(req.url).pathname);
      try {
        const body = await fs.readFile(path.join(out, "public", rel));
        return new Response(req.method === "HEAD" ? null : body, {
          headers: { "Content-Type": "application/x-from-assets", "Cache-Control": "public, max-age=0" },
        });
      } catch {
        return new Response("nope", { status: 404 });
      }
    },
  };
  return {
    m,
    out,
    r: (p: string) => resolve(table, m, p),
    req: (p: string, init?: RequestInit) =>
      handleRequest(new Request("http://x" + p, init), m, table, assets),
  };
}

export async function fixture(files: Record<string, string>) {
  const dir = await tmp();
  await writeTree(dir, files);
  return load(dir);
}
