// Pure routing logic, ported from the Go implementation (internal/config, internal/router,
// internal/filesystem). No Workers or Node APIs: shared by the build step and the Worker.
// All paths are root-relative POSIX strings ("/a/b", root = "/").

export interface Manifest {
  version: 1;
  assetsHash: string;
  global: { git: { username: string; route: string } };
  scopes: Scope[];
  files: Record<string, { size: number; mtime: number }>;
  dirs: Record<string, { mtime: number; entries: string[] }>;
  readmes: Record<string, string>;
}

export interface Scope {
  prefix: string;
  dir: string;
  routes: RouteDef[];
  dist?: { path: string; fallback?: string };
  listing: boolean;
  headers: Record<string, string>;
}

export interface RouteDef {
  from: string;
  to: string;
  type: "rewrite" | "redirect" | "file";
  status?: number;
  download?: boolean;
  params?: Record<string, { allow?: string[] }>;
}

export type Decision =
  | { kind: "notFound" }
  | { kind: "redirect"; status: number; location: string; headers: Record<string, string> }
  | { kind: "file"; path: string; download: boolean; headers: Record<string, string> }
  | { kind: "readme"; dir: string; headers: Record<string, string> }
  | { kind: "listing"; dir: string; headers: Record<string, string> };

const NOT_FOUND: Decision = { kind: "notFound" };
const ALLOWED_REDIRECT_STATUS = new Set([301, 302, 307, 308]);
const BLOCKED_NAMES = new Set([".routes.yml", ".routes.yaml", ".config.yml", ".config.yaml"]);
const GIT_SEGMENT = /^[a-zA-Z0-9._-]+$/;
const ROUTE_PLACEHOLDER = /\{([A-Za-z][A-Za-z0-9_-]*)\}/g;
const TARGET_PLACEHOLDER = /\{([A-Za-z][A-Za-z0-9_.-]*)\}/g;

// ---- paths -----------------------------------------------------------------

/** Go path.Clean. */
export function cleanPath(p: string): string {
  if (p === "") return ".";
  const rooted = p.startsWith("/");
  const out: string[] = [];
  for (const seg of p.split("/")) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") {
      if (out.length > 0 && out[out.length - 1] !== "..") out.pop();
      else if (!rooted) out.push("..");
    } else out.push(seg);
  }
  const joined = out.join("/");
  if (rooted) return "/" + joined;
  return joined === "" ? "." : joined;
}

export function isBlockedFileName(base: string): boolean {
  return BLOCKED_NAMES.has(base.toLowerCase());
}

export function isBlockedPath(urlPath: string): boolean {
  const clean = cleanPath("/" + urlPath.replace(/^\//, ""));
  return clean
    .replace(/^\/+|\/+$/g, "")
    .split("/")
    .some(isBlockedFileName);
}

export function pathClean(value: string): string {
  if (value === "" || value === "/") return "/";
  value = value.replace(/\/$/, "");
  if (value === "") return "/";
  return value.startsWith("/") ? value : "/" + value;
}

export function normalizeFromPath(from: string): string {
  from = from.trim();
  if (!from.startsWith("/")) from = "/" + from;
  return pathClean(from);
}

export function joinURLPrefix(prefix: string, from: string): string {
  from = normalizeFromPath(from);
  if (prefix === "" || prefix === "/") return from;
  prefix = pathClean(prefix);
  return from === "/" ? prefix : prefix + from;
}

export function normalizeURLPath(value: string): string {
  if (value === "") return "/";
  if (!value.startsWith("/")) value = "/" + value;
  const clean = cleanPath(value);
  return clean === "." ? "/" : clean;
}

export function pathUnderPrefix(urlPath: string, prefix: string): boolean {
  if (prefix === "" || prefix === "/") return true;
  prefix = prefix.replace(/\/$/, "");
  return urlPath === prefix || urlPath.startsWith(prefix + "/");
}

/** Resolve a route target / dist path to a root-relative path or throw. */
export function resolveTargetPath(scopeDir: string, to: string): string {
  to = to.trim();
  if (to.startsWith("/")) {
    if (to.includes("..")) throw new Error("path traversal not allowed");
    const clean = cleanPath(to);
    if (clean === "/") throw new Error("invalid root path");
    return clean;
  }
  if (to.startsWith("\\") || /^[a-zA-Z]:/.test(to)) {
    throw new Error(`absolute filesystem paths not allowed: ${to}`);
  }
  const clean = cleanPath(to.replace(/\\/g, "/"));
  if (clean === ".." || clean.startsWith("../")) {
    throw new Error(`path traversal not allowed: ${to}`);
  }
  return cleanPath(scopeDir + "/" + clean);
}

export function normalizeRedirectTarget(to: string): string {
  to = to.trim();
  if (to === "") throw new Error("empty redirect target");
  if (to.includes("://")) {
    let u: URL;
    try {
      u = new URL(to);
    } catch (e) {
      throw new Error(`invalid redirect URL: ${(e as Error).message}`);
    }
    if (u.protocol !== "http:" && u.protocol !== "https:") {
      throw new Error("redirect URL scheme must be http or https");
    }
    if (u.host === "") throw new Error("invalid redirect URL: missing host");
    return to;
  }
  if (to.startsWith("//")) {
    let host = "";
    try {
      host = new URL("https:" + to).host;
    } catch {
      /* fallthrough */
    }
    if (host === "") throw new Error("invalid protocol-relative redirect URL");
    return to;
  }
  if (to.startsWith("/")) {
    if (to.includes("..")) throw new Error("path traversal not allowed");
    const clean = cleanPath(to);
    return clean === "." ? "/" : clean;
  }
  if (to.includes(":")) throw new Error("unsupported redirect target");
  return "/" + to.replace(/^\//, "");
}

function isSameSiteRedirect(location: string): boolean {
  return !location.includes("://") && !location.startsWith("//");
}

// ---- params ----------------------------------------------------------------

export function routeParamNames(pattern: string): string[] {
  const names: string[] = [];
  const seen = new Set<string>();
  for (const m of pattern.matchAll(ROUTE_PLACEHOLDER)) {
    if (seen.has(m[1])) throw new Error(`duplicate route param "${m[1]}"`);
    seen.add(m[1]);
    names.push(m[1]);
  }
  if (/[{}]/.test(pattern) && /[{}]/.test(pattern.replace(ROUTE_PLACEHOLDER, ""))) {
    throw new Error("invalid route placeholder syntax");
  }
  return names;
}

export function targetPlaceholderNames(target: string): string[] {
  const names = [...target.matchAll(TARGET_PLACEHOLDER)].map((m) => m[1]);
  if (/[{}]/.test(target) && /[{}]/.test(target.replace(TARGET_PLACEHOLDER, ""))) {
    throw new Error("invalid target placeholder syntax");
  }
  return names;
}

export function validateRouteParams(route: RouteDef): void {
  const names = routeParamNames(route.from);
  if (names.length > 0 && route.from.includes("*")) {
    throw new Error("cannot mix named params and wildcard in from");
  }
  const defined = new Set(names);
  for (const [name, rule] of Object.entries(route.params ?? {})) {
    if (!defined.has(name)) throw new Error(`params.${name} is not declared in from`);
    const allow = rule?.allow ?? [];
    const seen = new Set<string>();
    for (const value of allow) {
      if (value === "") throw new Error(`params.${name}.allow contains empty value`);
      if (value.includes("/")) {
        throw new Error(`params.${name}.allow value "${value}" must not contain /`);
      }
      if (seen.has(value)) throw new Error(`params.${name}.allow contains duplicate "${value}"`);
      seen.add(value);
    }
  }
  for (const name of targetPlaceholderNames(route.to)) {
    if (defined.has(name) || name === "git.username") continue;
    throw new Error(`target placeholder "${name}" is not available`);
  }
}

/** Returns null when a placeholder has no value (Go returns an error -> 404). */
export function interpolateTarget(
  target: string,
  params: Record<string, string> | undefined,
  username: string,
): string | null {
  let failed = false;
  const result = target.replace(TARGET_PLACEHOLDER, (token, name: string) => {
    if (params && Object.hasOwn(params, name)) return params[name];
    if (name === "git.username" && username !== "") return username;
    failed = true;
    return token;
  });
  return failed ? null : result;
}

export function paramAllowed(allow: string[] | undefined, value: string): boolean {
  return !allow || allow.length === 0 || allow.includes(value);
}

function quoteMeta(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function compileParamPattern(pattern: string): { re: RegExp; keys: string[] } | null {
  const keys = routeParamNames(pattern);
  if (keys.length === 0) return null;
  let src = "^";
  let cursor = 0;
  for (const m of pattern.matchAll(ROUTE_PLACEHOLDER)) {
    src += quoteMeta(pattern.slice(cursor, m.index)) + "([^/]+)";
    cursor = m.index + m[0].length;
  }
  src += quoteMeta(pattern.slice(cursor)) + "$";
  return { re: new RegExp(src), keys };
}

export function wildcardMatch(pattern: string, urlPath: string): boolean {
  if (!pattern.includes("*")) return pattern === urlPath;
  if (pattern.endsWith("/*")) {
    const prefix = pattern.slice(0, -2);
    return urlPath === prefix || urlPath.startsWith(prefix + "/");
  }
  const parts = pattern.split("*");
  return parts.length === 2 && urlPath.startsWith(parts[0]) && urlPath.endsWith(parts[1]);
}

// ---- config validation -----------------------------------------------------

/** Port of config.validateRoute (target existence is checked by the build). */
export function validateRoute(route: RouteDef, scopeDir: string): void {
  if (route.from.trim() === "") throw new Error("from is required");
  if (route.to.trim() === "") throw new Error("to is required");
  validateRouteParams(route);
  if (isBlockedPath(normalizeFromPath(route.from))) {
    throw new Error("cannot map config file as route from");
  }
  const dynamicTarget = route.to.includes("{");
  switch (route.type) {
    case "rewrite":
    case "file": {
      if (dynamicTarget) return;
      let abs: string;
      try {
        abs = resolveTargetPath(scopeDir, route.to);
      } catch (e) {
        throw new Error(`to: ${(e as Error).message}`);
      }
      if (isBlockedPath(abs)) throw new Error("cannot map config file as route target");
      return;
    }
    case "redirect": {
      if (route.status != null && route.status !== 0 && !ALLOWED_REDIRECT_STATUS.has(route.status)) {
        throw new Error(`invalid redirect status ${route.status}`);
      }
      if (dynamicTarget) return;
      let location: string;
      try {
        location = normalizeRedirectTarget(route.to);
      } catch (e) {
        throw new Error(`to: ${(e as Error).message}`);
      }
      if (isSameSiteRedirect(location) && isBlockedPath(location)) {
        throw new Error("cannot redirect to config file");
      }
      return;
    }
    default:
      throw new Error(`invalid type "${String(route.type)}"`);
  }
}

export function parseGlobalConfig(raw: unknown): Manifest["global"] {
  const git = ((raw as { git?: { username?: unknown; route?: unknown } } | null)?.git ?? {}) as {
    username?: unknown;
    route?: unknown;
  };
  const str = (v: unknown, field: string): string => {
    if (v == null) return "";
    if (typeof v !== "string") throw new Error(`${field} must be a string`);
    return v.trim();
  };
  const username = str(git.username, "git.username");
  let route = str(git.route, "git.route");
  if (username !== "" && !GIT_SEGMENT.test(username)) {
    throw new Error(`git.username: invalid segment "${username}"`);
  }
  if (route !== "") {
    route = normalizeFromPath(route);
    if (route === "/") throw new Error("git.route must not be /");
  }
  return { git: { username, route } };
}

export function resolveGitRedirect(g: Manifest["global"], urlPath: string): string | null {
  const { username } = g.git;
  if (username.trim() === "") return null;
  const prefix = g.git.route === "" ? "/git" : g.git.route;
  urlPath = cleanPath("/" + urlPath.replace(/^\//, ""));
  if (urlPath !== prefix && !urlPath.startsWith(prefix + "/")) return null;
  const suffix = urlPath.slice(prefix.length).replace(/^\//, "");
  if (suffix === "") return "https://github.com/" + username;
  const segs = suffix.split("/");
  if (segs.length > 2 || segs.some((s) => !GIT_SEGMENT.test(s))) return null;
  return segs.length === 1
    ? `https://github.com/${username}/${segs[0]}`
    : `https://github.com/${segs[0]}/${segs[1]}`;
}

// ---- table + resolve -------------------------------------------------------

interface Compiled {
  scopeLen: number;
  scope: Scope;
  route: RouteDef;
  matchPath: string;
  re?: RegExp;
  keys?: string[];
}

export interface Table {
  exact: Compiled[];
  param: Compiled[];
  wildcard: Compiled[];
  dist: Scope[];
  scopes: Scope[];
}

export function compileTable(m: Manifest): Table {
  const t: Table = { exact: [], param: [], wildcard: [], dist: [], scopes: m.scopes };
  for (const scope of m.scopes) {
    for (const route of scope.routes) {
      const c: Compiled = {
        scopeLen: scope.prefix.length,
        scope,
        route,
        matchPath: joinURLPrefix(scope.prefix, route.from),
      };
      const p = compileParamPattern(c.matchPath);
      if (p) {
        c.re = p.re;
        c.keys = p.keys;
        t.param.push(c);
      } else if (route.from.includes("*")) t.wildcard.push(c);
      else t.exact.push(c);
    }
    if (scope.dist) t.dist.push(scope);
  }
  const byLen = (a: Compiled, b: Compiled) =>
    b.scopeLen - a.scopeLen || b.matchPath.length - a.matchPath.length;
  t.exact.sort(byLen);
  t.param.sort(byLen);
  t.wildcard.sort(byLen);
  t.dist.sort((a, b) => b.prefix.length - a.prefix.length);
  return t;
}

function scopeFor(t: Table, urlPath: string, listingOnly: boolean): Scope | null {
  let best: Scope | null = null;
  let bestLen = -1;
  for (const s of t.scopes) {
    if (listingOnly && !s.listing) continue;
    const prefix = s.prefix === "" ? "/" : s.prefix;
    if (pathUnderPrefix(urlPath, prefix) || urlPath === prefix.replace(/\/$/, "")) {
      if (s.prefix.length > bestLen) {
        best = s;
        bestLen = s.prefix.length;
      }
    }
  }
  return best;
}

export function scopeHeadersForPath(t: Table, urlPath: string): Record<string, string> {
  return { ...(scopeFor(t, urlPath, false)?.headers ?? {}) };
}

export function longestListingScope(t: Table, urlPath: string): Scope | null {
  return scopeFor(t, urlPath, true);
}

function decisionFromRoute(
  c: Compiled,
  m: Manifest,
  params: Record<string, string> | undefined,
): Decision {
  const headers = { ...c.scope.headers };
  const target = interpolateTarget(c.route.to, params, m.global.git.username);
  if (target === null) return NOT_FOUND;
  switch (c.route.type) {
    case "redirect": {
      let location: string;
      try {
        location = normalizeRedirectTarget(target);
      } catch {
        return NOT_FOUND;
      }
      if (isSameSiteRedirect(location) && isBlockedPath(location)) return NOT_FOUND;
      return { kind: "redirect", status: c.route.status || 302, location, headers };
    }
    case "rewrite":
    case "file": {
      let abs: string;
      try {
        abs = resolveTargetPath(c.scope.dir, target);
      } catch {
        return NOT_FOUND;
      }
      if (isBlockedPath(abs) || !Object.hasOwn(m.files, abs)) return NOT_FOUND;
      return {
        kind: "file",
        path: abs,
        download: c.route.type === "file" && !!c.route.download,
        headers,
      };
    }
    default:
      return NOT_FOUND;
  }
}

function matchDist(t: Table, m: Manifest, urlPath: string): Decision {
  for (const scope of t.dist) {
    const dist = scope.dist!;
    const prefix = scope.prefix;
    if (prefix !== "" && urlPath !== prefix && !urlPath.startsWith(prefix + "/")) continue;
    const suffix = (prefix === "" ? urlPath : urlPath.slice(prefix.length)).replace(/^\//, "");
    let target: string;
    if (suffix === "") target = dist.fallback || cleanPath(dist.path + "/index.html");
    else target = cleanPath(dist.path + "/" + suffix);
    if (Object.hasOwn(m.files, target)) {
      return { kind: "file", path: target, download: false, headers: { ...scope.headers } };
    }
    if (dist.fallback && Object.hasOwn(m.files, dist.fallback)) {
      return { kind: "file", path: dist.fallback, download: false, headers: { ...scope.headers } };
    }
  }
  return NOT_FOUND;
}

export function resolve(t: Table, m: Manifest, rawPath: string): Decision {
  const urlPath = normalizeURLPath(rawPath);
  if (isBlockedPath(urlPath)) return NOT_FOUND;

  for (const c of t.exact) {
    if (c.matchPath === urlPath) {
      const d = decisionFromRoute(c, m, undefined);
      if (d.kind !== "notFound") return d;
    }
  }

  for (const c of t.param) {
    const match = c.re!.exec(urlPath);
    if (!match) continue;
    const params: Record<string, string> = {};
    let allowed = true;
    for (let i = 0; i < c.keys!.length; i++) {
      const key = c.keys![i];
      if (!paramAllowed(c.route.params?.[key]?.allow, match[i + 1])) {
        allowed = false;
        break;
      }
      params[key] = match[i + 1];
    }
    if (allowed) {
      const d = decisionFromRoute(c, m, params);
      if (d.kind !== "notFound") return d;
    }
  }

  for (const c of t.wildcard) {
    if (wildcardMatch(c.matchPath, urlPath)) {
      const d = decisionFromRoute(c, m, undefined);
      if (d.kind !== "notFound") return d;
    }
  }

  const git = resolveGitRedirect(m.global, urlPath);
  if (git) return { kind: "redirect", status: 302, location: git, headers: {} };

  const dist = matchDist(t, m, urlPath);
  if (dist.kind !== "notFound") return dist;

  if (Object.hasOwn(m.files, urlPath)) {
    return { kind: "file", path: urlPath, download: false, headers: scopeHeadersForPath(t, urlPath) };
  }
  const dir = m.dirs[urlPath];
  if (!dir) return NOT_FOUND;
  for (const name of ["index.html", "index.htm"]) {
    if (dir.entries.includes(name)) {
      const p = urlPath === "/" ? "/" + name : `${urlPath}/${name}`;
      return { kind: "file", path: p, download: false, headers: scopeHeadersForPath(t, urlPath) };
    }
  }
  if (Object.hasOwn(m.readmes, urlPath)) {
    return { kind: "readme", dir: urlPath, headers: scopeHeadersForPath(t, urlPath) };
  }
  const listing = longestListingScope(t, urlPath);
  if (listing) return { kind: "listing", dir: urlPath, headers: { ...listing.headers } };
  return NOT_FOUND;
}
