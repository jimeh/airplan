import { expect, test } from "bun:test";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

test("Bun uses isolated installs with a seven-day dependency minimum", async () => {
  const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const source = await readFile(resolve(repositoryRoot, "bunfig.toml"), "utf8");
  const config = Bun.TOML.parse(source) as {
    install?: {
      linker?: string;
      minimumReleaseAge?: number;
      minimumReleaseAgeExcludes?: unknown[];
    };
  };

  expect(config.install?.linker).toBe("isolated");
  expect(config.install?.minimumReleaseAge).toBe(7 * 24 * 60 * 60);
  expect(config.install?.minimumReleaseAgeExcludes).toEqual([]);
});
