import fs from "node:fs/promises";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { build } from "../build/build.js";
import { tmp, writeTree } from "./helpers.js";

async function data(files: Record<string, string>) {
  const dir = await tmp();
  await writeTree(dir, files);
  return dir;
}

describe("build", () => {
  it("excludes dotfiles from public and manifest", async () => {
    const dir = await data({ "a.txt": "a", ".secret": "s", ".routes.yml": "routes: []\n", ".config.yml": "", "d/.x/y": "y" });
    const out = await tmp();
    const m = await build(dir, out);
    expect(Object.keys(m.files)).toEqual(["/a.txt"]);
    expect(await fs.readdir(path.join(out, "public"))).toEqual(["a.txt"]);
  });

  it("fails on static target that does not exist", async () => {
    const dir = await data({ ".routes.yml": "routes:\n  - from: /x\n    to: ./missing.txt\n    type: rewrite\n" });
    await expect(build(dir, await tmp())).rejects.toThrow(/target not found/);
  });

  it("fails on invalid param declaration, type, and bad git config", async () => {
    const bad = (routes: string) => data({ ".routes.yml": routes, "f.txt": "f" });
    await expect(
      build(await bad("routes:\n  - from: /x\n    to: ./f.txt\n    type: rewrite\n    params:\n      y:\n        allow: [a]\n"), await tmp()),
    ).rejects.toThrow(/params\.y is not declared in from/);
    await expect(build(await bad("routes:\n  - from: /x\n    to: ./f.txt\n    type: nope\n"), await tmp())).rejects.toThrow(/invalid type "nope"/);
    await expect(
      build(await bad("routes:\n  - from: /x\n    to: https://a.b\n    type: redirect\n    status: 200\n"), await tmp()),
    ).rejects.toThrow(/invalid redirect status 200/);
    await expect(
      build(await data({ ".config.yml": "git:\n  route: /\n  username: a\n" }), await tmp()),
    ).rejects.toThrow(/git.route must not be \//);
  });

  it("fails when data dir is missing", async () => {
    await expect(build(path.join(await tmp(), "nope"), await tmp())).rejects.toThrow(/data dir not found/);
  });

  it("assetsHash is stable and tracks content, not config", async () => {
    const dir = await data({ "a.txt": "one", ".routes.yml": "routes: []\n" });
    const h1 = (await build(dir, await tmp())).assetsHash;
    await fs.writeFile(path.join(dir, ".routes.yml"), "listing: true\n");
    expect((await build(dir, await tmp())).assetsHash).toBe(h1);
    await fs.writeFile(path.join(dir, "a.txt"), "two");
    expect((await build(dir, await tmp())).assetsHash).not.toBe(h1);
  });
});
