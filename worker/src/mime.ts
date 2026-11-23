// Overrides copied from internal/filesystem/serve.go (mimeOverrides), plus a small fallback set.
const TYPES: Record<string, string> = {
  ".ps1": "text/plain; charset=utf-8",
  ".sh": "text/plain; charset=utf-8",
  ".bat": "text/plain; charset=utf-8",
  ".cmd": "text/plain; charset=utf-8",
  ".exe": "application/octet-stream",
  ".py": "text/plain; charset=utf-8",
  ".js": "text/plain; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".htm": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".yml": "text/yaml; charset=utf-8",
  ".yaml": "text/yaml; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".md": "text/markdown; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".wasm": "application/wasm",
  ".zip": "application/zip",
  ".pdf": "application/pdf",
  ".xml": "application/xml",
  ".woff2": "font/woff2",
};

export function contentType(name: string): string {
  const dot = name.lastIndexOf(".");
  const ext = dot < 0 ? "" : name.slice(dot).toLowerCase();
  return TYPES[ext] ?? "application/octet-stream";
}
