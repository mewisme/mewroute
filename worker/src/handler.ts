import { listingPage } from "./html.js";
import { contentType } from "./mime.js";
import { resolve, type Decision, type Manifest, type Table } from "./resolve.js";

const DEFAULT_CACHE = "no-cache, must-revalidate";
const TEXT = "text/plain; charset=utf-8";
const HTML = "text/html; charset=utf-8";

export interface AssetFetcher {
  fetch(request: Request): Promise<Response>;
}

function text(body: string, status: number, extra: Record<string, string> = {}, head = false): Response {
  return new Response(head ? null : body, { status, headers: { "Content-Type": TEXT, ...extra } });
}

function applyScope(h: Headers, scope: Record<string, string>): Set<string> {
  const keys = new Set<string>();
  for (const [k, v] of Object.entries(scope)) {
    h.set(k, v);
    keys.add(k.toLowerCase());
  }
  return keys;
}

function encodePath(p: string): string {
  return p.split("/").map(encodeURIComponent).join("/");
}

function page(body: string, scope: Record<string, string>, head: boolean): Response {
  const h = new Headers();
  const keys = applyScope(h, scope);
  if (!keys.has("content-type")) h.set("Content-Type", HTML);
  if (!keys.has("cache-control")) h.set("Cache-Control", DEFAULT_CACHE);
  return new Response(head ? null : body, { status: 200, headers: h });
}

async function serveFile(
  d: Extract<Decision, { kind: "file" }>,
  request: Request,
  assets: AssetFetcher,
): Promise<Response> {
  const url = new URL(encodePath(d.path), request.url);
  const asset = await assets.fetch(new Request(url, { method: request.method, headers: request.headers }));
  if (asset.status === 404) return text("404 page not found\n", 404, {}, request.method === "HEAD");
  const h = new Headers(asset.headers);
  const keys = applyScope(h, d.headers);
  if (!keys.has("content-type")) h.set("Content-Type", contentType(d.path));
  if (!keys.has("cache-control")) h.set("Cache-Control", DEFAULT_CACHE);
  if (d.download) {
    h.set("Content-Disposition", `attachment; filename="${d.path.slice(d.path.lastIndexOf("/") + 1)}"`);
  }
  return new Response(asset.body, { status: asset.status, statusText: asset.statusText, headers: h });
}

export async function handleRequest(
  request: Request,
  m: Manifest,
  table: Table,
  assets: AssetFetcher,
): Promise<Response> {
  const method = request.method;
  const head = method === "HEAD";
  const url = new URL(request.url);

  if (url.pathname === "/healthz") {
    if (method !== "GET" && !head) return text("Method Not Allowed\n", 405, { Allow: "GET, HEAD" });
    return text("ok", 200, {}, head);
  }
  if (method !== "GET" && !head) return text("Method Not Allowed\n", 405, { Allow: "GET, HEAD" });

  let path: string;
  try {
    path = decodeURIComponent(url.pathname);
  } catch {
    return text("404 page not found\n", 404, {}, head);
  }
  if (path.includes("\0")) return text("404 page not found\n", 404, {}, head);

  const d = resolve(table, m, path);
  switch (d.kind) {
    case "redirect": {
      const h = new Headers();
      applyScope(h, d.headers);
      h.set("Location", d.location);
      return new Response(null, { status: d.status, headers: h });
    }
    case "file":
      return serveFile(d, request, assets);
    case "readme":
      return page(m.readmes[d.dir], d.headers, head);
    case "listing":
      return page(listingPage(m, d.dir), d.headers, head);
    default:
      return text("404 page not found\n", 404, {}, head);
  }
}
