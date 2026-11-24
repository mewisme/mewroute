import { describe, expect, it } from "vitest";
import { parseRange } from "../src/handler.js";
import { fixture } from "./helpers.js";

describe("parseRange", () => {
  it("parses forms", () => {
    expect(parseRange("bytes=0-3", 10)).toEqual({ start: 0, end: 3 });
    expect(parseRange("bytes=5-", 10)).toEqual({ start: 5, end: 9 });
    expect(parseRange("bytes=-4", 10)).toEqual({ start: 6, end: 9 });
    expect(parseRange("bytes=2-99", 10)).toEqual({ start: 2, end: 9 });
  });
  it("rejects / ignores", () => {
    expect(parseRange("bytes=10-", 10)).toBe("invalid");
    expect(parseRange("bytes=5-2", 10)).toBe("invalid");
    expect(parseRange("bytes=0-1,3-4", 10)).toBeNull();
    expect(parseRange("items=0-1", 10)).toBeNull();
  });
});

describe("range responses", async () => {
  const { req } = await fixture({ "a.txt": "0123456789", "tools/a..b.txt": "dots", ".routes.yml": "routes:\n  - from: /dots\n    to: /tools/a..b.txt\n    type: rewrite\n" });
  it("206 with Content-Range", async () => {
    const res = await req("/a.txt", { headers: { Range: "bytes=2-4" } });
    expect(res.status).toBe(206);
    expect(res.headers.get("Content-Range")).toBe("bytes 2-4/10");
    expect(await res.text()).toBe("234");
  });
  it("416 when unsatisfiable", async () => {
    const res = await req("/a.txt", { headers: { Range: "bytes=50-" } });
    expect(res.status).toBe(416);
  });
  it("a..b.txt is an allowed route target", async () => {
    const res = await req("/dots");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("dots");
  });
});
