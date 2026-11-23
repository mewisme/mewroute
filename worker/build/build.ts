import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Marked } from "marked";
import { gfmHeadingId } from "marked-gfm-heading-id";
import { parse } from "yaml";
import { readmePage } from "../src/html.js";
import {
  compileParamPattern,
  joinURLPrefix,
  parseGlobalConfig,
  resolveTargetPath,
  validateRoute,
  type Manifest,
  type RouteDef,
  type Scope,
} from "../src/resolve.js";

const MAX_ASSET_BYTES = 25 * 1024 * 1024;
const MAX_ASSETS = 20000;

const workerDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function asRecord(v: unknown, where: string): Record<string, unknown> {
  if (v == null) return {};
  if (typeof v !== "object" || Array.isArray(v)) throw new Error(`${where}: expected a mapping`);
  return v as Record<string, unknown>;
}

function parseRoute(raw: unknown, where: string): RouteDef {
  const r = asRecord(raw, where);
  const str = (k: string): string => {
    const v = r[k];
    if (v == null) return "";
    if (typeof v !== "string") throw new Error(`${where}: ${k} must be a string`);
    return v;
  };
  const route: RouteDef = { from: str("from"), to: str("to"), type: str("type") as RouteDef["type"] };
  if (r.status != null) {
    if (!Number.isInteger(r.status)) throw new Error(`${where}: status must be an integer`);
    route.status = r.status as number;
  }
  if (r.download != null) {
    if (typeof r.download !== "boolean") throw new Error(`${where}: download must be a boolean`);
    route.download = r.download;
  }
  if (r.params != null) {
    route.params = {};
    for (const [name, rule] of Object.entries(asRecord(r.params, `${where}: params`))) {
      const allow = asRecord(rule, `${where}: params.${name}`).allow;
      if (allow != null && (!Array.isArray(allow) || allow.some((a) => typeof a !== "string"))) {
        throw new Error(`${where}: params.${name}.allow must be a list of strings`);
      }
      route.params[name] = allow == null ? {} : { allow: allow as string[] };
    }
  }
  return route;
}

interface Walked {
  files: Manifest["files"];
  dirs: Manifest["dirs"];
  diskPaths: Record<string, string>; // root-relative url path -> absolute disk path
  dirDisk: Record<string, string>;
}

async function walk(dataDir: string): Promise<Walked> {
  const w: Walked = { files: {}, dirs: {}, diskPaths: {}, dirDisk: {} };
  async function visit(abs: string, urlDir: string): Promise<void> {
    const st = await fs.stat(abs);
    const names: { name: string; isDir: boolean }[] = [];
    for (const e of await fs.readdir(abs, { withFileTypes: true })) {
      if (e.name.startsWith(".") || e.name === "node_modules" || e.isSymbolicLink()) continue;
      const child = path.join(abs, e.name);
      const urlPath = (urlDir === "/" ? "" : urlDir) + "/" + e.name;
      if (e.isDirectory()) {
        names.push({ name: e.name, isDir: true });
        await visit(child, urlPath);
      } else if (e.isFile()) {
        const fst = await fs.lstat(child);
        if (fst.size > MAX_ASSET_BYTES) throw new Error(`asset too large: ${urlPath}`);
        names.push({ name: e.name, isDir: false });
        w.files[urlPath] = { size: fst.size, mtime: Math.floor(fst.mtimeMs) };
        w.diskPaths[urlPath] = child;
      }
    }
    names.sort((a, b) =>
      a.isDir !== b.isDir
        ? a.isDir
          ? -1
          : 1
        : a.name.toLowerCase() < b.name.toLowerCase()
          ? -1
          : a.name.toLowerCase() > b.name.toLowerCase()
            ? 1
            : 0,
    );
    w.dirs[urlDir] = {
      mtime: Math.floor(st.mtimeMs),
      entries: names.map((n) => (n.isDir ? n.name + "/" : n.name)),
    };
    w.dirDisk[urlDir] = abs;
  }
  await visit(dataDir, "/");
  const n = Object.keys(w.files).length;
  if (n > MAX_ASSETS) throw new Error(`too many assets: ${n}`);
  return w;
}

async function readYaml(file: string): Promise<unknown | undefined> {
  let text: string;
  try {
    text = await fs.readFile(file, "utf8");
  } catch (e) {
    if ((e as NodeJS.ErrnoException).code === "ENOENT") return undefined;
    throw e;
  }
  try {
    return parse(text);
  } catch (e) {
    throw new Error(`${file}: parse yaml: ${(e as Error).message}`);
  }
}

async function loadScope(w: Walked, urlDir: string, raw: unknown, cfgPath: string): Promise<Scope> {
  const rf = asRecord(raw, cfgPath);
  const scope: Scope = {
    prefix: urlDir === "/" ? "" : urlDir,
    dir: urlDir,
    routes: [],
    listing: false,
    headers: {},
  };
  if (rf.listing != null) {
    if (typeof rf.listing !== "boolean") throw new Error(`${cfgPath}: listing must be a boolean`);
    scope.listing = rf.listing;
  }
  for (const [k, v] of Object.entries(asRecord(rf.headers, `${cfgPath}: headers`))) {
    if (typeof v !== "string") throw new Error(`${cfgPath}: headers.${k} must be a string`);
    scope.headers[k] = v;
  }
  const routes = rf.routes ?? [];
  if (!Array.isArray(routes)) throw new Error(`${cfgPath}: routes must be a list`);
  routes.forEach((raw, i) => {
    const where = `${cfgPath} route[${i}]`;
    const route = parseRoute(raw, where);
    try {
      validateRoute(route, urlDir);
      if ((route.type === "rewrite" || route.type === "file") && !route.to.includes("{")) {
        const target = resolveTargetPath(urlDir, route.to);
        if (!Object.hasOwn(w.files, target)) throw new Error(`target not found: ${target}`);
      }
    } catch (e) {
      throw new Error(`${where}: ${(e as Error).message}`);
    }
    scope.routes.push(route);
  });
  if (rf.dist != null) {
    const dist = asRecord(rf.dist, `${cfgPath}: dist`);
    if (typeof dist.path !== "string" || dist.path === "") {
      throw new Error(`${cfgPath}: dist.path is required`);
    }
    const distPath = resolveTargetPath(urlDir, dist.path);
    if (!Object.hasOwn(w.dirs, distPath)) {
      throw new Error(`${cfgPath} dist: path "${dist.path}" does not exist or is not a directory`);
    }
    scope.dist = { path: distPath };
    if (dist.fallback != null && dist.fallback !== "") {
      if (typeof dist.fallback !== "string") throw new Error(`${cfgPath}: dist.fallback must be a string`);
      scope.dist.fallback = resolveTargetPath(distPath, dist.fallback);
    }
  }
  return scope;
}

export async function build(dataDir: string, outDir: string): Promise<Manifest> {
  dataDir = path.resolve(dataDir);
  try {
    if (!(await fs.stat(dataDir)).isDirectory()) throw new Error();
  } catch {
    throw new Error(`data dir not found: ${dataDir}`);
  }
  const w = await walk(dataDir);

  const globalRaw = await readYaml(path.join(dataDir, ".config.yml"));
  let global: Manifest["global"];
  try {
    global = parseGlobalConfig(globalRaw);
  } catch (e) {
    throw new Error(`.config.yml: ${(e as Error).message}`);
  }

  const scopes: Scope[] = [];
  for (const urlDir of Object.keys(w.dirs).sort()) {
    const diskDir = w.dirDisk[urlDir];
    const raw = await readYaml(path.join(diskDir, ".routes.yml"));
    if (raw === undefined) continue;
    const cfgPath = (urlDir === "/" ? "" : urlDir.slice(1) + "/") + ".routes.yml";
    scopes.push(await loadScope(w, urlDir, raw, cfgPath));
  }

  // Non-fatal: routes that make a real file unreachable.
  for (const s of scopes) {
    for (const r of s.routes) {
      const matchPath = joinURLPrefix(s.prefix, r.from);
      const p = compileParamPattern(matchPath);
      for (const f of Object.keys(w.files)) {
        if (p ? p.re.test(f) : !r.from.includes("*") && matchPath === f) {
          console.warn(`warn: route ${matchPath} shadows file ${f}`);
        }
      }
    }
  }

  const marked = new Marked({ gfm: true, breaks: true }, gfmHeadingId());
  const readmes: Manifest["readmes"] = {};
  for (const [urlDir, d] of Object.entries(w.dirs)) {
    const name = d.entries.find((e) => !e.endsWith("/") && e.toLowerCase() === "readme.md");
    if (!name) continue;
    const src = await fs.readFile(path.join(w.dirDisk[urlDir], name), "utf8");
    const body = marked.parse(src, { async: false });
    const title = urlDir === "/" ? name.replace(/\.[^.]*$/, "") : urlDir.replace(/^\/+|\/+$/g, "");
    readmes[urlDir] = readmePage(title, body);
  }

  const hash = createHash("sha256");
  for (const f of Object.keys(w.files).sort()) {
    const content = createHash("sha256").update(await fs.readFile(w.diskPaths[f])).digest("hex");
    hash.update(`${f}\t${w.files[f].size}\t${content}\n`);
  }

  const manifest: Manifest = {
    version: 1,
    assetsHash: hash.digest("hex"),
    global,
    scopes,
    files: w.files,
    dirs: w.dirs,
    readmes,
  };

  const publicDir = path.join(outDir, "public");
  await fs.rm(publicDir, { recursive: true, force: true });
  for (const [f, disk] of Object.entries(w.diskPaths)) {
    const dest = path.join(publicDir, f);
    await fs.mkdir(path.dirname(dest), { recursive: true });
    await fs.copyFile(disk, dest);
  }
  await fs.mkdir(publicDir, { recursive: true });
  await fs.mkdir(path.join(outDir, ".mewroute"), { recursive: true });
  await fs.writeFile(path.join(outDir, ".mewroute", "manifest.json"), JSON.stringify(manifest));
  return manifest;
}

export function resolveDataDir(argv: string[]): string {
  const i = argv.indexOf("--data");
  if (i >= 0 && argv[i + 1]) return path.resolve(argv[i + 1]);
  if (process.env.MEWROUTE_DATA) return path.resolve(process.env.MEWROUTE_DATA);
  return path.resolve(workerDir, "../data");
}

export const WORKER_DIR = workerDir;

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const m = await build(resolveDataDir(process.argv.slice(2)), workerDir);
    console.log(
      `built ${Object.keys(m.files).length} files, ${m.scopes.length} scopes -> .mewroute/manifest.json`,
    );
  } catch (e) {
    console.error((e as Error).message);
    process.exit(1);
  }
}
