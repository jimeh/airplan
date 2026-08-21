import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

import { expect, test as base } from "@playwright/test";

import { binaryPath, cleanEnv, fixtureBinaryPath, repoRoot } from "./airplan-binary.ts";

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const fixturePath = join(here, "testdata", "smoke.md");
const bundleFixtureRoot = join(here, "testdata", "bundle");
const sourceFixturePath = join(
  repoRoot,
  "airplan",
  "testdata",
  "TestRenderMarkdownGolden",
  "upload_example_go.html",
);
const expectedCode = "const answer = 42;\nconsole.log(answer);\n";
const mermaidModule = `
let theme = 'github-light';
let themeVariables = {};
function escapeHTML(value) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}
export default {
  initialize(config) {
    theme = config.themeVariables.airplanTheme;
    themeVariables = config.themeVariables;
  },
  async render(id, source) {
    const name = theme;
    const renderThemeVariables = { ...themeVariables };
    globalThis.__airplanMermaidRenders ||= {};
    globalThis.__airplanMermaidRenders[name] = (globalThis.__airplanMermaidRenders[name] || 0) + 1;
    if (globalThis.__airplanMermaidStartupTheme && !globalThis.__airplanMermaidStartupThemeSent) {
      globalThis.__airplanMermaidStartupThemeSent = true;
      setTimeout(() => window.dispatchEvent(new CustomEvent('airplan:themechange', {
        detail: {
          mode: 'light', resolvedMode: 'light',
          theme: globalThis.__airplanMermaidStartupTheme, variant: 'light',
        },
      })), 5);
    }
    if (globalThis.__airplanMermaidStartupPrint && !globalThis.__airplanMermaidStartupPrintSent) {
      globalThis.__airplanMermaidStartupPrintSent = true;
      setTimeout(() => window.dispatchEvent(new Event('beforeprint')), 5);
    }
    if (globalThis.__airplanMermaidDelay) {
      await new Promise((resolve) => setTimeout(resolve, globalThis.__airplanMermaidDelay));
    }
    if (name === 'tokyo-night') await new Promise((resolve) => setTimeout(resolve, 20));
    if (name === 'one-dark') throw new Error('intentional per-theme failure');
    if (globalThis.__airplanMermaidFailSource &&
        source.includes(globalThis.__airplanMermaidFailSource)) {
      throw new Error('intentional per-diagram failure');
    }
    return {
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="240"' +
        ' viewBox="0 0 640 240" role="img" data-mermaid-theme="' + name +
        '" id="' + id + '" aria-labelledby="' + id + '-title ' + id +
        '-label" aria-describedby="' + id + '-desc">' +
        '<title id="' + id + '-title">Diagram</title>' +
        '<desc id="' + id + '-desc">Rendered Mermaid fixture</desc>' +
        '<defs><marker id="' + id + '-arrow"><path fill="' + renderThemeVariables.lineColor +
        '" d="M0 0L10 5L0 10z"/>' +
        '</marker><linearGradient id="' + id + '-gradient"><stop offset="0"/>' +
        '</linearGradient></defs><style>#' + id + '-node{fill:url(#' + id +
        '-gradient)}#' + id + '-label{font-weight:600}#' + id +
        '-node-extra{opacity:.5}</style>' +
        '<g id="' + id + '-node" aria-labelledby="' + id + '-label">' +
        '<rect x="18" y="72" width="580" height="96" rx="10" fill="' +
        renderThemeVariables.primaryColor + '" stroke="' + renderThemeVariables.primaryBorderColor + '"/>' +
        '<path d="M20 120H600" stroke="' + renderThemeVariables.lineColor +
        '" marker-end="url(#' + id + '-arrow)"/>' +
        '<a href="#' + id + '-node"><text id="' + id + '-label" x="24"' +
        ' y="112" fill="' + renderThemeVariables.textColor + '">' +
        escapeHTML(source) + '</text></a></g></svg>',
    };
  },
};
`;
const legacyThemePage = `<!doctype html><html><body>
<button type="button" id="legacy-dark">Use legacy dark</button>
<script>document.querySelector('#legacy-dark').addEventListener('click', () => {
  localStorage.setItem('airplan-theme', 'dark');
});</script></body></html>`;

let baseURL = "";
let bundleURL = "";
let collectionURL = "";
let customURL = "";
let fixedURL = "";
let fixedCollectionURL = "";
let subsetURL = "";
let collectionMembers = new Map<string, Buffer>();
let fixtureSource = "";
let mermaidURL = "";
let server: ReturnType<typeof createServer>;
let sourceURL = "";
let tempRoot = "";
let collectionHTML = Buffer.alloc(0);
let customHTML = Buffer.alloc(0);
let fixedHTML = Buffer.alloc(0);
let fixedCollectionHTML = Buffer.alloc(0);
let subsetHTML = Buffer.alloc(0);
let revisionHTML = Buffer.alloc(0);
let revisionNotesHTML = Buffer.alloc(0);
let bundleMembers = new Map<string, Buffer>();
const versionRequests: Array<{
  headers: import("node:http").IncomingHttpHeaders;
  url: string;
}> = [];

function isVersionManifestURL(url: string) {
  try {
    return new URL(url).pathname.endsWith("/.airplan-versions.json");
  } catch {
    return false;
  }
}

function isRevisionMarkerURL(url: string) {
  try {
    return new URL(url).pathname.endsWith("/.airplan.json");
  } catch {
    return false;
  }
}

function linkedBundlePage(
  source: Buffer,
  revision: number,
  chainID: string,
  logicalPath: string,
  entrypoint: string,
) {
  const html = source.toString();
  const versions = '<meta name="airplan-versions" content="../.airplan-versions.json">';
  if (!html.includes(versions)) throw new Error("bundle fixture lacks versions metadata");
  return html.replace(
    versions,
    versions +
      `\n<meta name="airplan-revision" content="${revision}">` +
      `\n<meta name="airplan-revision-chain" content="${chainID}">` +
      `\n<meta name="airplan-page-path" content="${logicalPath}">` +
      `\n<meta name="airplan-entrypoint" content="${entrypoint}">`,
  );
}

const test = base.extend({
  page: async ({ page }, use) => {
    const errors: string[] = [];
    await page.route(mermaidURL, (route) =>
      route.fulfill({
        body: mermaidModule,
        contentType: "text/javascript; charset=utf-8",
      }),
    );
    page.on("pageerror", (error) => {
      errors.push(`page error: ${error.message}`);
    });
    page.on("response", (response) => {
      if (response.status() !== 404) return;
      if (isVersionManifestURL(response.url()) || isRevisionMarkerURL(response.url())) return;
      errors.push(`response error: 404 ${response.url()}`);
    });
    page.on("console", (message) => {
      if (message.type() === "error") {
        if (
          message.text() ===
          "Failed to load resource: the server responded with a status of 404 (Not Found)"
        ) {
          return;
        }
        errors.push(`console error: ${message.text()}`);
      }
    });

    await use(page);
    expect(errors, "the rendered page emitted browser errors").toEqual([]);
  },
});

test.beforeAll(async () => {
  if (!existsSync(binaryPath)) {
    throw new Error(
      `${binaryPath} is missing; it is built by ` +
        "tests/browser/global-setup.ts, which runs only when Playwright " +
        "uses this repository's playwright.config.ts",
    );
  }
  tempRoot = await mkdtemp(join(tmpdir(), "airplan-browser-"));
  fixtureSource = await readFile(fixturePath, "utf8");
  const outputPath = join(tempRoot, "index.html");
  const collectionOutputPath = join(tempRoot, "collection.html");
  const customOutputPath = join(tempRoot, "custom.html");
  const fixedOutputPath = join(tempRoot, "fixed.html");
  const fixedCollectionOutputPath = join(tempRoot, "fixed-collection.html");
  const subsetOutputPath = join(tempRoot, "subset.html");
  const revisionOutputPath = join(tempRoot, "revision.html");
  const bundleOutputPath = join(tempRoot, "bundle");
  const configRoot = join(tempRoot, "config");
  const env = cleanEnv();
  env.XDG_CONFIG_HOME = configRoot;

  // The binary is built once by the global setup; running it here keeps
  // this hook well inside its timeout even on a cold Go cache.
  await execFileAsync(
    binaryPath,
    ["preview", "--repo", "none", "--output", outputPath, fixturePath],
    { cwd: repoRoot, env },
  );
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--repo",
      "none",
      "--slug",
      "index",
      "--page",
      "docs/design.md",
      "--page",
      "examples/server.go",
      "--output-dir",
      bundleOutputPath,
      "README.md",
    ],
    { cwd: bundleFixtureRoot, env },
  );
  const subsetConfigPath = join(tempRoot, "subset-config", "airplan.toml");
  await mkdir(dirname(subsetConfigPath), { recursive: true });
  await writeFile(
    subsetConfigPath,
    `available_themes = ["tokyo-night", "one-dark"]
light_theme = "one-dark"
dark_theme = "tokyo-night"
`,
  );
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--config",
      subsetConfigPath,
      "--repo",
      "none",
      "--output",
      subsetOutputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  const fixedConfigPath = join(tempRoot, "fixed-config", "airplan.toml");
  await mkdir(dirname(fixedConfigPath), { recursive: true });
  await writeFile(fixedConfigPath, `theme = "tokyo-night"\n`);
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--config",
      fixedConfigPath,
      "--repo",
      "none",
      "--output",
      fixedOutputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  await execFileAsync(fixtureBinaryPath, [revisionOutputPath], {
    cwd: repoRoot,
    env,
  });
  await writeFile(
    join(tempRoot, "shot.svg"),
    '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">' +
      '<rect width="20" height="20" fill="green"/></svg>',
  );
  await writeFile(join(tempRoot, "demo.webm"), "video fixture");
  await writeFile(join(tempRoot, "sound.ogg"), "audio fixture");
  await writeFile(join(tempRoot, "notes.bin"), "generic fixture");
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--config",
      fixedConfigPath,
      "--files",
      "--repo",
      "none",
      "--output",
      fixedCollectionOutputPath,
      join(tempRoot, "notes.bin"),
    ],
    { cwd: repoRoot, env },
  );
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--files",
      "--repo",
      "none",
      "--title",
      "<Evidence & results>",
      "--output",
      collectionOutputPath,
      join(tempRoot, "shot.svg"),
      join(tempRoot, "demo.webm"),
      join(tempRoot, "sound.ogg"),
      join(tempRoot, "notes.bin"),
    ],
    { cwd: repoRoot, env },
  );
  const customConfigPath = join(tempRoot, "custom-config", "airplan.toml");
  await mkdir(dirname(customConfigPath), { recursive: true });
  await writeFile(
    customConfigPath,
    `light_theme = "custom-dark"
dark_theme = "github-light"

[themes.custom-dark]
name = "Custom Dark"
variant = "dark"
background = "#101010"
foreground = "#eeeeee"
muted = "#aaaaaa"
accent = "#6699ff"
accent_foreground = "#101010"
border = "#444444"
surface = "#202020"
surface_emphasis = "#303030"
info = "#55bbdd"
success = "#66bb77"
important = "#bb88ee"
warning = "#ddbb55"
danger = "#ee6677"
syntax = "derived"
`,
  );
  await execFileAsync(
    binaryPath,
    [
      "preview",
      "--config",
      customConfigPath,
      "--repo",
      "none",
      "--output",
      customOutputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  const html = await readFile(outputPath);
  collectionHTML = await readFile(collectionOutputPath);
  customHTML = await readFile(customOutputPath);
  fixedHTML = await readFile(fixedOutputPath);
  fixedCollectionHTML = await readFile(fixedCollectionOutputPath);
  subsetHTML = await readFile(subsetOutputPath);
  revisionHTML = await readFile(revisionOutputPath);
  revisionNotesHTML = await readFile(join(tempRoot, "notes.html"));
  bundleMembers = new Map([
    ["/bundle/index.html", await readFile(join(bundleOutputPath, "index.html"))],
    ["/bundle/docs/design.html", await readFile(join(bundleOutputPath, "docs", "design.html"))],
    [
      "/bundle/examples/server.go.html",
      await readFile(join(bundleOutputPath, "examples", "server.go.html")),
    ],
  ]);
  collectionMembers = new Map([
    ["/demo.webm", await readFile(join(tempRoot, "demo.webm"))],
    ["/sound.ogg", await readFile(join(tempRoot, "sound.ogg"))],
    ["/notes.bin", await readFile(join(tempRoot, "notes.bin"))],
  ]);
  const sourceHTML = await readFile(sourceFixturePath);
  const match = html.toString().match(/(https:\/\/[^"']+\/mermaid[^"']+\.mjs)/);
  if (!match) throw new Error("rendered fixture has no Mermaid module URL");
  [, mermaidURL] = match;

  server = createServer(async (request, response) => {
    let body;
    if (request.url === "/") {
      body = html;
    } else if (request.url === "/source") {
      body = sourceHTML;
    } else if (request.url === `/${"r".repeat(26)}/plan.html`) {
      body = revisionHTML;
    } else if (request.url === `/${"r".repeat(26)}/notes.html`) {
      body = revisionNotesHTML;
    } else if (request.url === `/${"r".repeat(26)}/.airplan-changes.diff`) {
      response.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
      response.end("--- revision-1/plan.md\n+++ revision-2/plan.md\n");
      return;
    } else if (request.url && /^\/[a-z2-7]{26}\/plan\.html$/.test(request.url)) {
      body = html;
    } else if (request.url && bundleMembers.has(request.url)) {
      body = bundleMembers.get(request.url);
      response.writeHead(200, {
        "Cache-Control": "no-store",
        "Content-Type": "text/html; charset=utf-8",
      });
      response.end(body);
      return;
    } else if (request.url === "/collection") {
      body = collectionHTML;
    } else if (request.url === "/custom") {
      body = customHTML;
    } else if (request.url === "/subset") {
      body = subsetHTML;
    } else if (request.url === "/fixed") {
      body = fixedHTML;
    } else if (request.url === "/fixed-collection") {
      body = fixedCollectionHTML;
    } else if (request.url === "/legacy-theme") {
      body = Buffer.from(legacyThemePage);
    } else if (request.url === "/shot.svg") {
      body = await readFile(join(tempRoot, "shot.svg"));
      response.writeHead(200, { "Content-Type": "image/svg+xml" });
      response.end(body);
      return;
    } else if (request.url && collectionMembers.has(request.url)) {
      body = collectionMembers.get(request.url);
      response.writeHead(200, { "Content-Type": "application/octet-stream" });
      response.end(body);
      return;
    } else if (request.url?.startsWith("/.airplan-versions.json?")) {
      versionRequests.push({ url: request.url, headers: request.headers });
      response.writeHead(404).end();
      return;
    } else {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    response.end(body);
  });
  await new Promise<void>((resolve, reject) => {
    server!.once("error", reject);
    server!.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address() as AddressInfo;
  baseURL = `http://127.0.0.1:${address.port}`;
  bundleURL = `${baseURL}/bundle/index.html`;
  collectionURL = `${baseURL}/collection`;
  customURL = `${baseURL}/custom`;
  subsetURL = `${baseURL}/subset`;
  fixedURL = `${baseURL}/fixed`;
  fixedCollectionURL = `${baseURL}/fixed-collection`;
  sourceURL = `${baseURL}/source`;
});

test("bundle pages use ordinary navigation and update both rails", async ({
  context,
  page,
}, testInfo) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: baseURL });
  await page.goto(bundleURL);
  await expect(page).toHaveTitle("Bundle overview");
  await expect(page.locator("pre.mermaid")).toHaveCount(0);

  const inlinePages = page.locator(".page-shell > .pages-nav");
  const openPages = page.getByRole("button", { name: "Open pages" });
  const pagesTrigger = page.locator(".pages-trigger");
  await expect(pagesTrigger).toHaveAttribute("aria-controls", "pages-popover");
  await expect(pagesTrigger).toHaveAttribute("popovertarget", "");
  expect(await pagesTrigger.getAttribute("aria-haspopup")).toBeNull();
  expect(
    await pagesTrigger.evaluate(
      (trigger) => (trigger as HTMLButtonElement).popoverTargetElement?.id,
    ),
  ).toBe("pages-popover");
  if (testInfo.project.name.startsWith("narrow-")) {
    await expect(inlinePages).toBeHidden();
    await expect(openPages).toBeVisible();
  } else {
    await expect(inlinePages).toBeVisible();
    await expect(openPages).toBeHidden();
    const columns = await page.evaluate(() => {
      const pages = document.querySelector(".pages-nav")!.getBoundingClientRect();
      const content = document.querySelector(".content")!.getBoundingClientRect();
      const toc = document.querySelector(".toc")!.getBoundingClientRect();
      return {
        contentLeft: content.left,
        contentRight: content.right,
        pagesRight: pages.right,
        tocLeft: toc.left,
      };
    });
    expect(columns.pagesRight).toBeLessThanOrEqual(columns.contentLeft);
    expect(columns.tocLeft).toBeGreaterThanOrEqual(columns.contentRight);
  }

  await expect(inlinePages.locator('a[aria-current="page"]')).toContainText("README.md");
  await expect(page.locator("#toc .rail-title")).toHaveText("On this page");
  await expect(page.locator(".page-sequence-next")).toContainText("Design notes");
  const entryLifetime = await page.evaluate(
    () => (window as typeof window & { __airplanBundleLifetime?: string }).__airplanBundleLifetime,
  );
  expect(entryLifetime).toBeTruthy();

  if (testInfo.project.name.startsWith("narrow-")) {
    await openPages.click();
    const popover = page.locator("#pages-popover");
    await expect(popover).toBeVisible();
    await expect(popover.locator('a[aria-current="page"]')).toContainText("README.md");
    await expect(openPages).toHaveAttribute("aria-expanded", "true");
    await openPages.click();
    await expect(popover).toBeHidden();
    await openPages.click();
    await page.keyboard.press("Escape");
    await expect(popover).toBeHidden();
    await expect(openPages).toBeFocused();
    await openPages.click();
    await page.mouse.click(380, 820);
    await expect(popover).toBeHidden();
    await openPages.click();

    await page.setViewportSize({ width: 1400, height: 844 });
    await expect(popover).toBeHidden();
    await page.setViewportSize({ width: 390, height: 844 });
    await openPages.click();
    await expect(popover).toBeVisible();
    await Promise.all([
      page.waitForURL(`${baseURL}/bundle/docs/design.html`),
      popover.getByRole("link", { name: /Design notes/ }).click(),
    ]);
  } else {
    await Promise.all([
      page.waitForURL(`${baseURL}/bundle/docs/design.html`),
      inlinePages.getByRole("link", { name: /Design notes/ }).click(),
    ]);
  }

  await expect(page).toHaveTitle("Design notes");
  const designLifetime = await page.evaluate(
    () => (window as typeof window & { __airplanBundleLifetime?: string }).__airplanBundleLifetime,
  );
  expect(designLifetime).toBeTruthy();
  expect(designLifetime).not.toBe(entryLifetime);
  await expect
    .poll(() => page.evaluate(() => sessionStorage.getItem("airplan-bundle-authored-runs")))
    .toBe("2");
  await expect(page.locator(".page-shell > .pages-nav a[aria-current=page]")).toContainText(
    "docs/design.md",
  );
  await expect(page.locator("pre.mermaid > svg")).toHaveCount(1);
  await expect(page.locator(".page-sequence-previous")).toContainText("Bundle overview");
  await expect(page.locator(".page-sequence-next")).toContainText("server.go");

  const sourceButton = page.getByRole("button", { name: "Source view" });
  await sourceButton.click();
  await expect(page.locator("#source")).toBeVisible();
  await page.getByRole("button", { name: "Copy markdown" }).click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("# Design notes");
  await page.getByRole("button", { name: "Rendered view" }).click();

  if (testInfo.project.name.startsWith("narrow-")) {
    await page.getByRole("button", { name: "Open table of contents" }).click();
    await page.locator("#toc-dialog").getByRole("link", { name: "Deep dive" }).click();
  } else {
    await page.locator("#toc").getByRole("link", { name: "Deep dive" }).click();
  }
  await expect(page).toHaveURL(`${baseURL}/bundle/docs/design.html#deep-dive`);
  await page.goBack();
  await expect(page).toHaveURL(`${baseURL}/bundle/docs/design.html`);
  await page.goBack();
  await expect(page).toHaveURL(bundleURL);
  await page.goForward();
  await expect(page).toHaveURL(`${baseURL}/bundle/docs/design.html`);
});

test("collapsed bundle navigation stays sticky with Pages on the left", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers the medium breakpoint");

  await page.setViewportSize({ width: 1000, height: 640 });
  await page.goto(`${baseURL}/bundle/docs/design.html`);

  const toolbar = page.getByRole("navigation", { name: "Document controls" });
  const pages = page.getByRole("button", { name: "Open pages" });
  await expect(page.locator(".page-shell > .pages-nav")).toBeHidden();
  await expect(pages).toBeVisible();
  const layout = await toolbar.evaluate((element) => {
    const styles = getComputedStyle(element);
    const trigger = element.querySelector(".pages-trigger")!.getBoundingClientRect();
    const bounds = element.getBoundingClientRect();
    return {
      position: styles.position,
      left: trigger.left - bounds.left,
      leftPadding: Number.parseFloat(styles.paddingLeft),
    };
  });
  expect(layout.position).toBe("sticky");
  expect(layout.left).toBeCloseTo(layout.leftPadding, 0);

  await page.evaluate(() => window.scrollTo(0, 320));
  await expect
    .poll(() => toolbar.evaluate((element) => element.getBoundingClientRect().top))
    .toBeCloseTo(0, 0);
  const stickyHeight = await page
    .locator("html")
    .evaluate((element) =>
      Number.parseFloat(getComputedStyle(element).getPropertyValue("--airplan-sticky-height")),
    );
  expect(stickyHeight).toBeCloseTo(
    await toolbar.evaluate((element) => (element as HTMLElement).offsetHeight),
    0,
  );
});

test("wide rail headings remain outside their scroll areas", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one wide project covers rail structure");

  await page.setViewportSize({ width: 1400, height: 240 });
  await page.goto(`${baseURL}/bundle/docs/design.html`);
  const pagesBounds = await page.locator(".pages-nav").boundingBox();
  const tocBounds = await page.locator(".toc").boundingBox();
  expect(pagesBounds).not.toBeNull();
  expect(tocBounds).not.toBeNull();
  expect(pagesBounds!.x + pagesBounds!.width).toBeLessThanOrEqual(tocBounds!.x);
  for (const selector of [".pages-nav", ".toc"]) {
    const rail = page.locator(selector);
    const result = await rail.evaluate((element) => {
      const title = element.querySelector<HTMLElement>(".rail-title")!;
      const list = element.querySelector<HTMLOListElement>("ol")!;
      const before = title.getBoundingClientRect().top;
      list.scrollTop = 48;
      return {
        railOverflow: getComputedStyle(element).overflowY,
        listOverflow: getComputedStyle(list).overflowY,
        titleTopBefore: before,
        titleTopAfter: title.getBoundingClientRect().top,
      };
    });
    expect(result.railOverflow).toBe("hidden");
    expect(result.listOverflow).toBe("auto");
    expect(result.titleTopAfter).toBeCloseTo(result.titleTopBefore, 0);
  }
});

test("bundle navigation keeps its narrow no-JavaScript fallback", async ({ browser }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers the no-JS fallback");
  const context = await browser.newContext({
    colorScheme: "light",
    javaScriptEnabled: false,
    viewport: { width: 390, height: 844 },
  });
  try {
    const page = await context.newPage();
    await page.goto(bundleURL);
    const pages = page.locator(".page-shell > .pages-nav");
    await expect(pages).toBeVisible();
    await expect(page.getByRole("button", { name: "Open pages" })).toBeHidden();
    await expect(pages.locator('a[aria-current="page"]')).toContainText("README.md");
    await pages.getByRole("link", { name: /Design notes/ }).click();
    await expect(page).toHaveURL(`${baseURL}/bundle/docs/design.html`);
    await expect(page.locator(".page-shell > .pages-nav a[aria-current=page]")).toContainText(
      "docs/design.md",
    );
  } finally {
    await context.close();
  }
});

test("built-in page transitions honor reduced motion", async ({ page }) => {
  await page.goto(bundleURL);
  const compactCSS = await page.locator("style").evaluateAll((styles) =>
    styles
      .map((style) => style.textContent || "")
      .join("\n")
      .replaceAll(/\s/g, ""),
  );
  expect(compactCSS).toContain(
    "@media(prefers-reduced-motion:no-preference){@view-transition{navigation:auto;}}",
  );
  expect(
    await page.evaluate(() => matchMedia("(prefers-reduced-motion:no-preference)").matches),
  ).toBe(true);
  await page.emulateMedia({ reducedMotion: "reduce" });
  expect(
    await page.evaluate(() => matchMedia("(prefers-reduced-motion:no-preference)").matches),
  ).toBe(false);
});

test("bundle print output contains only the loaded page", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers bundle printing");
  await page.goto(`${baseURL}/bundle/docs/design.html`);
  const detail = page.locator("#bundle-print-detail");
  await expect(detail).not.toHaveAttribute("open", "");
  await page.emulateMedia({ media: "print" });
  await expect(detail.getByText("Printed bundle content")).toBeVisible();
  await expect(page.locator(".pages-nav")).toBeHidden();
  await expect(page.locator(".page-sequence")).toBeHidden();
  await expect(page.locator(".pages-trigger")).toBeHidden();
});

test("collection overview presents and links every media kind", async ({ context, page }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: baseURL });
  await page.goto(collectionURL);
  await expect(page).toHaveTitle("<Evidence & results>");
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "<Evidence & results>",
    }),
  ).toBeVisible();
  await expect(page.locator("ol.files > li.file")).toHaveCount(4);
  await expect(page.locator('img[loading="lazy"]')).toHaveCount(1);
  const imageFile = page.locator(".file", {
    has: page.getByRole("heading", { name: "shot.svg" }),
  });
  const imagePreview = imageFile.locator(".preview--image > a");
  await expect(imagePreview).toHaveAttribute("href", "./shot.svg");
  await expect(imageFile.getByRole("link", { name: "Open" })).toHaveAttribute("href", "./shot.svg");
  await expect(imagePreview).toHaveAccessibleName("shot.svg");
  await expect(page.locator("video[controls]:not([autoplay])")).toHaveCount(1);
  await expect(page.locator("audio[controls]:not([autoplay])")).toHaveCount(1);
  await expect(page.locator(".file .preview")).toHaveCount(3);
  const visualMediaGaps = await page.locator(".preview img, .preview video").evaluateAll((media) =>
    media.map((element) => {
      const frame = element.closest(".preview")!.getBoundingClientRect();
      const content = element.getBoundingClientRect();
      return {
        top: content.top - frame.top,
        right: frame.right - content.right,
        bottom: frame.bottom - content.bottom,
        left: content.left - frame.left,
      };
    }),
  );
  for (const gaps of visualMediaGaps) {
    expect(gaps).toEqual({ top: 0, right: 0, bottom: 0, left: 0 });
  }
  await expect(page.locator(".preview img")).toHaveCSS("max-height", "none");
  await expect(page.locator(".preview video")).toHaveCSS("object-fit", "contain");
  await expect(page.locator(".preview--audio")).toHaveCSS("border-top-width", "1px");
  await expect(
    page
      .locator(".file", {
        has: page.getByRole("heading", { name: "notes.bin" }),
      })
      .locator(".preview"),
  ).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "notes.bin" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open" })).toHaveCount(4);
  await expect(page.getByRole("link", { name: "Download" })).toHaveCount(4);
  await page.keyboard.press("Tab");
  const overviewCopy = page.getByRole("button", {
    name: "Copy page link",
  });
  await expect(overviewCopy).toBeFocused();
  await expect(overviewCopy).toHaveCSS("outline-style", "solid");
  await expect(overviewCopy).toHaveCSS("outline-width", "2px");
  const fileCopy = page.locator('[data-copy="./notes.bin"]');
  await fileCopy.click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(`${baseURL}/notes.bin`);
  await expect(fileCopy).toHaveText("Copied");
  await page.waitForTimeout(300);
  await fileCopy.click();
  await page.waitForTimeout(1000);
  await expect(fileCopy).toHaveText("Copied");
  await page.waitForTimeout(300);
  await expect(fileCopy).toHaveText("Copy link");
  await overviewCopy.click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(collectionURL);
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
});

test("collection overview shares document theme controls", async ({ page }) => {
  await page.goto(collectionURL);
  const root = page.locator("html");
  await page.getByRole("button", { name: "Appearance" }).click();
  const lightTheme = page.getByRole("button", { name: "Light", exact: true });
  const systemTheme = page.getByRole("button", { name: "System", exact: true });
  const darkTheme = page.getByRole("button", { name: "Dark", exact: true });

  await expect(systemTheme).toHaveAttribute("aria-pressed", "true");
  await expect(root).toHaveAttribute("data-airplan-mode", "system");

  await darkTheme.click();
  await expect(root).toHaveAttribute("data-airplan-mode", "dark");
  await expect(darkTheme).toHaveAttribute("aria-pressed", "true");
  await expect.poll(() => page.evaluate(() => localStorage.getItem("airplan-theme"))).toBe("dark");

  await page.reload();
  await expect(root).toHaveAttribute("data-airplan-mode", "dark");
  await page.getByRole("button", { name: "Appearance" }).click();
  await expect(darkTheme).toHaveAttribute("aria-pressed", "true");

  await lightTheme.click();
  await expect(root).toHaveAttribute("data-airplan-mode", "light");
  await expect(lightTheme).toHaveAttribute("aria-pressed", "true");

  await systemTheme.click();
  await expect(root).toHaveAttribute("data-airplan-mode", "system");
  await expect(systemTheme).toHaveAttribute("aria-pressed", "true");
  await expect.poll(() => page.evaluate(() => localStorage.getItem("airplan-theme"))).toBeNull();
});

test("appearance panel groups the full catalog and allows cross-variant slots", async ({
  page,
}, testInfo) => {
  await page.goto(baseURL);
  const root = page.locator("html");
  const trigger = page.getByRole("button", { name: "Appearance" });
  await trigger.click();
  const panel = page.getByRole("group", { name: "Appearance settings" });
  await expect(panel).toBeVisible();
  const lightSelect = page.getByRole("combobox", { name: "Light theme", exact: true });
  const darkSelect = page.getByRole("combobox", { name: "Dark theme", exact: true });
  const options = async (select: typeof lightSelect) =>
    select.locator("option").evaluateAll((nodes) =>
      nodes.map((node) => ({
        value: (node as HTMLOptionElement).value,
        label: node.textContent,
      })),
    );
  expect(await options(lightSelect)).toEqual(await options(darkSelect));
  await expect(lightSelect.locator("optgroup")).toHaveCount(2);
  await expect(lightSelect.locator('optgroup[label="Light themes"] option')).toHaveCount(5);
  await expect(lightSelect.locator('optgroup[label="Dark themes"] option')).toHaveCount(6);

  await page.getByRole("button", { name: "Light", exact: true }).click();
  await lightSelect.selectOption("one-dark");
  await expect(root).toHaveAttribute("data-airplan-resolved-mode", "light");
  await expect(root).toHaveAttribute("data-airplan-theme", "one-dark");
  await expect(root).toHaveAttribute("data-airplan-theme-variant", "dark");
  await expect(root).toHaveCSS("color-scheme", "dark");

  await darkSelect.selectOption("tokyo-night-day");
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await expect(root).toHaveAttribute("data-airplan-resolved-mode", "dark");
  await expect(root).toHaveAttribute("data-airplan-theme", "tokyo-night-day");
  await expect(root).toHaveAttribute("data-airplan-theme-variant", "light");
  await expect(root).toHaveCSS("color-scheme", "light");
  await page.reload();
  await expect(root).toHaveAttribute("data-airplan-theme", "tokyo-night-day");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("airplan-light-theme")))
    .toBe("one-dark");

  await page.evaluate(() => localStorage.setItem("airplan-light-theme", "custom-on-another-page"));
  await page.getByRole("button", { name: "Appearance" }).click();
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await page.reload();
  await expect(root).toHaveAttribute("data-airplan-theme", "github-light");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("airplan-light-theme")))
    .toBe("custom-on-another-page");

  if (testInfo.project.name.startsWith("narrow-")) {
    await trigger.click();
    const box = await panel.boundingBox();
    if (!box) throw new Error("appearance panel has no bounds");
    const viewport = page.viewportSize();
    if (!viewport) throw new Error("appearance panel test has no viewport");
    expect(box.x).toBeGreaterThanOrEqual(8);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width - 8);
  }
  if (testInfo.project.name === "desktop-light" || testInfo.project.name === "narrow-light") {
    if (!(await panel.isVisible())) await trigger.click();
    await page.screenshot({ path: testInfo.outputPath("appearance-panel.png"), fullPage: true });
  }
});

test("configured subset is the only grouped appearance catalog", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers catalog filtering");
  await page.goto(subsetURL);
  await page.getByRole("button", { name: "Appearance" }).click();
  const light = page.getByRole("combobox", { name: "Light theme", exact: true });
  const dark = page.getByRole("combobox", { name: "Dark theme", exact: true });
  for (const select of [light, dark]) {
    await expect(select.locator('optgroup[label="Light themes"]')).toHaveCount(0);
    await expect(select.locator('optgroup[label="Dark themes"] option')).toHaveCount(2);
    await expect(select.locator("option")).toHaveText(["Tokyo Night", "One Dark"]);
  }
});

test("forced theme ignores stored preferences and omits appearance controls", async ({
  browser,
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers fixed pages");
  await page.addInitScript(() => {
    localStorage.setItem("airplan-color-mode", "light");
    localStorage.setItem("airplan-light-theme", "github-light");
    localStorage.setItem("airplan-dark-theme", "github-dark");
  });
  for (const url of [fixedURL, fixedCollectionURL]) {
    await page.goto(url);
    await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", "tokyo-night");
    await expect(page.getByRole("button", { name: "Appearance" })).toHaveCount(0);
  }
  await page.goto(fixedURL);
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "tokyo-night",
  );
  await page.emulateMedia({ media: "print" });
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-light",
  );
  const noJS = await browser.newContext({ colorScheme: "light", javaScriptEnabled: false });
  try {
    const noJSPage = await noJS.newPage();
    await noJSPage.goto(fixedURL);
    await expect(noJSPage.getByRole("button", { name: "Appearance" })).toHaveCount(0);
    await expect(noJSPage.locator("body")).toHaveCSS("background-color", "rgb(26, 27, 38)");
  } finally {
    await noJS.close();
  }
});

test("appearance controls pair mode labels with icons and align custom select chevrons", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "narrow-light", "one narrow project covers appearance UI");
  await page.goto(baseURL);
  const trigger = page.getByRole("button", { name: "Appearance" });
  await expect(trigger.locator('[data-airplan-resolved-icon="light"]')).toBeVisible();
  await expect(trigger.locator('[data-airplan-resolved-icon="dark"]')).toBeHidden();

  await trigger.click();
  for (const name of ["System", "Light", "Dark"]) {
    await expect(page.getByRole("button", { name, exact: true }).locator("svg.icon")).toHaveCount(
      1,
    );
  }
  const lightSelect = page.getByRole("combobox", { name: "Light theme", exact: true });
  const darkSelect = page.getByRole("combobox", { name: "Dark theme", exact: true });
  await expect(lightSelect).toBeVisible();
  await expect(darkSelect).toBeVisible();
  await expect(lightSelect).toHaveCSS("appearance", "none");

  const selectGeometry = await lightSelect.evaluate((select) => {
    const icon = select.parentElement!.querySelector<SVGElement>("[data-airplan-select-icon]")!;
    const selectBox = select.getBoundingClientRect();
    const iconBox = icon.getBoundingClientRect();
    return {
      endInset: selectBox.right - iconBox.right,
      centerOffset: Math.abs(
        selectBox.top + selectBox.height / 2 - (iconBox.top + iconBox.height / 2),
      ),
      pointerEvents: getComputedStyle(icon).pointerEvents,
    };
  });
  expect(selectGeometry.endInset).toBeGreaterThanOrEqual(8);
  expect(selectGeometry.endInset).toBeLessThanOrEqual(16);
  expect(selectGeometry.centerOffset).toBeLessThanOrEqual(1);
  expect(selectGeometry.pointerEvents).toBe("none");

  await page.emulateMedia({ colorScheme: "dark" });
  await expect(trigger.locator('[data-airplan-resolved-icon="light"]')).toBeHidden();
  await expect(trigger.locator('[data-airplan-resolved-icon="dark"]')).toBeVisible();
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await expect(trigger.locator('[data-airplan-resolved-icon="light"]')).toBeVisible();
  await expect(trigger.locator('[data-airplan-resolved-icon="dark"]')).toBeHidden();
  await lightSelect.selectOption("one-dark");
  await expect(trigger.locator('[data-airplan-resolved-icon="light"]')).toBeVisible();
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await expect(trigger.locator('[data-airplan-resolved-icon="light"]')).toBeHidden();
  await expect(trigger.locator('[data-airplan-resolved-icon="dark"]')).toBeVisible();
});

test("appearance panel dismisses accessibly and restores focus", async ({ page }) => {
  await page.goto(baseURL);
  const trigger = page.getByRole("button", { name: "Appearance" });
  const panel = page.getByRole("group", { name: "Appearance settings" });
  await trigger.click();
  await expect(panel).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("appearance dismissal test has no viewport");
  await page.mouse.click(4, viewport.height - 4);
  await expect(panel).toBeHidden();
  await expect(trigger).toBeFocused();

  if (viewport.width <= 768) return;
  await trigger.click();
  const sourceView = page.getByRole("button", { name: "Source view" });
  await sourceView.click();
  await expect(panel).toBeHidden();
  await expect(sourceView).toBeFocused();
});

test("mode follows system changes and legacy state cannot clobber new state", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers storage migration");
  await page.addInitScript(() => localStorage.setItem("airplan-theme", "dark"));
  await page.goto(baseURL);
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "dark");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("airplan-color-mode")))
    .toBe("dark");
  await page.getByRole("button", { name: "Appearance" }).click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).toHaveAttribute("data-airplan-resolved-mode", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-airplan-resolved-mode", "light");
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await page.goto(`${baseURL}/legacy-theme`);
  await page.getByRole("button", { name: "Use legacy dark" }).click();
  await page.goto(baseURL);
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "light");
});

test("custom uploader themes drive no-JS defaults and derived syntax", async ({
  browser,
  page,
}, testInfo) => {
  await page.goto(customURL);
  const expectedTheme = testInfo.project.name.endsWith("-dark") ? "github-light" : "custom-dark";
  await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", expectedTheme);
  if (expectedTheme === "custom-dark") {
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(16, 16, 16)");
    await expect(page.locator(".chroma .k, .chroma .kd").first()).toHaveCSS(
      "color",
      "rgb(102, 153, 255)",
    );
  }
  if (testInfo.project.name === "desktop-light") {
    await page.screenshot({ path: testInfo.outputPath("custom-theme.png"), fullPage: true });
  }
  await page.getByRole("button", { name: "Appearance" }).click();
  await expect(
    page
      .getByRole("combobox", { name: "Light theme", exact: true })
      .locator('option[value="custom-dark"]'),
  ).toHaveText("Custom Dark");

  const noJS = await browser.newContext({
    colorScheme: testInfo.project.name.endsWith("-dark") ? "dark" : "light",
    javaScriptEnabled: false,
  });
  try {
    const noJSPage = await noJS.newPage();
    await noJSPage.goto(customURL);
    await expect(noJSPage.getByRole("button", { name: "Appearance" })).toHaveCount(0);
    await expect(noJSPage.locator("body")).toHaveCSS(
      "background-color",
      testInfo.project.name.endsWith("-dark") ? "rgb(255, 255, 255)" : "rgb(16, 16, 16)",
    );
  } finally {
    await noJS.close();
  }
});

test("every built-in theme produces a coherent live palette", async ({ page }, testInfo) => {
  await page.goto(baseURL);
  await page.getByRole("button", { name: "Appearance" }).click();
  await page.getByRole("button", { name: "Light", exact: true }).click();
  const select = page.getByRole("combobox", { name: "Light theme", exact: true });
  const catalog = await select.locator("option").evaluateAll((nodes) =>
    nodes.map((node) => ({
      id: (node as HTMLOptionElement).value,
      variant:
        (node.parentElement as HTMLOptGroupElement | null)?.label === "Dark themes"
          ? "dark"
          : "light",
    })),
  );
  expect(catalog).toHaveLength(11);
  for (const theme of catalog) {
    await select.selectOption(theme.id);
    await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", theme.id);
    await expect(page.locator("html")).toHaveCSS("color-scheme", theme.variant);
    const colors = await page.locator("body").evaluate((body) => {
      const styles = getComputedStyle(body);
      return [styles.backgroundColor, styles.color];
    });
    expect(colors[0]).not.toBe(colors[1]);
    expect(colors[0]).not.toBe("rgba(0, 0, 0, 0)");
    if (testInfo.project.name === "desktop-light") {
      await page.screenshot({ path: testInfo.outputPath(`theme-${theme.id}.png`), fullPage: true });
    }
  }
});

test("built-in pages share canonical toolbar control styling", async ({ page }) => {
  const controlStyles = async () =>
    page.evaluate(() => {
      const toolbar = document.querySelector<HTMLElement>(".toolbar")!;
      const toggle = toolbar.querySelector<HTMLElement>(".appearance")!;
      const button = toggle.querySelector<HTMLButtonElement>("[data-airplan-appearance-trigger]")!;
      const icon = button.querySelector<SVGElement>(".icon")!;
      const action = toolbar.querySelector<HTMLButtonElement>(".toolbar-actions button")!;
      const toolbarStyle = getComputedStyle(toolbar);
      const toggleStyle = getComputedStyle(toggle);
      const buttonStyle = getComputedStyle(button);
      const iconStyle = getComputedStyle(icon);
      const actionStyle = getComputedStyle(action);
      return {
        toolbarWidth: toolbar.getBoundingClientRect().width,
        themeRight: window.innerWidth - toggle.getBoundingClientRect().right,
        toolbarPaddingLeft: toolbarStyle.paddingLeft,
        toolbarPaddingRight: toolbarStyle.paddingRight,
        toolbarGap: toolbarStyle.gap,
        toggleHeight: toggle.getBoundingClientRect().height,
        toggleGap: toggleStyle.gap,
        togglePadding: toggleStyle.padding,
        toggleRadius: toggleStyle.borderRadius,
        buttonWidth: button.getBoundingClientRect().width,
        buttonHeight: button.getBoundingClientRect().height,
        buttonPadding: buttonStyle.padding,
        buttonRadius: buttonStyle.borderRadius,
        iconWidth: iconStyle.width,
        iconHeight: iconStyle.height,
        actionHeight: action.getBoundingClientRect().height,
        actionPadding: actionStyle.padding,
        actionRadius: actionStyle.borderRadius,
        actionDisplay: actionStyle.display,
        actionAlignItems: actionStyle.alignItems,
        actionJustifyContent: actionStyle.justifyContent,
        actionGap: actionStyle.gap,
        actionColor: actionStyle.color,
        actionBackground: actionStyle.backgroundColor,
        actionFontSize: actionStyle.fontSize,
        actionLineHeight: actionStyle.lineHeight,
      };
    });

  await page.goto(baseURL);
  const documentStyles = await controlStyles();
  await page.goto(collectionURL);
  expect(await controlStyles()).toEqual(documentStyles);
});

test("toolbar controls do not transition during theme changes", async ({ page }) => {
  for (const url of [baseURL, collectionURL]) {
    await page.goto(url);
    const controls = page.locator(".toolbar a, .toolbar button");
    await expect
      .poll(async () =>
        controls.evaluateAll((elements) =>
          elements.every((element) => getComputedStyle(element).transitionDuration === "0s"),
        ),
      )
      .toBe(true);
    await page.getByRole("button", { name: "Appearance" }).click();
    await page.getByRole("button", { name: "Dark", exact: true }).click();
    await expect
      .poll(async () =>
        controls.evaluateAll((elements) =>
          elements.every((element) => getComputedStyle(element).transitionDuration === "0s"),
        ),
      )
      .toBe(true);
  }
});

test.afterAll(async () => {
  if (server) {
    await new Promise<void>((resolve, reject) => {
      server!.close((error) => (error ? reject(error) : resolve()));
    });
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
});

test("standalone Markdown performs cache-busted dormant revision discovery", async ({ page }) => {
  const start = versionRequests.length;
  await page.goto(baseURL);
  await expect.poll(() => versionRequests.length).toBe(start + 1);
  await page.reload();
  await expect.poll(() => versionRequests.length).toBe(start + 2);

  const requests = versionRequests.slice(start);
  const nonces = requests.map(({ url }) => new URL(url, baseURL).searchParams.get("_airplan"));
  expect(nonces[0]).toBeTruthy();
  expect(nonces[1]).toBeTruthy();
  expect(nonces[1]).not.toBe(nonces[0]);
  for (const request of requests) {
    expect(request.headers["cache-control"]).toContain("no-cache");
  }
});

test("revision metadata renders a compact picker and stale notice", async ({ page }) => {
  const dirs = ["a".repeat(26), "b".repeat(26), "c".repeat(26)];
  const revisions = dirs.map((dir, index) => ({
    number: index + 1,
    url: `${baseURL}/${dir}/plan.html`,
    created_at: `2026-08-15T10:${String(index * 10).padStart(2, "0")}:00Z`,
    ...(index === 0
      ? {}
      : {
          diff_url: `${baseURL}/${dir}/.airplan-changes.diff`,
        }),
  }));
  await page.route("**/.airplan-versions.json?*", (route) => {
    const requestURL = new URL(route.request().url());
    const currentDir = requestURL.pathname.split("/").at(-2);
    const currentRevision = dirs.indexOf(currentDir || "") + 1;
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        schema: "airplan-versions",
        version: 1,
        chain_id: "d".repeat(26),
        current_revision: currentRevision,
        latest_revision: 3,
        last_assigned_revision: 3,
        revisions,
      }),
    });
  });
  await page.goto(`${baseURL}/${dirs[0]}/plan.html`);
  const picker = page.getByRole("combobox", { name: "Document revision" });
  await expect(picker).toHaveValue(revisions[0].url);
  await expect(picker.locator("option")).toHaveCount(3);
  await expect(picker.locator("option")).toHaveText([
    "Revision 1 of 3",
    "Revision 2 of 3",
    "Revision 3 (Latest)",
  ]);
  await expect(page.getByRole("link", { name: "Previous" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Next" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /Latest: revision/ })).toHaveCount(0);
  await expect(page.locator(".toolbar").getByRole("combobox")).toHaveCount(0);
  const heading = page.locator("[data-revision-heading]");
  await expect(heading.getByRole("combobox")).toBeVisible();
  await expect(page.locator(".revision-picker-label")).toHaveText("Revision 1 of 3");
  await expect(page.locator(".revision-picker-label")).toHaveAttribute("aria-hidden", "true");
  await expect(heading).toHaveClass(/is-stale/);
  const coverage = await heading.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const select = element.querySelector("select")!.getBoundingClientRect();
    const chevron = getComputedStyle(element, "::after");
    return {
      top: select.top - bounds.top,
      right: bounds.right - select.right,
      bottom: bounds.bottom - select.bottom,
      left: select.left - bounds.left,
      chevronWidth: Number.parseFloat(chevron.width),
      width: bounds.width,
    };
  });
  for (const edge of ["top", "right", "bottom", "left"] as const) {
    expect(coverage[edge]).toBeCloseTo(0, 0);
  }
  expect(coverage.chevronWidth).toBeCloseTo(6, 0);
  expect(coverage.width).toBeLessThan(180);
  await Promise.all([page.waitForURL(revisions[2].url), picker.selectOption(revisions[2].url)]);
  await expect(page.locator(".revision-picker-label")).toHaveText("Revision 3 (Latest)");
  await expect(page.locator("[data-revision-heading]")).not.toHaveClass(/is-stale/);
});

test("child revision selection preserves logical page and falls back to entry", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers marker routing");
  const currentDir = "v".repeat(26);
  const targetDir = "w".repeat(26);
  const chainID = "s".repeat(26);
  const specialLogical = "docs/design ?#%.md";
  const specialRendered = "docs/design ?#%.html";
  const encodedRendered = specialRendered.split("/").map(encodeURIComponent).join("/");
  const currentChild = `${baseURL}/${currentDir}/${encodedRendered}`;
  const targetChild = `${baseURL}/${targetDir}/${encodedRendered}`;
  const targetEntry = `${baseURL}/${targetDir}/index.html`;
  const childHTML = linkedBundlePage(
    bundleMembers.get("/bundle/docs/design.html")!,
    2,
    chainID,
    specialLogical,
    "../index.html",
  );
  await page.route(currentChild, (route) =>
    route.fulfill({ contentType: "text/html; charset=utf-8", body: childHTML }),
  );
  await page.route(`${baseURL}/${targetDir}/**`, (route) =>
    route.fulfill({ contentType: "text/html; charset=utf-8", body: childHTML }),
  );
  const versionsPattern = `**/${currentDir}/.airplan-versions.json?*`;
  let versionsBody = JSON.stringify({
    schema: "airplan-versions",
    version: 1,
    chain_id: chainID,
    current_revision: 2,
    latest_revision: 2,
    last_assigned_revision: 2,
    revisions: [
      { number: 1, url: targetEntry, created_at: "2026-08-15T10:00:00Z" },
      {
        number: 2,
        url: `${baseURL}/${currentDir}/index.html`,
        created_at: "2026-08-15T10:10:00Z",
        diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
      },
    ],
  });
  await page.route(versionsPattern, (route) =>
    route.fulfill({
      contentType: "application/json",
      body: versionsBody,
    }),
  );
  const markerPattern = `**/${targetDir}/.airplan.json?*`;
  const validMarker = {
    schema: "airplan-upload",
    version: 6,
    directory: targetDir,
    created_at: "2026-08-15T10:00:00Z",
    kind: "document",
    slug: "index",
    format: "md",
    title: "Bundle overview",
    repo: "https://github.com/acme/service",
    producer: { name: "airplan", version: "0.10.0" },
    render: {
      generation: 5,
      template: { kind: "builtin" },
      indexable: false,
      no_external_assets: false,
      mermaid_url: "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs",
      themes: {
        default_light: "github-light",
        default_dark: "github-dark",
        catalog_sha256: "c".repeat(64),
      },
    },
    entrypoint: "index.html",
    revision: { chain_id: chainID, number: 1 },
    objects: [
      {
        name: "index.html",
        role: "page",
        bytes: 100,
        content_type: "text/html; charset=utf-8",
        sha256: "a".repeat(64),
      },
      {
        name: "index.md",
        role: "source",
        bytes: 10,
        content_type: "text/markdown; charset=utf-8",
        sha256: "b".repeat(64),
      },
      {
        name: specialRendered,
        role: "page",
        bytes: 100,
        content_type: "text/html; charset=utf-8",
        sha256: "d".repeat(64),
      },
      {
        name: specialLogical,
        role: "source",
        bytes: 10,
        content_type: "text/markdown; charset=utf-8",
        sha256: "e".repeat(64),
      },
    ],
    pages: [
      {
        path: "README.md",
        page: "index.html",
        source: "index.md",
        format: "md",
        lang: "Markdown",
      },
      {
        path: specialLogical,
        page: specialRendered,
        source: specialLogical,
        format: "md",
        lang: "Markdown",
      },
    ],
  };
  let markerBody = JSON.stringify(validMarker);
  await page.route(markerPattern, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 100));
    await route.fulfill({ contentType: "application/json", body: markerBody });
  });

  await page.goto(currentChild + "#deep-dive");
  const selector = page.getByRole("combobox", { name: "Document revision" });
  const navigation = page.waitForURL(targetChild + "#deep-dive");
  const selection = selector.selectOption({ label: "Revision 1 of 2" });
  await expect(selector).toBeDisabled();
  await Promise.all([navigation, selection]);
  await expect(page).toHaveURL(targetChild + "#deep-dive");

  const revisionOneEntry = `${baseURL}/${"q".repeat(26)}/index.html`;
  versionsBody = JSON.stringify({
    schema: "airplan-versions",
    version: 1,
    chain_id: chainID,
    current_revision: 2,
    latest_revision: 3,
    last_assigned_revision: 3,
    revisions: [
      { number: 1, url: revisionOneEntry, created_at: "2026-08-15T09:50:00Z" },
      {
        number: 2,
        url: `${baseURL}/${currentDir}/index.html`,
        created_at: "2026-08-15T10:00:00Z",
        diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
      },
      {
        number: 3,
        url: targetEntry,
        created_at: "2026-08-15T10:10:00Z",
        diff_url: `${baseURL}/${targetDir}/.airplan-changes.diff`,
      },
    ],
  });
  const validRevisionThreeMarker = {
    ...validMarker,
    extension: { future: true },
    producer: { ...validMarker.producer, extension: "accepted" },
    render: {
      ...validMarker.render,
      extension: true,
      template: { ...validMarker.render.template, extension: true },
      themes: { ...validMarker.render.themes, extension: true },
    },
    revision: {
      chain_id: chainID,
      number: 3,
      previous_url: `${baseURL}/${currentDir}/index.html`,
      extension: true,
    },
    objects: [
      { ...validMarker.objects[0], extension: true },
      ...validMarker.objects.slice(1),
      {
        name: ".airplan-changes.diff",
        role: "diff",
        bytes: 100,
        content_type: "text/plain; charset=utf-8",
        sha256: "f".repeat(64),
        extension: true,
      },
    ],
    pages: [{ ...validMarker.pages[0], extension: true }, ...validMarker.pages.slice(1)],
  };
  markerBody = JSON.stringify(validRevisionThreeMarker);
  await page.goto(currentChild + "#deep-dive");
  await Promise.all([
    page.waitForURL(targetChild + "#deep-dive"),
    page.getByRole("combobox", { name: "Document revision" }).selectOption({
      label: "Revision 3 (Latest)",
    }),
  ]);
  await expect(page).toHaveURL(targetChild + "#deep-dive");

  const conflictPageObjects = ["a!b", "a/b"].flatMap((logical, index) => [
    {
      name: `${logical}.html`,
      role: "page",
      bytes: 100,
      content_type: "text/html; charset=utf-8",
      sha256: String(index + 1).repeat(64),
    },
    {
      name: logical,
      role: "source",
      bytes: 10,
      content_type: "text/markdown; charset=utf-8",
      sha256: String(index + 3).repeat(64),
    },
  ]);
  const conflictPages = ["a!b", "a/b"].map((logical) => ({
    path: logical,
    page: `${logical}.html`,
    source: logical,
    format: "md",
    lang: "Markdown",
  }));
  const pageAncestorConflictMarker = {
    ...validRevisionThreeMarker,
    objects: [...validRevisionThreeMarker.objects, ...conflictPageObjects],
    pages: [
      { ...validRevisionThreeMarker.pages[0], path: "a" },
      ...validRevisionThreeMarker.pages.slice(1),
      ...conflictPages,
    ],
  };
  const objectAncestorConflictMarker = {
    ...validRevisionThreeMarker,
    objects: [
      ...validRevisionThreeMarker.objects,
      ...["a", "a!b", "a/b"].map((name, index) => ({
        name,
        role: "asset",
        bytes: 1,
        content_type: "application/octet-stream",
        sha256: String(index + 6).repeat(64),
      })),
    ],
  };
  const invalidTargets = [
    { name: "missing marker", status: 404, body: "" },
    { name: "malformed marker", status: 200, body: "{" },
    {
      name: "duplicate root field",
      status: 200,
      body: JSON.stringify(validRevisionThreeMarker).replace(/^\{/, '{"schema":"airplan-upload",'),
    },
    {
      name: "duplicate nested field",
      status: 200,
      body: JSON.stringify(validRevisionThreeMarker).replace(
        '"producer":{"name":"airplan"',
        '"producer":{"name":"airplan","name":"airplan"',
      ),
    },
    {
      name: "wrong version",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, version: 5 }),
    },
    {
      name: "wrong directory",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, directory: "x".repeat(26) }),
    },
    {
      name: "non-adjacent object ancestor conflict",
      status: 200,
      body: JSON.stringify(objectAncestorConflictMarker),
    },
    {
      name: "non-adjacent page ancestor conflict",
      status: 200,
      body: JSON.stringify(pageAncestorConflictMarker),
    },
    {
      name: "noncanonical repository",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        repo: "https://github.com/acme/service.git",
      }),
    },
    {
      name: "noncanonical slug",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, slug: "Index" }),
    },
    {
      name: "non-UTC timestamp",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        created_at: "2026-08-15T11:10:00+01:00",
      }),
    },
    {
      name: "missing producer",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, producer: undefined }),
    },
    {
      name: "missing light theme provenance",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        render: {
          ...validRevisionThreeMarker.render,
          themes: { ...validRevisionThreeMarker.render.themes, default_light: undefined },
        },
      }),
    },
    {
      name: "null dark theme provenance",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        render: {
          ...validRevisionThreeMarker.render,
          themes: { ...validRevisionThreeMarker.render.themes, default_dark: null },
        },
      }),
    },
    {
      name: "oversized theme provenance",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        render: {
          ...validRevisionThreeMarker.render,
          themes: { ...validRevisionThreeMarker.render.themes, default_light: "a".repeat(49) },
        },
      }),
    },
    {
      name: "wrong chain",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        revision: { ...validRevisionThreeMarker.revision, chain_id: "z".repeat(26) },
      }),
    },
    {
      name: "reserved case-folded object",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        objects: [
          ...validRevisionThreeMarker.objects,
          {
            name: "docs/.AIRPLAN-secret",
            role: "asset",
            bytes: 0,
            content_type: "application/octet-stream",
            sha256: "f".repeat(64),
          },
        ],
      }),
    },
    {
      name: "oversized marker",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, padding: "x".repeat(256 * 1024) }),
    },
    {
      name: "mismatched entrypoint",
      status: 200,
      body: JSON.stringify({ ...validRevisionThreeMarker, entrypoint: "other.html" }),
    },
    {
      name: "traversal mapping",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        pages: [
          validRevisionThreeMarker.pages[0],
          { ...validRevisionThreeMarker.pages[1], page: "../escape.html" },
        ],
      }),
    },
    {
      name: "duplicate rendered mapping",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        pages: [
          ...validRevisionThreeMarker.pages,
          { ...validRevisionThreeMarker.pages[1], path: "docs/copy.md" },
        ],
      }),
    },
    {
      name: "missing logical page",
      status: 200,
      body: JSON.stringify({
        ...validRevisionThreeMarker,
        pages: validRevisionThreeMarker.pages.slice(0, 1),
      }),
    },
  ];
  for (const invalid of invalidTargets) {
    await page.unroute(markerPattern);
    await page.route(markerPattern, (route) =>
      route.fulfill({
        status: invalid.status,
        contentType: "application/json",
        body: invalid.body,
      }),
    );
    await page.goto(currentChild + "#deep-dive");
    await page.getByRole("combobox", { name: "Document revision" }).selectOption({
      label: "Revision 3 (Latest)",
    });
    await expect(page, invalid.name).toHaveURL(targetEntry);
  }

  await page.goto(currentChild + "#deep-dive");
  await page.evaluate((limit) => {
    sessionStorage.removeItem("airplan-marker-stream-pulls");
    sessionStorage.removeItem("airplan-marker-stream-cancelled");
    const nativeFetch = window.fetch.bind(window);
    window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const requestURL = input instanceof Request ? input.url : String(input);
      if (!new URL(requestURL, window.location.href).pathname.endsWith("/.airplan.json"))
        return nativeFetch(input, init);
      let pulls = 0;
      return new Response(
        new ReadableStream<Uint8Array>({
          pull(controller) {
            pulls += 1;
            sessionStorage.setItem("airplan-marker-stream-pulls", String(pulls));
            controller.enqueue(new Uint8Array(limit + 1));
            if (pulls === 5) controller.close();
          },
          cancel() {
            sessionStorage.setItem("airplan-marker-stream-cancelled", "true");
          },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }) as typeof window.fetch;
  }, 256 * 1024);
  await page.getByRole("combobox", { name: "Document revision" }).selectOption({
    label: "Revision 3 (Latest)",
  });
  await expect(page).toHaveURL(targetEntry);
  await expect
    .poll(() => page.evaluate(() => sessionStorage.getItem("airplan-marker-stream-cancelled")))
    .toBe("true");
  const markerPulls = Number(
    await page.evaluate(() => sessionStorage.getItem("airplan-marker-stream-pulls")),
  );
  expect(markerPulls).toBeGreaterThan(0);
  expect(markerPulls).toBeLessThan(5);
});

test("revision metadata rejects same-origin URLs outside the current key prefix", async ({
  page,
}) => {
  const currentDir = "e".repeat(26);
  const otherDir = "f".repeat(26);
  await page.route("**/.airplan-versions.json?*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        schema: "airplan-versions",
        version: 1,
        chain_id: "g".repeat(26),
        current_revision: 1,
        latest_revision: 2,
        last_assigned_revision: 2,
        revisions: [
          {
            number: 1,
            url: `${baseURL}/${currentDir}/plan.html`,
            created_at: "2026-08-15T10:00:00Z",
          },
          {
            number: 2,
            url: `${baseURL}/other/${otherDir}/plan.html`,
            created_at: "2026-08-15T10:10:00Z",
            diff_url: `${baseURL}/other/${otherDir}/.airplan-changes.diff`,
          },
        ],
      }),
    }),
  );
  await page.goto(`${baseURL}/${currentDir}/plan.html`);
  await expect(page.getByRole("combobox", { name: "Document revision" })).toHaveCount(0);
  await expect(page.locator(".revision-heading.is-picker")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Browser smoke plan" })).toBeVisible();
});

test("valid revision metadata with one live member labels it as latest", async ({ page }) => {
  const currentDir = "h".repeat(26);
  const warnings: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "warning") warnings.push(message.text());
  });
  await page.route("**/.airplan-versions.json?*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        schema: "airplan-versions",
        version: 1,
        chain_id: "i".repeat(26),
        current_revision: 2,
        latest_revision: 2,
        last_assigned_revision: 2,
        revisions: [
          {
            number: 1,
            deleted: true,
            deleted_at: "2026-08-15T10:20:00Z",
          },
          {
            number: 2,
            url: `${baseURL}/${currentDir}/plan.html`,
            created_at: "2026-08-15T10:10:00Z",
            diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
          },
        ],
      }),
    }),
  );
  await page.goto(`${baseURL}/${currentDir}/plan.html`);
  const picker = page.getByRole("combobox", { name: "Document revision" });
  await expect(picker).toBeVisible();
  await expect(picker.locator("option")).toHaveText(["Revision 2 (Latest)"]);
  await expect(page.locator(".revision-picker-label")).toHaveText("Revision 2 (Latest)");
  await expect(page.locator("[data-revision-heading]")).not.toHaveClass(/is-stale/);
  expect(warnings).toEqual([]);
});

test("empty revision metadata fails closed without breaking the document", async ({ page }) => {
  const currentDir = "j".repeat(26);
  const warnings: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "warning") warnings.push(message.text());
  });
  await page.route("**/.airplan-versions.json?*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        schema: "airplan-versions",
        version: 1,
        chain_id: "k".repeat(26),
        current_revision: 1,
        latest_revision: 0,
        last_assigned_revision: 0,
        revisions: [],
      }),
    }),
  );
  await page.goto(`${baseURL}/${currentDir}/plan.html`);
  await expect(page.getByRole("combobox", { name: "Document revision" })).toHaveCount(0);
  await expect(page.locator(".revision-heading.is-picker")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Browser smoke plan" })).toBeVisible();
  await expect
    .poll(() => warnings)
    .toContain("airplan: revision metadata is unavailable or invalid");
});

test("revision Changes view switches and exposes its adjacent raw diff", async ({ page }) => {
  const firstDir = "q".repeat(26);
  const currentDir = "r".repeat(26);
  const revisions = [
    {
      number: 1,
      url: `${baseURL}/${firstDir}/plan.html`,
      created_at: "2026-08-15T10:00:00Z",
    },
    {
      number: 2,
      url: `${baseURL}/${currentDir}/plan.html`,
      created_at: "2026-08-15T10:10:00Z",
      diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
    },
  ];
  await page.route("**/.airplan-versions.json?*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        schema: "airplan-versions",
        version: 1,
        chain_id: "s".repeat(26),
        current_revision: 2,
        latest_revision: 2,
        last_assigned_revision: 2,
        revisions,
      }),
    }),
  );
  await page.goto(revisions[1].url);
  const readerControls = page.locator(".reader-controls");
  for (const width of [1400, 700]) {
    await page.setViewportSize({ width, height: 800 });
    await expect
      .poll(() =>
        readerControls.evaluate((element) => {
          const view = element.querySelector(".viewtoggle")!;
          const revision = element.querySelector(".revision-controls")!;
          return {
            domOrder: Boolean(
              view.compareDocumentPosition(revision) & Node.DOCUMENT_POSITION_FOLLOWING,
            ),
            visualOrder:
              view.getBoundingClientRect().bottom <= revision.getBoundingClientRect().top,
          };
        }),
      )
      .toEqual({ domOrder: true, visualOrder: true });
  }
  await expect(page.getByRole("button", { name: "Rendered view" }).locator("svg.icon")).toHaveCount(
    1,
  );
  await expect(page.getByRole("button", { name: "Source view" }).locator("svg.icon")).toHaveCount(
    1,
  );
  await page.setViewportSize({ width: 700, height: 800 });
  const toolbarLayout = await page.locator(".toolbar").evaluate((element) => {
    const view = document.querySelector(".reader-controls .viewtoggle")!.getBoundingClientRect();
    const toolbar = element.getBoundingClientRect();
    return {
      display: getComputedStyle(element).display,
      position: getComputedStyle(element).position,
      toolbarBottom: toolbar.bottom,
      viewTop: view.top,
    };
  });
  expect(toolbarLayout.display).toBe("flex");
  expect(toolbarLayout.position).toBe("sticky");
  expect(toolbarLayout.viewTop).toBeGreaterThanOrEqual(toolbarLayout.toolbarBottom);
  const modeBackgrounds = await page
    .locator(".reader-controls .mode-toggle button")
    .evaluateAll((buttons) =>
      buttons.map((button) => ({
        active: button.classList.contains("active"),
        background: getComputedStyle(button).backgroundColor,
      })),
    );
  expect(
    modeBackgrounds.filter((button) => !button.active).map((button) => button.background),
  ).toEqual(["rgba(0, 0, 0, 0)", "rgba(0, 0, 0, 0)"]);
  expect(modeBackgrounds.find((button) => button.active)?.background).not.toBe("rgba(0, 0, 0, 0)");
  const pagesButton = page.getByRole("button", { name: "Open pages" });
  await expect(pagesButton).toBeVisible();
  await pagesButton.click();
  await page.locator(".pages-popover-nav").getByRole("link", { name: "Notes notes.md" }).click();
  await expect(page).toHaveURL(`${baseURL}/${currentDir}/notes.html`);
  await expect(page.getByRole("heading", { level: 1, name: "Notes" })).toBeVisible();
  await page.goto(revisions[1].url);
  const changesButton = page.getByRole("button", { name: "Changes view" });
  await expect(changesButton).toBeVisible();
  await changesButton.click();
  await expect(page.locator("#changes")).toBeVisible();
  await expect(page.locator("#rendered")).toBeHidden();
  await expect(page.locator("#changes")).toContainText("Changes to plan.md from revision 1");
  await expect(page.locator("#changes")).toContainText("-Original");
  await expect(page.locator("#changes")).toContainText("+Revised");
  await expect(page.getByRole("link", { name: "Open raw diff" })).toHaveAttribute(
    "href",
    "./.airplan-changes.diff",
  );
  await page.emulateMedia({ media: "print" });
  await expect(page.locator("#changes")).toBeHidden();
  await expect(page.locator("#rendered")).toBeVisible();
  await page.emulateMedia({ media: "screen" });
  await page.getByRole("button", { name: "Rendered view" }).click();
  await expect(page.locator("#rendered")).toBeVisible();
  await expect(page.locator("#changes")).toBeHidden();
  await page.getByRole("link", { name: "All changes" }).click();
  await expect(page).toHaveURL(/#airplan-all-changes$/);
  await expect(page.locator("[data-airplan-all-changes]")).toBeVisible();
  await expect(page.locator("#rendered")).toBeHidden();
  await expect(page.getByRole("button", { name: "Open pages" })).toBeHidden();
  const allChangesActions = page.getByRole("navigation", { name: "All changes actions" });
  await expect(allChangesActions.getByRole("link", { name: "Back to document" })).toBeVisible();
  await expect(allChangesActions.getByRole("link", { name: "Open raw diff" })).toBeVisible();
  const actionPlacement = await allChangesActions.evaluate((element) => {
    const heading = element.parentElement!.querySelector("h1")!;
    const diff = element.parentElement!.querySelector("pre")!;
    return {
      beforeHeading: element.getBoundingClientRect().bottom <= heading.getBoundingClientRect().top,
      beforeDiff: element.getBoundingClientRect().bottom <= diff.getBoundingClientRect().top,
    };
  });
  expect(actionPlacement).toEqual({ beforeHeading: true, beforeDiff: true });
  await page.reload();
  await expect(page.locator("[data-airplan-all-changes]")).toBeVisible();
  await page.goBack();
  await expect(page).not.toHaveURL(/#airplan-all-changes$/);
  await expect(page.locator("#rendered")).toBeVisible();
});

test("rendered page controls work", async ({ context, page }, testInfo) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: baseURL });
  await page.goto(baseURL);

  const dark = testInfo.project.name.endsWith("-dark");
  await expect(page).toHaveTitle("Browser smoke plan");
  await expect(page.getByRole("heading", { level: 1, name: "Browser smoke plan" })).toBeVisible();
  await expect(page.locator("#rendered").getByText("This fixture verifies")).toBeVisible();
  const toolbar = page.getByRole("navigation", { name: "Document controls" });
  const narrow = testInfo.project.name.startsWith("narrow-");
  await expect(toolbar).toHaveCSS("justify-content", "flex-end");
  await expect
    .poll(() =>
      toolbar.evaluate((element) =>
        Array.from(element.querySelectorAll(".copy-source, .download, .raw, .appearance"))
          .filter((child) => !(child as HTMLElement).hidden)
          .map((child) =>
            Array.from(child.classList).find((name) =>
              ["copy-source", "download", "raw", "appearance"].includes(name),
            ),
          ),
      ),
    )
    .toEqual(["copy-source", "appearance"]);
  await expect(page.locator(".reader-controls .viewtoggle")).toBeVisible();
  await expect(toolbar.locator(".viewtoggle")).toHaveCount(0);
  const dividerDisplay = await page
    .locator(".appearance")
    .evaluate((element) => getComputedStyle(element, "::before").display);
  const copyDivider = await page
    .locator(".copy-source")
    .evaluate((element) => getComputedStyle(element, "::before").content);
  expect(copyDivider).toBe("none");
  if (narrow) {
    const alignment = await toolbar.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const theme = element.querySelector(".appearance")!.getBoundingClientRect();
      const copy = element.querySelector(".copy-source")!.getBoundingClientRect();
      const fileActions = element.querySelector(".file-actions")!.getBoundingClientRect();
      return {
        right: bounds.right - theme.right,
        rightPadding: Number.parseFloat(styles.paddingRight),
        themeCenter: theme.top + theme.height / 2,
        copyCenter: copy.top + copy.height / 2,
        actionsRight: bounds.right - fileActions.right,
      };
    });
    expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
    expect(alignment.copyCenter).toBeCloseTo(alignment.themeCenter, 0);
    expect(alignment.actionsRight).toBeGreaterThanOrEqual(0);
    expect(dividerDisplay).toBe("none");
  } else {
    expect(dividerDisplay).not.toBe("none");
    const dividerSpacing = await page.locator(".appearance").evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const divider = getComputedStyle(element, "::before");
      const previousAction = element.previousElementSibling!.lastElementChild!;
      const previousLabel = previousAction.lastElementChild!.getBoundingClientRect();
      const dividerX = bounds.left + Number.parseFloat(divider.left);
      return {
        before: dividerX - previousLabel.right,
        after: bounds.left - dividerX,
      };
    });
    expect(dividerSpacing.before).toBeCloseTo(dividerSpacing.after, 0);
  }
  expect(
    await page.evaluate(
      (scheme) => window.matchMedia(`(prefers-color-scheme: ${scheme})`).matches,
      dark ? "dark" : "light",
    ),
  ).toBe(true);
  const theme = await page.evaluate(() => {
    const styles = window.getComputedStyle(document.body);
    const brightness = (value: string) =>
      (value.match(/\d+/g) || [])
        .slice(0, 3)
        .map(Number)
        .reduce((sum: number, channel: number) => sum + channel, 0);
    return {
      background: brightness(styles.backgroundColor),
      foreground: brightness(styles.color),
    };
  });
  if (dark) {
    expect(theme.background).toBeLessThan(theme.foreground);
  } else {
    expect(theme.background).toBeGreaterThan(theme.foreground);
  }

  await page.getByRole("button", { name: "Appearance" }).click();
  const lightTheme = page.getByRole("button", { name: "Light", exact: true });
  const systemTheme = page.getByRole("button", { name: "System", exact: true });
  const darkTheme = page.getByRole("button", { name: "Dark", exact: true });
  const diagram = page.locator("pre.mermaid svg").first();
  await expect(systemTheme).toHaveAttribute("aria-pressed", "true");
  await expect(diagram).toHaveAttribute(
    "data-mermaid-theme",
    dark ? "github-dark" : "github-light",
  );

  await lightTheme.click();
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "light");
  await expect(lightTheme).toHaveAttribute("aria-pressed", "true");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "github-light");
  await expect(page.locator(".chroma .nx").first()).toHaveCSS("color", "rgb(31, 35, 40)");

  await darkTheme.click();
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "dark");
  await expect(darkTheme).toHaveAttribute("aria-pressed", "true");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "github-dark");
  await expect(page.locator(".chroma .nx").first()).toHaveCSS("color", "rgb(230, 237, 243)");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "dark");
  await page.getByRole("button", { name: "Appearance" }).click();
  await expect(darkTheme).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-dark",
  );
  await systemTheme.click();
  await expect(page.locator("html")).toHaveAttribute("data-airplan-mode", "system");
  await expect(systemTheme).toHaveAttribute("aria-pressed", "true");

  if (testInfo.project.name.startsWith("narrow-")) {
    await expect(page.locator("#toc")).toBeHidden();
    const openToc = page.getByRole("button", {
      name: "Open table of contents",
    });
    await expect(openToc).toHaveAttribute("aria-hidden", "false");
    await openToc.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Contents" })).toBeVisible();
    await dialog.getByRole("link", { name: "Code sample" }).click();
    await expect(dialog).toBeHidden();
    await expect(page).toHaveURL(/#code-sample$/);
  } else {
    const inlineToc = page.locator("#toc");
    await inlineToc.getByRole("link", { name: "Details" }).click();
    await expect(page).toHaveURL(/#details$/);
    await expect(page.getByRole("heading", { name: "Details" })).toBeVisible();
  }

  const renderedButton = page.getByRole("button", { name: "Rendered view" });
  const sourceButton = page.getByRole("button", { name: "Source view" });
  await expect(renderedButton).toHaveAttribute("aria-pressed", "true");
  await expect(sourceButton).toHaveAttribute("aria-pressed", "false");
  await sourceButton.click();
  await expect(sourceButton).toHaveAttribute("aria-pressed", "true");
  await expect(renderedButton).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator("#source")).toBeVisible();
  await expect(page.locator("#rendered")).toBeHidden();

  await page.getByRole("button", { name: "Copy markdown" }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(fixtureSource);

  await renderedButton.click();
  await expect(page.locator("#rendered")).toBeVisible();
  const copyCode = page.getByRole("button", { name: "Copy code" }).first();
  await copyCode.click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(expectedCode);
});

test("Mermaid viewer is shared, theme-aware, and responsive", async ({ page }, testInfo) => {
  await page.goto(baseURL);

  const triggers = page.getByRole("button", { name: "Open diagram viewer" });
  await expect(triggers).toHaveCount(4);
  const firstTrigger = triggers.first();
  await expect(firstTrigger).toHaveCSS("opacity", "0");
  await expect(firstTrigger).toHaveCSS("pointer-events", "none");
  await page.locator("pre.mermaid").first().hover();
  await expect(firstTrigger).toHaveCSS("opacity", "1");
  await firstTrigger.focus();
  await expect(firstTrigger).toBeFocused();
  await firstTrigger.click();

  const dialog = page.locator("[data-airplan-mermaid-dialog]");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Mermaid diagram")).toBeVisible();
  await expect(dialog.locator("[data-airplan-mermaid-canvas]")).toBeFocused();
  const geometry = await dialog.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    return {
      width: bounds.width,
      height: bounds.height,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      background: getComputedStyle(element).backgroundColor,
    };
  });
  expect(geometry.width).toBeGreaterThan(geometry.viewportWidth * 0.9);
  expect(geometry.height).toBeGreaterThan(geometry.viewportHeight * 0.9);
  expect(geometry.background).toBe(
    testInfo.project.name.endsWith("-dark") ? "rgb(13, 17, 23)" : "rgb(255, 255, 255)",
  );

  const inlineIDs = await page
    .locator("pre.mermaid svg")
    .first()
    .locator("[id]")
    .evaluateAll((elements) => elements.map((el) => el.id));
  const inlineRootID = await page.locator("pre.mermaid svg").first().getAttribute("id");
  const clone = dialog.locator(".mermaid-surface > svg");
  const cloneData = await clone.evaluate((svg) => ({
    ids: Array.from(svg.querySelectorAll("[id]"), (el) => el.id),
    marker: svg.querySelector("[marker-end]")!.getAttribute("marker-end") || "",
    href: svg.querySelector("a")!.getAttribute("href") || "",
    labelledBy: (svg.getAttribute("aria-labelledby") || "").split(/\s+/),
    describedBy: (svg.getAttribute("aria-describedby") || "").split(/\s+/),
    nestedLabelledBy: svg.querySelector("g")!.getAttribute("aria-labelledby") || "",
    style: svg.querySelector("style")!.textContent || "",
  }));
  expect(cloneData.ids.some((id) => inlineIDs.includes(id))).toBe(false);
  const referenced = [
    cloneData.marker.slice(5, -1),
    cloneData.href.slice(1),
    ...cloneData.labelledBy,
    ...cloneData.describedBy,
    cloneData.nestedLabelledBy,
  ];
  for (const id of referenced) {
    expect(cloneData.ids).toContain(id);
  }
  for (const id of cloneData.ids) {
    if (id.endsWith("-node") || id.endsWith("-label")) {
      expect(cloneData.style).toContain(`#${id}`);
    }
  }
  const cloneNodeID = cloneData.ids.find((id) => id.endsWith("-node"));
  expect(cloneData.style).toContain(`#${inlineRootID || ""}-node-extra{opacity:.5}`);
  expect(cloneData.style).not.toContain(`#${cloneNodeID}-extra`);

  await dialog.getByRole("button", { name: "Close diagram viewer" }).click();
  await expect(dialog).toBeHidden();
  await expect(firstTrigger).toBeFocused();
  await page.locator("pre.mermaid").nth(1).hover();
  await triggers.nth(1).click();
  await expect(dialog).toBeVisible();
  await expect(page.locator("[data-airplan-mermaid-dialog]")).toHaveCount(1);
  await expect(dialog.locator(".mermaid-surface > svg")).toContainText("Source --> Render");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(triggers.nth(1)).toBeFocused();
});

test("Mermaid themes render lazily, cache, reject races, and isolate failures", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers Mermaid cache behavior");
  await page.goto(baseURL);
  const diagram = page.locator("pre.mermaid svg").first();
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "github-light");
  await expect
    .poll(() =>
      page.evaluate(() => ({
        ...(globalThis as typeof globalThis & { __airplanMermaidRenders?: Record<string, number> })
          .__airplanMermaidRenders,
      })),
    )
    .toMatchObject({ "github-light": 4, "github-dark": 4 });

  await page.getByRole("button", { name: "Appearance" }).click();
  await page.getByRole("button", { name: "Light", exact: true }).click();
  const select = page.getByRole("combobox", { name: "Light theme", exact: true });
  await select.selectOption("tokyo-night");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "tokyo-night");
  await select.selectOption("solarized-light");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "solarized-light");
  await select.selectOption("tokyo-night");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "tokyo-night");
  expect(
    await page.evaluate(
      () =>
        (globalThis as typeof globalThis & { __airplanMermaidRenders: Record<string, number> })
          .__airplanMermaidRenders["tokyo-night"],
    ),
  ).toBe(4);

  await select.selectOption("github-dark");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "github-dark");
  await select.selectOption("tokyo-night");
  await select.selectOption("catppuccin-latte");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "catppuccin-latte");

  await select.selectOption("one-dark");
  await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", "one-dark");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "catppuccin-latte");
  await page.emulateMedia({ media: "print" });
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "github-light");
  await page.emulateMedia({ media: "screen" });
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "catppuccin-latte");
  await select.selectOption("rose-pine-dawn");
  await expect(diagram).toHaveAttribute("data-mermaid-theme", "rose-pine-dawn");
});

test("Mermaid startup applies theme changes that arrive while rendering", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers startup races");
  await page.addInitScript(() => {
    const fixtureWindow = globalThis as typeof globalThis & {
      __airplanMermaidDelay?: number;
      __airplanMermaidStartupTheme?: string;
    };
    fixtureWindow.__airplanMermaidDelay = 30;
    fixtureWindow.__airplanMermaidStartupTheme = "tokyo-night-day";
  });
  await page.goto(baseURL);
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "tokyo-night-day",
  );
});

test("Mermaid startup applies print changes that arrive while rendering", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers startup print races");
  await page.addInitScript(() => {
    localStorage.setItem("airplan-color-mode", "dark");
    const fixtureWindow = globalThis as typeof globalThis & {
      __airplanMermaidDelay?: number;
      __airplanMermaidStartupPrint?: boolean;
    };
    fixtureWindow.__airplanMermaidDelay = 30;
    fixtureWindow.__airplanMermaidStartupPrint = true;
  });
  await page.goto(baseURL);
  await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", "github-dark");
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-light",
  );
});

test("Mermaid print render IDs replace disallowed key characters with hyphens", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers generated IDs");
  await page.addInitScript(() => {
    localStorage.setItem("airplan-light-theme", "solarized-dark");
    localStorage.setItem("airplan-dark-theme", "tokyo-night");
  });
  await page.goto(baseURL);
  await page.emulateMedia({ media: "print" });
  await expect(page.locator("pre.mermaid > svg").first()).toHaveAttribute(
    "id",
    "airplan-mermaid---airplan-print-github-light-0",
  );
});

test("Mermaid isolates one failed diagram from valid diagrams", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers diagram isolation");
  await page.goto(baseURL);
  await page.evaluate(() => {
    const fixtureWindow = globalThis as typeof globalThis & {
      __airplanMermaidFailSource?: string;
    };
    fixtureWindow.__airplanMermaidFailSource = "Source --> Render";
  });
  await page.getByRole("button", { name: "Appearance" }).click();
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Light theme", exact: true })
    .selectOption("solarized-light");

  const diagrams = page.locator("pre.mermaid");
  await expect(diagrams.locator('svg[data-mermaid-theme="solarized-light"]')).toHaveCount(3);
  await expect(diagrams.nth(0).locator("svg[data-mermaid-theme]")).toHaveAttribute(
    "data-mermaid-theme",
    "solarized-light",
  );
  await expect(diagrams.nth(1).locator("svg[data-mermaid-theme]")).toHaveAttribute(
    "data-mermaid-theme",
    "github-light",
  );
  await expect(page.getByRole("button", { name: "Open diagram viewer" })).toHaveCount(4);
});

test("Mermaid opener media behavior follows input and motion preferences", async ({
  browser,
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop-light",
    "one desktop project covers computed media-query behavior",
  );

  await page.goto(baseURL);
  const trigger = page
    .getByRole("button", {
      name: "Open diagram viewer",
    })
    .first();
  await expect(trigger).toHaveCSS("opacity", "0");
  await expect(trigger).toHaveCSS("pointer-events", "none");

  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(trigger).toHaveCSS("transition-duration", "0s");

  const touchContext = await browser.newContext({
    colorScheme: "light",
    hasTouch: true,
    viewport: { width: 390, height: 844 },
  });
  try {
    const touchPage = await touchContext.newPage();
    await touchPage.route(mermaidURL, (route) =>
      route.fulfill({
        body: mermaidModule,
        contentType: "application/javascript",
        status: 200,
      }),
    );
    await touchPage.goto(baseURL);
    expect(await touchPage.evaluate(() => matchMedia("(hover: none)").matches)).toBe(true);
    expect(await touchPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches)).toBe(true);
    const touchTrigger = touchPage
      .getByRole("button", {
        name: "Open diagram viewer",
      })
      .first();
    await expect(touchTrigger).toHaveCSS("opacity", "1");
    await expect(touchTrigger).toHaveCSS("pointer-events", "auto");
  } finally {
    await touchContext.close();
  }
});

test("Mermaid viewer zooms, pans, and preserves its view across themes", async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop-light",
    "one desktop project covers the complete interaction cluster",
  );
  await page.goto(baseURL);
  const trigger = page
    .getByRole("button", {
      name: "Open diagram viewer",
    })
    .first();
  await page.locator("pre.mermaid").first().hover();
  await trigger.click();

  const dialog = page.locator("[data-airplan-mermaid-dialog]");
  const canvas = dialog.locator("[data-airplan-mermaid-canvas]");
  const surface = dialog.locator("[data-airplan-mermaid-surface]");
  const zoom = dialog.locator("[data-airplan-mermaid-zoom-value]");
  const initialPercent = Number.parseInt((await zoom.textContent()) || "", 10);
  expect(initialPercent).toBeLessThanOrEqual(100);

  const modifiedKeyPrevented = await canvas.evaluate((element) => {
    const event = new KeyboardEvent("keydown", {
      bubbles: true,
      cancelable: true,
      ctrlKey: true,
      key: "+",
    });
    element.dispatchEvent(event);
    return event.defaultPrevented;
  });
  expect(modifiedKeyPrevented).toBe(false);
  await expect(zoom).toHaveText(`${initialPercent}%`);

  await dialog.getByRole("button", { name: "Zoom in" }).click();
  await expect(zoom).toHaveText(`${Math.round(initialPercent * 1.25)}%`);
  await dialog.getByRole("button", { name: "Zoom out" }).click();
  await expect(zoom).toHaveText(`${initialPercent}%`);

  const bounds = await canvas.boundingBox();
  if (!bounds) throw new Error("Mermaid canvas has no bounding box");
  await canvas.dispatchEvent("wheel", {
    deltaY: -100,
    clientX: bounds.x + bounds.width * 0.75,
    clientY: bounds.y + bounds.height * 0.25,
  });
  await expect(zoom).toHaveText(`${Math.round(initialPercent * 1.2)}%`);
  const anchoredTransform = await surface.getAttribute("style");
  expect(anchoredTransform).not.toContain("translate(0px, 0px)");

  await canvas.focus();
  await page.keyboard.press("ArrowRight");
  const keyboardTransform = await surface.getAttribute("style");
  expect(keyboardTransform).not.toBe(anchoredTransform);
  await page.keyboard.press("Shift+ArrowDown");
  const shiftedTransform = await surface.getAttribute("style");
  expect(shiftedTransform).not.toBe(keyboardTransform);

  await canvas.dispatchEvent("pointerdown", {
    button: 0,
    pointerId: 7,
    clientX: 200,
    clientY: 200,
  });
  await canvas.dispatchEvent("pointermove", {
    pointerId: 7,
    clientX: 260,
    clientY: 230,
  });
  await canvas.dispatchEvent("pointerup", {
    pointerId: 7,
    clientX: 260,
    clientY: 230,
  });
  const draggedTransform = await surface.getAttribute("style");
  expect(draggedTransform).not.toBe(shiftedTransform);

  const viewerSVG = dialog.locator(".mermaid-surface > svg");
  const svgTransform = await viewerSVG.getAttribute("style");
  const oldCloneID = await viewerSVG.getAttribute("id");
  if (!svgTransform || !oldCloneID || !draggedTransform) {
    throw new Error("Mermaid viewer state is incomplete");
  }
  await page.evaluate(() => {
    document.documentElement.dataset.airplanTheme = "github-dark";
    window.dispatchEvent(
      new CustomEvent("airplan:themechange", {
        detail: { mode: "dark", resolvedMode: "dark", theme: "github-dark", variant: "dark" },
      }),
    );
  });
  await expect(viewerSVG).toHaveAttribute("data-mermaid-theme", "github-dark");
  await expect(viewerSVG).not.toHaveAttribute("id", oldCloneID);
  await expect(surface).toHaveAttribute("style", draggedTransform);
  await expect(viewerSVG).toHaveAttribute("style", svgTransform);

  await page.keyboard.press("+");
  await expect(zoom).not.toHaveText(`${Math.round(initialPercent * 1.2)}%`);
  for (let index = 0; index < 40; index += 1) {
    await page.keyboard.press("-");
  }
  await expect(zoom).toHaveText("5%");
  for (let index = 0; index < 40; index += 1) {
    await page.keyboard.press("+");
  }
  await expect(zoom).toHaveText("800%");
  await page.keyboard.press("0");
  await expect(zoom).toHaveText(`${initialPercent}%`);
  await expect(surface).toHaveAttribute("style", "transform: translate(0px, 0px);");

  await page.mouse.click(2, 2);
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
});

test("Mermaid failures preserve source without viewer controls", async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop-light",
    "one desktop project covers the progressive-enhancement fallback",
  );
  await page.unroute(mermaidURL);
  await page.route(mermaidURL, (route) =>
    route.fulfill({
      body: `export default {
        initialize() {},
        async render() { throw new Error('fixture render failure'); }
      };`,
      contentType: "text/javascript; charset=utf-8",
    }),
  );
  await page.goto(baseURL);

  const diagrams = page.locator("pre.mermaid");
  await expect(diagrams).toHaveCount(4);
  await expect(diagrams.first()).toHaveClass(/mermaid-failed/);
  await expect(diagrams.first()).toContainText("flowchart LR");
  await expect(
    page.getByRole("button", {
      name: "Open diagram viewer",
    }),
  ).toHaveCount(0);
  await expect(page.locator("[data-airplan-mermaid-dialog]")).toHaveCount(0);

  await page.unroute(mermaidURL);
  await page.route(mermaidURL, (route) =>
    route.fulfill({
      body: "this is not valid JavaScript",
      contentType: "text/javascript; charset=utf-8",
    }),
  );
  await page.reload();
  await expect(diagrams.first()).not.toHaveClass(/mermaid-rendered/);
  await expect(diagrams.first()).toContainText("flowchart LR");
  await expect(
    page.getByRole("button", {
      name: "Open diagram viewer",
    }),
  ).toHaveCount(0);
  await expect(page.locator("[data-airplan-mermaid-dialog]")).toHaveCount(0);
});

test("Mermaid viewer requires native modal dialog support", async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop-light",
    "one desktop project covers dialog feature detection",
  );
  await page.addInitScript(() => {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
      configurable: true,
      value: undefined,
    });
  });
  await page.goto(baseURL);

  await expect(page.locator("pre.mermaid svg")).toHaveCount(4);
  await expect(
    page.getByRole("button", {
      name: "Open diagram viewer",
    }),
  ).toHaveCount(0);
  await expect(page.locator("[data-airplan-mermaid-dialog]")).toHaveCount(0);
});

test("revision 404 filtering does not match unrelated resources", () => {
  expect(isVersionManifestURL("https://plans.example.com/id/.airplan-versions.json?nonce=1")).toBe(
    true,
  );
  expect(isVersionManifestURL("https://plans.example.com/id/missing.png")).toBe(false);
  expect(
    isVersionManifestURL(
      "https://plans.example.com/id/missing.png?redirect=/.airplan-versions.json?nonce=1",
    ),
  ).toBe(false);
});

test("uploaded source controls share the first row on narrow screens", async ({
  page,
}, testInfo) => {
  test.skip(
    !testInfo.project.name.startsWith("narrow-"),
    "the regression only applies to the narrow toolbar grid",
  );

  await page.goto(sourceURL);

  const toolbar = page.getByRole("navigation", {
    name: "Document controls",
  });
  await expect(toolbar.locator(".viewtoggle")).toHaveCount(0);
  await expect(toolbar.getByRole("link", { name: "Download source" })).toBeVisible();
  await expect(toolbar.getByRole("link", { name: "Open raw source" })).toBeVisible();

  const alignment = await toolbar.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const styles = getComputedStyle(element);
    const actions = element.querySelector(".file-actions")!.getBoundingClientRect();
    const theme = element.querySelector(".appearance")!.getBoundingClientRect();
    return {
      actionsCenter: actions.top + actions.height / 2,
      right: bounds.right - theme.right,
      rightPadding: Number.parseFloat(styles.paddingRight),
      themeCenter: theme.top + theme.height / 2,
    };
  });
  expect(alignment.actionsCenter).toBeCloseTo(alignment.themeCenter, 0);
  expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
});

test("print view is compact and expands disclosures", async ({ browser, page }, testInfo) => {
  test.skip(
    !testInfo.project.name.startsWith("desktop-"),
    "desktop projects cover both print color schemes",
  );

  await page.goto(baseURL);
  const frontmatter = page.locator(".frontmatter");
  const disclosure = page.locator("#print-disclosure");
  await expect(frontmatter).not.toHaveAttribute("open", "");
  await expect(frontmatter.getByText("Print coverage")).toBeHidden();
  await expect(disclosure).not.toHaveAttribute("open", "");
  await expect(disclosure.getByText("Print must include")).toBeHidden();

  await page.getByRole("button", { name: "Appearance" }).click();
  const darkTheme = page.getByRole("button", { name: "Dark", exact: true });
  await darkTheme.click();
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-dark",
  );
  await page.emulateMedia({ media: "print" });
  await expect(page.locator(".toolbar")).toBeHidden();
  await expect(
    page
      .getByRole("button", {
        name: "Open diagram viewer",
      })
      .first(),
  ).toBeHidden();
  await expect(page.locator("[data-airplan-mermaid-dialog]")).toHaveCSS("display", "none");
  await expect(frontmatter.getByText("Print coverage")).toBeVisible();
  await expect(disclosure.getByText("Print must include")).toBeVisible();
  await expect(page.locator("body")).toHaveCSS("font-size", "14px");
  await expect(page.locator("body")).toHaveCSS("line-height", "20.3px");
  await expect(page.locator("body")).toHaveCSS("background-color", "rgb(255, 255, 255)");
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "Browser smoke plan",
    }),
  ).toHaveCSS("color", "rgb(31, 35, 40)");
  await expect(page.locator(".chroma .k, .chroma .kd").first()).toHaveCSS(
    "color",
    "rgb(207, 34, 46)",
  );
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-light",
  );

  const noJSContext = await browser.newContext({
    colorScheme: testInfo.project.name.endsWith("-dark") ? "dark" : "light",
    javaScriptEnabled: false,
  });
  try {
    const noJSPage = await noJSContext.newPage();
    await noJSPage.goto(baseURL);
    await noJSPage.emulateMedia({ media: "print" });
    await expect(noJSPage.locator("html")).toHaveCSS("color-scheme", "light");
    await expect(noJSPage.locator("body")).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await expect(noJSPage.locator("body")).toHaveCSS("color", "rgb(31, 35, 40)");
    await expect(noJSPage.locator(".frontmatter").getByText("Print coverage")).toBeVisible();
    await expect(
      noJSPage.locator("#print-disclosure").getByText("Print must include"),
    ).toBeVisible();
    for (const selector of ["[data-print-hidden]", "[data-print-script]", "[data-print-style]"]) {
      await expect(noJSPage.locator(selector)).toHaveCount(1);
      await expect(noJSPage.locator(selector)).toBeHidden();
    }
  } finally {
    await noJSContext.close();
  }

  await page.emulateMedia({ media: "screen" });
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-dark",
  );
  const initialStates = await page
    .locator("details")
    .evaluateAll((details) => details.map((detail) => (detail as HTMLDetailsElement).open));
  await page.evaluate(() => {
    const stateWindow = window as typeof window & {
      printDisclosureStates: boolean[][];
    };
    stateWindow.printDisclosureStates = [];
    window.addEventListener("beforeprint", () => {
      stateWindow.printDisclosureStates.push(
        Array.from(document.querySelectorAll<HTMLDetailsElement>("details")).map(
          (details) => details.open,
        ),
      );
    });
    window.addEventListener("afterprint", () => {
      stateWindow.printDisclosureStates.push(
        Array.from(document.querySelectorAll<HTMLDetailsElement>("details")).map(
          (details) => details.open,
        ),
      );
    });
  });
  await page.pdf({ format: "Letter", printBackground: true });
  const states = await page.evaluate(
    () => (window as typeof window & { printDisclosureStates: boolean[][] }).printDisclosureStates,
  );
  expect(states).toHaveLength(2);
  expect(states[0]).toHaveLength(initialStates.length);
  expect(states[0].every((open: boolean) => open)).toBe(true);
  expect(states[1]).toEqual(initialStates);
  await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
    "data-mermaid-theme",
    "github-dark",
  );
  await expect(frontmatter).not.toHaveAttribute("open", "");
  await expect(disclosure).not.toHaveAttribute("open", "");
  await expect(page.locator("#print-open-disclosure")).toHaveAttribute("open", "");
});

test("print palette stays GitHub Light from Solarized, One Dark, and custom themes", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "one project covers fixed print palettes");
  for (const sample of [
    { name: "solarized-light", url: baseURL, select: "solarized-light" },
    { name: "one-dark", url: baseURL, select: "one-dark" },
    { name: "custom-dark", url: customURL, select: "" },
  ]) {
    await page.emulateMedia({ media: "screen", colorScheme: "light" });
    await page.goto(sample.url);
    if (sample.select) {
      await page.getByRole("button", { name: "Appearance" }).click();
      await page.getByRole("button", { name: "Light", exact: true }).click();
      await page
        .getByRole("combobox", { name: "Light theme", exact: true })
        .selectOption(sample.select);
      await expect(page.locator("html")).toHaveAttribute("data-airplan-theme", sample.select);
    }
    await page.emulateMedia({ media: "print" });
    await expect(page.locator("html")).toHaveCSS("color-scheme", "light");
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await expect(page.locator("body")).toHaveCSS("color", "rgb(31, 35, 40)");
    await expect(page.locator(".chroma .k, .chroma .kd").first()).toHaveCSS(
      "color",
      "rgb(207, 34, 46)",
    );
    await expect(page.locator("pre.mermaid svg").first()).toHaveAttribute(
      "data-mermaid-theme",
      "github-light",
    );
    await page.screenshot({
      path: testInfo.outputPath(`print-${sample.name}.png`),
      fullPage: true,
    });
  }
});
