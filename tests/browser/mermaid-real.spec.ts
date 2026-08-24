import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

import { expect, test } from "@playwright/test";

import { binaryPath, cleanEnv, repoRoot } from "./airplan-binary.ts";

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const fixturePath = join(here, "testdata", "real-mermaid.md");
const manifestPath = join(repoRoot, "internal", "deps", "mermaid.json");
const packageJSONPath = join(repoRoot, "package.json");

let baseURL = "";
let mermaidDistRoot = "";
let mermaidDistURL = "";
let server: ReturnType<typeof createServer>;
let tempRoot = "";

test.beforeAll(async () => {
  if (!existsSync(binaryPath)) {
    throw new Error(
      `${binaryPath} is missing; tests/browser/global-setup.ts must build it before the suite`,
    );
  }

  const manifest = JSON.parse(await readFile(manifestPath, "utf8")) as {
    package: string;
    version: string;
  };
  const packageJSON = JSON.parse(await readFile(packageJSONPath, "utf8")) as {
    devDependencies?: Record<string, string>;
  };
  expect(manifest.package).toBe("mermaid");
  expect(packageJSON.devDependencies?.mermaid).toBe(manifest.version);

  mermaidDistRoot = resolve(repoRoot, "node_modules", "mermaid", "dist");
  mermaidDistURL = `https://cdn.jsdelivr.net/npm/mermaid@${manifest.version}/dist/`;

  tempRoot = await mkdtemp(join(tmpdir(), "airplan-real-mermaid-"));
  const outputPath = join(tempRoot, "index.html");
  await execFileAsync(
    binaryPath,
    ["preview", "--repo", "none", "--output", outputPath, fixturePath],
    { cwd: repoRoot, env: cleanEnv() },
  );
  const html = await readFile(outputPath);
  expect(html.toString()).toContain(`${mermaidDistURL}mermaid.esm.min.mjs`);

  server = createServer((request, response) => {
    if (request.url !== "/") {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    response.end(html);
  });
  await new Promise<void>((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address() as AddressInfo;
  baseURL = `http://127.0.0.1:${address.port}`;
});

test.afterAll(async () => {
  if (server) {
    await new Promise<void>((resolveClose, reject) => {
      server.close((error) => (error ? reject(error) : resolveClose()));
    });
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
});

test("renders the pinned Mermaid runtime and keeps diagrams interactive", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers the real runtime");

  const browserErrors: string[] = [];
  page.on("pageerror", (error) => browserErrors.push(`page error: ${error.message}`));
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.endsWith("/.airplan-versions.json")) return;
    if (response.status() >= 400) {
      browserErrors.push(`response error: ${response.status()} ${response.url()}`);
    }
  });
  page.on("console", (message) => {
    if (
      message.text() ===
      "Failed to load resource: the server responded with a status of 404 (Not Found)"
    ) {
      return;
    }
    if (message.type() === "error" || message.text().startsWith("airplan: Mermaid")) {
      browserErrors.push(`${message.type()}: ${message.text()}`);
    }
  });

  const mermaidDistPath = new URL(mermaidDistURL).pathname;
  await page.route(`${mermaidDistURL}**`, async (route) => {
    const requestURL = new URL(route.request().url());
    const relativePath = decodeURIComponent(requestURL.pathname.slice(mermaidDistPath.length));
    const localPath = resolve(mermaidDistRoot, relativePath);
    if (!localPath.startsWith(`${mermaidDistRoot}${sep}`)) {
      await route.abort();
      return;
    }
    await route.fulfill({
      contentType: "text/javascript; charset=utf-8",
      headers: { "Access-Control-Allow-Origin": "*" },
      path: localPath,
    });
  });

  await page.goto(baseURL);

  const diagrams = page.locator("pre.mermaid");
  await expect(diagrams).toHaveCount(3);
  await expect(diagrams.locator(":scope > svg")).toHaveCount(3);
  await expect(page.locator("pre.mermaid.mermaid-failed")).toHaveCount(0);

  for (const [index, labels] of [
    [0, ["Source", "Renderer", "Rendered plan"]],
    [1, ["Renderer", "Document", "render"]],
    [2, ["Reader", "Airplan", "Uploads and renders", "Reads rendered plans"]],
  ] as const) {
    const svg = diagrams.nth(index).locator(":scope > svg");
    for (const label of labels) await expect(svg).toContainText(label);
    const geometry = await svg.evaluate((element) => {
      const root = element as SVGSVGElement;
      return { height: root.viewBox.baseVal.height, width: root.viewBox.baseVal.width };
    });
    expect(geometry.width).toBeGreaterThan(0);
    expect(geometry.height).toBeGreaterThan(0);
  }

  const firstDiagram = diagrams.first().locator(":scope > svg");
  const lightSVG = await firstDiagram.evaluate((element) => element.outerHTML);
  const appearanceButton = page.getByRole("button", { name: "Appearance" });
  await appearanceButton.click();
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await expect.poll(() => firstDiagram.evaluate((element) => element.outerHTML)).not.toBe(lightSVG);
  await appearanceButton.click();
  await expect(page.locator("[data-airplan-appearance-panel]")).toBeHidden();

  await diagrams.first().hover();
  await diagrams.first().getByRole("button", { name: "Open diagram viewer" }).click();
  const viewer = page.locator("[data-airplan-mermaid-dialog]");
  await expect(viewer).toBeVisible();
  await expect(viewer.locator(".mermaid-surface > svg")).toContainText("Rendered plan");
  expect(browserErrors, "the real Mermaid runtime emitted browser errors").toEqual([]);
});
