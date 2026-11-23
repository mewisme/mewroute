// HTML pages ported from internal/listing/html.go and internal/markdown/render.go.
import type { Manifest } from "./resolve.js";

/** Go html.EscapeString. */
export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/'/g, "&#39;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&#34;");
}

export function readmePage(title: string, body: string): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)}</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;line-height:1.6;max-width:48rem;margin:2rem auto;padding:0 1rem;color:#1f2328}
h1,h2,h3,h4{border-bottom:1px solid #d8dee4;padding-bottom:.3em}
a{color:#0969da;text-decoration:none}
a:hover{text-decoration:underline}
code,pre{font-family:ui-monospace,monospace;font-size:.9em}
pre{background:#f6f8fa;padding:1rem;overflow:auto;border-radius:6px}
code{background:#f6f8fa;padding:.2em .4em;border-radius:4px}
pre code{background:none;padding:0}
table{border-collapse:collapse;width:100%;margin:1rem 0}
th,td{border:1px solid #d8dee4;padding:.5rem .75rem}
blockquote{color:#59636e;border-left:.25em solid #d8dee4;margin:0;padding-left:1rem}
img{max-width:100%}
</style>
</head>
<body>
${body}
</body>
</html>
`;
}

export function listingPage(m: Manifest, dir: string): string {
  const base = dir === "/" ? "/" : dir.replace(/\/$/, "") || "/";
  const prefix = base === "/" ? "/" : base + "/";
  const items = (m.dirs[dir]?.entries ?? []).map((e) => {
    const isDir = e.endsWith("/");
    const name = isDir ? e.slice(0, -1) : e;
    const full = prefix + name;
    const meta = isDir ? m.dirs[full] : m.files[full];
    return { name, isDir, size: isDir ? 0 : m.files[full]?.size ?? 0, mod: meta?.mtime ?? 0 };
  });
  items.sort((a, b) =>
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

  let out = `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Index of ${escapeHtml(base)}</title>`;
  out +=
    "<style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse;width:100%}th,td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #ddd}th{background:#f5f5f5}a{text-decoration:none;color:#0366d6}</style></head><body>";
  out += `<h1>Index of ${escapeHtml(base)}</h1><table><thead><tr><th>Name</th><th>Size</th><th>Modified</th></tr></thead><tbody>`;
  if (base !== "/") {
    const idx = base.lastIndexOf("/");
    const parent = idx > 0 ? base.slice(0, idx + 1) : "/";
    out += `<tr><td><a href="${escapeHtml(parent)}">../</a></td><td>-</td><td>-</td></tr>`;
  }
  for (const it of items) {
    const href = prefix + it.name + (it.isDir ? "/" : "");
    const mod = new Date(it.mod).toUTCString().replace("GMT", "UTC");
    out += `<tr><td><a href="${escapeHtml(href)}">${escapeHtml(it.name + (it.isDir ? "/" : ""))}</a></td><td>${it.isDir ? "-" : it.size}</td><td>${escapeHtml(mod)}</td></tr>`;
  }
  return out + "</tbody></table></body></html>";
}
