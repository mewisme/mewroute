import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { load } from "./helpers.js";

const testdata = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../testdata");

// testdata/cases.json is shared with internal/router/cases_test.go so Go and Worker stay in lockstep.
const cases: { path: string; kind: string; file?: string; location?: string; status?: number }[] = JSON.parse(
  fs.readFileSync(path.join(testdata, "cases.json"), "utf8"),
);

describe("shared cases", async () => {
  const { r } = await load(testdata);
  for (const c of cases) {
    it(c.path, () => {
      const d = r(c.path);
      expect(d.kind).toBe(c.kind);
      if (c.file) expect(d).toMatchObject({ path: c.file });
      if (c.location) expect(d).toMatchObject({ location: c.location, status: c.status });
    });
  }
});
