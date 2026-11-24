import { execFileSync } from "node:child_process";
import { build, resolveDataDir, WORKER_DIR } from "./build.js";

const wrangler = (args: string[], capture = false): string =>
  execFileSync("pnpm", ["exec", "wrangler", ...args], {
    cwd: WORKER_DIR,
    shell: true,
    stdio: capture ? ["inherit", "pipe", "inherit"] : "inherit",
    encoding: "utf8",
  }) ?? "";

const configOnly = process.argv.includes("--config-only");
// Deploying overwrites the live manifest: never fall back silently to the sample ../data.
if (!process.argv.includes("--data") && !process.env.MEWROUTE_DATA && !process.argv.includes("--sample")) {
  console.error("set MEWROUTE_DATA (or --data <path>) to deploy; pass --sample to deploy the bundled sample data");
  process.exit(1);
}
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
    console.error("files changed since last deploy; run pnpm run deploy");
    process.exit(1);
  }
  putManifest();
} else {
  // Assets first: a new manifest must never reference files that are not live yet.
  wrangler(["deploy"]);
  putManifest();
  wrangler(["kv", "key", "put", "assets-hash", m.assetsHash, "--binding=MANIFEST", "--remote"]);
}
