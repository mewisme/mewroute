import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { fixture, load } from "./helpers.js";

const testdata = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../testdata");

describe("repo testdata (ported from resolver_test.go)", async () => {
  const { r, req } = await load(testdata);

  it("rewrite", () => expect(r("/scripts/hello")).toMatchObject({ kind: "file", path: "/scripts/hello.ps1" }));
  it("redirect 302", () =>
    expect(r("/scripts/old")).toMatchObject({ kind: "redirect", location: "/scripts/hello", status: 302 }));
  it("external redirect 301", () =>
    expect(r("/scripts/external")).toMatchObject({
      kind: "redirect",
      location: "https://example.com/docs",
      status: 301,
    }));
  it("dist SPA fallback", () => expect(r("/app/users/1")).toMatchObject({ kind: "file", path: "/app/build/index.html" }));
  it("dist asset", () => expect(r("/app/assets/main.js")).toMatchObject({ kind: "file", path: "/app/build/assets/main.js" }));
  it("static file", () => expect(r("/tools/linux/setup.sh").kind).toBe("file"));
  it("nested scope", () => expect(r("/nested/child/secret")).toMatchObject({ kind: "file", path: "/nested/child/nested.txt" }));
  it("blocked config", () => {
    expect(r("/scripts/.routes.yml").kind).toBe("notFound");
    expect(r("/.config.yml").kind).toBe("notFound");
    expect(r("/scripts/%2e%2e/.routes.yml").kind).toBe("notFound");
  });
  it("wildcard", () => expect(r("/scripts/docs/readme").kind).toBe("file"));
  it("listing", () => expect(r("/scripts").kind).toBe("listing"));
  it("root readme", () => expect(r("/").kind).toBe("readme"));
  it("git inactive without config", () => expect(r("/git/wrec").kind).toBe("notFound"));

  it("scope headers apply to dist and download adds disposition", async () => {
    const app = await req("/app/users/1");
    expect(app.status).toBe(200);
    expect(app.headers.get("X-App")).toBe("demo");
    expect(app.headers.get("Cache-Control")).toBe("no-store");
    expect(app.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
    const dl = await req("/scripts/download");
    expect(dl.headers.get("Content-Disposition")).toBe('attachment; filename="hello.ps1"');
    expect(dl.headers.get("X-Test-Scope")).toBe("scripts");
    expect(dl.headers.get("Content-Type")).toBe("text/plain; charset=utf-8");
    expect(dl.headers.get("Cache-Control")).toBe("no-cache, must-revalidate");
  });

  it("listing and readme pages", async () => {
    const l = await req("/scripts");
    expect(l.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
    const html = await l.text();
    expect(html).toContain("<h1>Index of /scripts</h1>");
    expect(html).toContain('<a href="/">../</a>');
    expect(html).toContain('<a href="/scripts/hello.ps1">hello.ps1</a>');
    expect(html).not.toContain(".routes.yml");
    const root = await req("/");
    expect(await root.text()).toContain("<title>README</title>");
  });

  it("methods, healthz, redirect headers, 404", async () => {
    expect((await req("/scripts/hello", { method: "POST" })).status).toBe(405);
    expect((await req("/scripts/hello", { method: "POST" })).headers.get("Allow")).toBe("GET, HEAD");
    expect(await (await req("/healthz")).text()).toBe("ok");
    const red = await req("/scripts/old", { method: "HEAD" });
    expect([red.status, red.headers.get("Location"), red.headers.get("X-Test-Scope")]).toEqual([302, "/scripts/hello", "scripts"]);
    const nf = await req("/nope");
    expect([nf.status, await nf.text()]).toEqual([404, "404 page not found\n"]);
    expect((await req("/%E0%A4%A")).status).toBe(404);
    expect((await req("/.config.yml")).status).toBe(404);
  });
});

describe("params and git (ported + user data)", () => {
  const userData = {
    ".config.yml": "git:\n  username: mewisme\n  route: /git\n",
    ".routes.yml": `routes:
  - from: /discloud.{ext}
    to: https://raw.githubusercontent.com/{git.username}/discloud-go/refs/heads/main/install.{ext}
    type: redirect
    params:
      ext:
        allow: [sh, ps1]
  - from: /{repo}.{ext}
    to: https://raw.githubusercontent.com/{git.username}/{repo}/refs/heads/main/install.{ext}
    type: redirect
    params:
      ext:
        allow: [sh, ps1]
`,
    "wrec/.routes.yml": "routes:\n  - from: /\n    to: ./install.ps1\n    type: rewrite\n",
    "wrec/install.ps1": "#Requires -Version 5.1\n",
  };

  it("discloud wins over generic param, allow list applies", async () => {
    const { r } = await fixture(userData);
    expect(r("/discloud.sh")).toMatchObject({
      status: 302,
      location: "https://raw.githubusercontent.com/mewisme/discloud-go/refs/heads/main/install.sh",
    });
    expect(r("/agentrule.ps1")).toMatchObject({
      location: "https://raw.githubusercontent.com/mewisme/agentrule/refs/heads/main/install.ps1",
    });
    expect(r("/agentrule.exe").kind).toBe("notFound");
  });

  it("scope root rewrite serves with text/plain", async () => {
    const { r, req } = await fixture(userData);
    expect(r("/wrec")).toMatchObject({ kind: "file", path: "/wrec/install.ps1" });
    const res = await req("/wrec");
    expect(res.headers.get("Content-Type")).toBe("text/plain; charset=utf-8");
    expect(await res.text()).toMatch(/^#Requires -Version 5.1/);
    expect((await req("/wrec/.routes.yml")).status).toBe(404);
  });

  it("git shortcut", async () => {
    const { r } = await fixture(userData);
    expect(r("/git/wrec")).toMatchObject({ status: 302, location: "https://github.com/mewisme/wrec" });
    expect(r("/git/mewisme/wrec")).toMatchObject({ location: "https://github.com/mewisme/wrec" });
    expect(r("/git")).toMatchObject({ location: "https://github.com/mewisme" });
    expect(r("/git/a/b/c").kind).toBe("notFound");
  });

  it("exact route wins over param route", async () => {
    const { r } = await fixture({
      ".routes.yml": `routes:
  - from: /fixed.ps1
    to: https://example.com/exact
    type: redirect
  - from: /{repo}.{ext}
    to: https://example.com/{repo}.{ext}
    type: redirect
`,
    });
    expect(r("/fixed.ps1")).toMatchObject({ location: "https://example.com/exact" });
    expect(r("/other.sh")).toMatchObject({ location: "https://example.com/other.sh" });
  });

  it("root-anchored target", async () => {
    const { r } = await fixture({
      ".routes.yml": "routes:\n  - from: /fetch\n    to: /scripts/hello.ps1\n    type: file\n",
      "scripts/hello.ps1": "hi",
    });
    expect(r("/fetch")).toMatchObject({ kind: "file", path: "/scripts/hello.ps1", download: false });
  });

  it("dynamic target resolving to a missing file is 404", async () => {
    const { r } = await fixture({
      ".routes.yml": "routes:\n  - from: /get/{name}\n    to: /files/{name}.txt\n    type: file\n",
      "files/a.txt": "a",
    });
    expect(r("/get/a").kind).toBe("file");
    expect(r("/get/b").kind).toBe("notFound");
  });
});
