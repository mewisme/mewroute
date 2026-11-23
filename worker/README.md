# mewroute on Cloudflare Workers

Same `.routes.yml` / `.config.yml` format as the Go server (which stays unchanged). A build step compiles the data dir into:

- `public/`: Workers Static Assets (every non-dot file),
- `.mewroute/manifest.json`: route table, file tree, rendered READMEs. Stored in KV under `manifest`.

Data dir: `--data <path>` > `MEWROUTE_DATA` > `../data`.

```sh
pnpm install
pnpm test
MEWROUTE_DATA=/path/to/data pnpm dev   # http://localhost:8787
```

## Deploy

One-time: `pnpm exec wrangler kv namespace create MANIFEST`, then put the returned `id` in `wrangler.jsonc` (`kv_namespaces[0].id`). Already done for the maintainer's account.

| Command | Use |
|---|---|
| `pnpm run deploy` | Full deploy: assets + code, then manifest and `assets-hash` to KV. Needed when files change. |
| `pnpm sync-config` | Config-only: pushes the manifest to KV without redeploying. Refuses if any file changed since the last deploy. Live after KV propagation (~60 s) plus the 30 s isolate cache. |

## Differences from the Go server

- Dotfiles/dotdirs, `node_modules` and symlinks are never published.
- Any invalid config fails the build (Go logs and skips the scope).
- No hot reload; no `Range` slicing for ranged requests (full 200 is returned).
- Max 25 MiB per file, 20000 files (Workers Assets limits).

## Notes

- Always set `MEWROUTE_DATA` (or `--data`) for `deploy` / `sync-config`: without it the build uses `../data` (sample data) and overwrites the live manifest and assets.
- `pnpm install` needs `allowBuilds` for `esbuild` and `workerd` (set in `pnpm-workspace.yaml`).
- Tests: `pnpm test` (vitest; parity cases ported from the Go resolver/params/handler tests).
