import { handleRequest } from "./handler.js";
import { compileTable, type Manifest, type Table } from "./resolve.js";

interface Env {
  ASSETS: Fetcher;
  MANIFEST: KVNamespace;
}

const TTL_MS = 30_000;
let cache: { manifest: Manifest; table: Table; loadedAt: number } | null = null;

async function loadManifest(env: Env): Promise<typeof cache> {
  if (cache && Date.now() - cache.loadedAt < TTL_MS) return cache;
  try {
    const manifest = await env.MANIFEST.get<Manifest>("manifest", { type: "json", cacheTtl: 30 });
    if (manifest && manifest.version === 1) {
      cache = { manifest, table: compileTable(manifest), loadedAt: Date.now() };
    }
  } catch (e) {
    console.error("manifest reload failed", e);
    if (cache) cache.loadedAt = Date.now(); // keep serving the previous manifest
  }
  return cache;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const loaded = await loadManifest(env);
    if (!loaded) {
      return new Response("manifest not loaded", {
        status: 503,
        headers: { "Content-Type": "text/plain; charset=utf-8" },
      });
    }
    return handleRequest(request, loaded.manifest, loaded.table, env.ASSETS);
  },
} satisfies ExportedHandler<Env>;
