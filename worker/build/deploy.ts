import { execFileSync } from "node:child_process";
import { build, resolveDataDir, WORKER_DIR } from "./build.js";

const wrangler = (args: string[], capture = false): string =>
  execFileSync("npx", ["wrangler", ...args], {
    cwd: WORKER_DIR,
    shell: true,
    stdio: capture ? ["inherit", "pipe", "inherit"] : "inherit",
    encoding: "utf8",
  }) ?? "";

const configOnly = process.argv.includes("--config-only");
const m = await build(resolveDataDir(process.argv.slice(2)), WORKER_DIR);
const putManifest = () =>
  wrangler(["kv", "key", "put", "manifest", "--path=.mewroute/manifest.json", "--binding=MANIFEST", "--remote"]);

if (configOnly) {
  let remote = "";
  try {
    remote = wrangler(["kv", "key", "get", "assets-hash", "--binding=MANIFEST", "--remote", "--text"], true).trim();
  } catch {
    /* missing key: treated as mismatch */
  }
  if (remote !== m.assetsHash) {
    console.error("files changed since last deploy; run npm run deploy");
    process.exit(1);
  }
  putManifest();
} else {
  // Assets first: a new manifest must never reference files that are not live yet.
  wrangler(["deploy"]);
  putManifest();
  wrangler(["kv", "key", "put", "assets-hash", m.assetsHash, "--binding=MANIFEST", "--remote"]);
}
