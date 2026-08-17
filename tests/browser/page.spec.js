import { execFile } from 'node:child_process';
import { existsSync } from 'node:fs';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { expect, test as base } from '@playwright/test';

import {
  binaryPath, cleanEnv, fixtureBinaryPath, repoRoot,
} from './airplan-binary.js';

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const fixturePath = join(here, 'testdata', 'smoke.md');
const sourceFixturePath = join(
  repoRoot,
  'airplan',
  'testdata',
  'TestRenderMarkdownGolden',
  'upload_example_go.html',
);
const expectedCode = 'const answer = 42;\nconsole.log(answer);\n';
const mermaidModule = `
let theme = 'default';
function escapeHTML(value) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}
export default {
  initialize(config) {
    theme = config.theme;
  },
  async render(id, source) {
    const name = theme === 'dark' ? 'dark' : 'light';
    return {
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="240"' +
        ' viewBox="0 0 640 240" role="img" data-mermaid-theme="' + name +
        '" id="' + id + '" aria-labelledby="' + id + '-title ' + id +
        '-label" aria-describedby="' + id + '-desc">' +
        '<title id="' + id + '-title">Diagram</title>' +
        '<desc id="' + id + '-desc">Rendered Mermaid fixture</desc>' +
        '<defs><marker id="' + id + '-arrow"><path d="M0 0L10 5L0 10z"/>' +
        '</marker><linearGradient id="' + id + '-gradient"><stop offset="0"/>' +
        '</linearGradient></defs><style>#' + id + '-node{fill:url(#' + id +
        '-gradient)}#' + id + '-label{font-weight:600}</style>' +
        '<g id="' + id + '-node" aria-labelledby="' + id + '-label">' +
        '<path d="M20 120H600" marker-end="url(#' + id + '-arrow)"/>' +
        '<a href="#' + id + '-node"><text id="' + id + '-label" x="24"' +
        ' y="112">' + escapeHTML(source) + '</text></a></g></svg>',
    };
  },
};
`;

let baseURL;
let collectionURL;
let collectionMembers;
let fixtureSource;
let mermaidURL;
let server;
let sourceURL;
let tempRoot;
let collectionHTML;
let revisionHTML;
const versionRequests = [];

function isVersionManifestURL(url) {
  try {
    return new URL(url).pathname.endsWith('/.airplan-versions.json');
  } catch {
    return false;
  }
}

const test = base.extend({
  page: async ({ page }, use) => {
    const errors = [];
    await page.route(mermaidURL, (route) => route.fulfill({
      body: mermaidModule,
      contentType: 'text/javascript; charset=utf-8',
    }));
    page.on('pageerror', (error) => {
      errors.push(`page error: ${error.message}`);
    });
    page.on('response', (response) => {
      if (response.status() !== 404) return;
      if (isVersionManifestURL(response.url())) return;
      errors.push(`response error: 404 ${response.url()}`);
    });
    page.on('console', (message) => {
      if (message.type() === 'error') {
        if (message.text() ===
            'Failed to load resource: the server responded with a status of 404 (Not Found)') {
          return;
        }
        errors.push(`console error: ${message.text()}`);
      }
    });

    await use(page);
    expect(errors, 'the rendered page emitted browser errors').toEqual([]);
  },
});

test.beforeAll(async () => {
  if (!existsSync(binaryPath)) {
    throw new Error(
      `${binaryPath} is missing; it is built by ` +
      'tests/browser/global-setup.js, which runs only when Playwright ' +
      'uses this repository\'s playwright.config.js',
    );
  }
  tempRoot = await mkdtemp(join(tmpdir(), 'airplan-browser-'));
  fixtureSource = await readFile(fixturePath, 'utf8');
  const outputPath = join(tempRoot, 'index.html');
  const collectionOutputPath = join(tempRoot, 'collection.html');
  const revisionOutputPath = join(tempRoot, 'revision.html');
  const configRoot = join(tempRoot, 'config');
  const env = cleanEnv();
  env.XDG_CONFIG_HOME = configRoot;

  // The binary is built once by the global setup; running it here keeps
  // this hook well inside its timeout even on a cold Go cache.
  await execFileAsync(
    binaryPath,
    [
      'preview',
      '--repo',
      'none',
      '--output',
      outputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  await execFileAsync(fixtureBinaryPath, [revisionOutputPath], {
    cwd: repoRoot, env,
  });
  await writeFile(join(tempRoot, 'shot.svg'),
    '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">' +
    '<rect width="20" height="20" fill="green"/></svg>');
  await writeFile(join(tempRoot, 'demo.webm'), 'video fixture');
  await writeFile(join(tempRoot, 'sound.ogg'), 'audio fixture');
  await writeFile(join(tempRoot, 'notes.bin'), 'generic fixture');
  await execFileAsync(
    binaryPath,
    [
      'preview', '--files', '--repo', 'none', '--title',
      '<Evidence & results>', '--output', collectionOutputPath,
      join(tempRoot, 'shot.svg'), join(tempRoot, 'demo.webm'),
      join(tempRoot, 'sound.ogg'), join(tempRoot, 'notes.bin'),
    ],
    { cwd: repoRoot, env },
  );
  const html = await readFile(outputPath);
  collectionHTML = await readFile(collectionOutputPath);
  revisionHTML = await readFile(revisionOutputPath);
  collectionMembers = new Map([
    ['/demo.webm', await readFile(join(tempRoot, 'demo.webm'))],
    ['/sound.ogg', await readFile(join(tempRoot, 'sound.ogg'))],
    ['/notes.bin', await readFile(join(tempRoot, 'notes.bin'))],
  ]);
  const sourceHTML = await readFile(sourceFixturePath);
  const match = html.toString().match(/await import\("([^"]+)"\)/);
  if (!match) throw new Error('rendered fixture has no Mermaid module URL');
  [, mermaidURL] = match;

  server = createServer(async (request, response) => {
    let body;
    if (request.url === '/') {
      body = html;
    } else if (request.url === '/source') {
      body = sourceHTML;
    } else if (request.url === `/${'r'.repeat(26)}/plan.html`) {
      body = revisionHTML;
    } else if (request.url === `/${'r'.repeat(26)}/.airplan-changes.diff`) {
      response.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
      response.end('--- revision-1/plan.md\n+++ revision-2/plan.md\n');
      return;
    } else if (/^\/[a-z2-7]{26}\/plan\.html$/.test(request.url)) {
      body = html;
    } else if (request.url === '/collection') {
      body = collectionHTML;
    } else if (request.url === '/shot.svg') {
      body = await readFile(join(tempRoot, 'shot.svg'));
      response.writeHead(200, { 'Content-Type': 'image/svg+xml' });
      response.end(body);
      return;
    } else if (collectionMembers.has(request.url)) {
      body = collectionMembers.get(request.url);
      response.writeHead(200, { 'Content-Type': 'application/octet-stream' });
      response.end(body);
      return;
    } else if (request.url.startsWith('/.airplan-versions.json?')) {
      versionRequests.push({ url: request.url, headers: request.headers });
      response.writeHead(404).end();
      return;
    } else {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    response.end(body);
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const address = server.address();
  baseURL = `http://127.0.0.1:${address.port}`;
  collectionURL = `${baseURL}/collection`;
  sourceURL = `${baseURL}/source`;
});

test('collection overview presents and links every media kind',
  async ({ context, page }) => {
    await context.grantPermissions(
      ['clipboard-read', 'clipboard-write'], { origin: baseURL },
    );
    await page.goto(collectionURL);
    await expect(page).toHaveTitle('<Evidence & results>');
    await expect(page.getByRole('heading', {
      level: 1, name: '<Evidence & results>',
    })).toBeVisible();
    await expect(page.locator('ol.files > li.file')).toHaveCount(4);
    await expect(page.locator('img[loading="lazy"]')).toHaveCount(1);
    const imageFile = page.locator('.file', {
      has: page.getByRole('heading', { name: 'shot.svg' }),
    });
    const imagePreview = imageFile.locator('.preview--image > a');
    await expect(imagePreview).toHaveAttribute('href', './shot.svg');
    await expect(imageFile.getByRole('link', { name: 'Open' }))
      .toHaveAttribute('href', './shot.svg');
    await expect(imagePreview).toHaveAccessibleName('shot.svg');
    await expect(page.locator('video[controls]:not([autoplay])')).toHaveCount(1);
    await expect(page.locator('audio[controls]:not([autoplay])')).toHaveCount(1);
    await expect(page.locator('.file .preview')).toHaveCount(3);
    const visualMediaGaps = await page.locator(
      '.preview img, .preview video',
    ).evaluateAll((media) => media.map((element) => {
      const frame = element.closest('.preview').getBoundingClientRect();
      const content = element.getBoundingClientRect();
      return {
        top: content.top - frame.top,
        right: frame.right - content.right,
        bottom: frame.bottom - content.bottom,
        left: content.left - frame.left,
      };
    }));
    for (const gaps of visualMediaGaps) {
      expect(gaps).toEqual({ top: 0, right: 0, bottom: 0, left: 0 });
    }
    await expect(page.locator('.preview img')).toHaveCSS(
      'max-height', 'none',
    );
    await expect(page.locator('.preview video')).toHaveCSS(
      'object-fit', 'contain',
    );
    await expect(page.locator('.preview--audio')).toHaveCSS(
      'border-top-width', '1px',
    );
    await expect(page.locator('.file', {
      has: page.getByRole('heading', { name: 'notes.bin' }),
    }).locator('.preview')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'notes.bin' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Open' })).toHaveCount(4);
    await expect(page.getByRole('link', { name: 'Download' })).toHaveCount(4);
    await page.keyboard.press('Tab');
    const overviewCopy = page.getByRole('button', {
      name: 'Copy page link',
    });
    await expect(overviewCopy).toBeFocused();
    await expect(overviewCopy).toHaveCSS('outline-style', 'solid');
    await expect(overviewCopy).toHaveCSS('outline-width', '2px');
    await page.locator('[data-copy="./notes.bin"]').click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(`${baseURL}/notes.bin`);
    await overviewCopy.click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(collectionURL);
    const overflow = await page.evaluate(() => (
      document.documentElement.scrollWidth > document.documentElement.clientWidth
    ));
    expect(overflow).toBe(false);
  });

test('collection overview shares document theme controls',
  async ({ page }) => {
    await page.goto(collectionURL);
    const root = page.locator('html');
    const lightTheme = page.getByRole('button', { name: 'Light theme' });
    const systemTheme = page.getByRole('button', { name: 'System theme' });
    const darkTheme = page.getByRole('button', { name: 'Dark theme' });

    await expect(systemTheme).toHaveAttribute('aria-pressed', 'true');
    await expect(root).not.toHaveAttribute('data-theme', /.+/);

    await darkTheme.click();
    await expect(root).toHaveAttribute('data-theme', 'dark');
    await expect(darkTheme).toHaveAttribute('aria-pressed', 'true');
    await expect.poll(() => page.evaluate(
      () => localStorage.getItem('airplan-theme'),
    )).toBe('dark');

    await page.reload();
    await expect(root).toHaveAttribute('data-theme', 'dark');
    await expect(darkTheme).toHaveAttribute('aria-pressed', 'true');

    await lightTheme.click();
    await expect(root).toHaveAttribute('data-theme', 'light');
    await expect(lightTheme).toHaveAttribute('aria-pressed', 'true');

    await systemTheme.click();
    await expect(root).not.toHaveAttribute('data-theme', /.+/);
    await expect(systemTheme).toHaveAttribute('aria-pressed', 'true');
    await expect.poll(() => page.evaluate(
      () => localStorage.getItem('airplan-theme'),
    )).toBeNull();
  });

test('built-in pages share canonical toolbar control styling',
  async ({ page }) => {
    const controlStyles = async () => page.evaluate(() => {
      const toolbar = document.querySelector('.toolbar');
      const toggle = toolbar.querySelector('.themetoggle');
      const button = toggle.querySelector('[data-theme="system"]');
      const icon = button.querySelector('.icon');
      const action = toolbar.querySelector('.toolbar-actions button');
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

test('toolbar controls do not transition during theme changes',
  async ({ page }) => {
    for (const url of [baseURL, collectionURL]) {
      await page.goto(url);
      const controls = page.locator('.toolbar a, .toolbar button');
      await expect.poll(async () => controls.evaluateAll((elements) => (
        elements.every((element) => (
          getComputedStyle(element).transitionDuration === '0s'
        ))
      ))).toBe(true);
      await page.getByRole('button', { name: 'Dark theme' }).click();
      await expect.poll(async () => controls.evaluateAll((elements) => (
        elements.every((element) => (
          getComputedStyle(element).transitionDuration === '0s'
        ))
      ))).toBe(true);
    }
  });

test.afterAll(async () => {
  if (server) {
    await new Promise((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
});

test('standalone Markdown performs cache-busted dormant revision discovery',
  async ({ page }) => {
    const start = versionRequests.length;
    await page.goto(baseURL);
    await expect.poll(() => versionRequests.length).toBe(start + 1);
    await page.reload();
    await expect.poll(() => versionRequests.length).toBe(start + 2);

    const requests = versionRequests.slice(start);
    const nonces = requests.map(({ url }) => (
      new URL(url, baseURL).searchParams.get('_airplan')
    ));
    expect(nonces[0]).toBeTruthy();
    expect(nonces[1]).toBeTruthy();
    expect(nonces[1]).not.toBe(nonces[0]);
    for (const request of requests) {
      expect(request.headers['cache-control']).toContain('no-cache');
    }
  });

test('revision metadata renders a compact picker and stale notice',
  async ({ page }) => {
    const dirs = ['a'.repeat(26), 'b'.repeat(26), 'c'.repeat(26)];
    const revisions = dirs.map((dir, index) => ({
      number: index + 1,
      url: `${baseURL}/${dir}/plan.html`,
      created_at: `2026-08-15T10:${String(index * 10).padStart(2, '0')}:00Z`,
      ...(index === 0 ? {} : {
        diff_url: `${baseURL}/${dir}/.airplan-changes.diff`,
      }),
    }));
    await page.route('**/.airplan-versions.json?*', (route) => {
      const requestURL = new URL(route.request().url());
      const currentDir = requestURL.pathname.split('/').at(-2);
      const currentRevision = dirs.indexOf(currentDir) + 1;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          schema: 'airplan-versions', version: 1,
          chain_id: 'd'.repeat(26), current_revision: currentRevision,
          latest_revision: 3, last_assigned_revision: 3, revisions,
        }),
      });
    });
    await page.goto(`${baseURL}/${dirs[0]}/plan.html`);
    const picker = page.getByRole('combobox', { name: 'Document revision' });
    await expect(picker).toHaveValue(revisions[0].url);
    await expect(picker.locator('option')).toHaveCount(3);
    await expect(picker.locator('option')).toHaveText([
      'Revision 1 of 3', 'Revision 2 of 3', 'Revision 3 (Latest)',
    ]);
    await expect(page.getByRole('link', { name: 'Previous' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Next' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: /Latest: revision/ }))
      .toHaveCount(0);
    await expect(page.locator('.toolbar').getByRole('combobox'))
      .toHaveCount(0);
    const heading = page.locator('[data-revision-heading]');
    await expect(heading.getByRole('combobox')).toBeVisible();
    await expect(page.locator('.revision-picker-label'))
      .toHaveText('Revision 1 of 3');
    await expect(page.locator('.revision-picker-label'))
      .toHaveAttribute('aria-hidden', 'true');
    await expect(heading).toHaveClass(/is-stale/);
    const coverage = await heading.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const select = element.querySelector('select').getBoundingClientRect();
      const chevron = getComputedStyle(element, '::after');
      return {
        top: select.top - bounds.top,
        right: bounds.right - select.right,
        bottom: bounds.bottom - select.bottom,
        left: select.left - bounds.left,
        chevronWidth: Number.parseFloat(chevron.width),
        width: bounds.width,
      };
    });
    for (const edge of ['top', 'right', 'bottom', 'left']) {
      expect(coverage[edge]).toBeCloseTo(0, 0);
    }
    expect(coverage.chevronWidth).toBeCloseTo(6, 0);
    expect(coverage.width).toBeLessThan(180);
    await Promise.all([
      page.waitForURL(revisions[2].url),
      picker.selectOption(revisions[2].url),
    ]);
    await expect(page.locator('.revision-picker-label'))
      .toHaveText('Revision 3 (Latest)');
    await expect(page.locator('[data-revision-heading]'))
      .not.toHaveClass(/is-stale/);
  });

test('revision metadata rejects same-origin URLs outside the current key prefix',
  async ({ page }) => {
    const currentDir = 'e'.repeat(26);
    const otherDir = 'f'.repeat(26);
    await page.route('**/.airplan-versions.json?*', (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'airplan-versions', version: 1,
        chain_id: 'g'.repeat(26), current_revision: 1,
        latest_revision: 2, last_assigned_revision: 2,
        revisions: [
          {
            number: 1,
            url: `${baseURL}/${currentDir}/plan.html`,
            created_at: '2026-08-15T10:00:00Z',
          },
          {
            number: 2,
            url: `${baseURL}/other/${otherDir}/plan.html`,
            created_at: '2026-08-15T10:10:00Z',
            diff_url: `${baseURL}/other/${otherDir}/.airplan-changes.diff`,
          },
        ],
      }),
    }));
    await page.goto(`${baseURL}/${currentDir}/plan.html`);
    await expect(page.getByRole('combobox', { name: 'Document revision' }))
      .toHaveCount(0);
    await expect(page.locator('.revision-heading.is-picker')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Browser smoke plan' }))
      .toBeVisible();
  });

test('valid revision metadata with one live member labels it as latest',
  async ({ page }) => {
    const currentDir = 'h'.repeat(26);
    const warnings = [];
    page.on('console', (message) => {
      if (message.type() === 'warning') warnings.push(message.text());
    });
    await page.route('**/.airplan-versions.json?*', (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'airplan-versions', version: 1,
        chain_id: 'i'.repeat(26), current_revision: 2,
        latest_revision: 2, last_assigned_revision: 2,
        revisions: [
          {
            number: 1, deleted: true,
            deleted_at: '2026-08-15T10:20:00Z',
          },
          {
            number: 2,
            url: `${baseURL}/${currentDir}/plan.html`,
            created_at: '2026-08-15T10:10:00Z',
            diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
          },
        ],
      }),
    }));
    await page.goto(`${baseURL}/${currentDir}/plan.html`);
    const picker = page.getByRole('combobox', { name: 'Document revision' });
    await expect(picker).toBeVisible();
    await expect(picker.locator('option')).toHaveText(['Revision 2 (Latest)']);
    await expect(page.locator('.revision-picker-label'))
      .toHaveText('Revision 2 (Latest)');
    await expect(page.locator('[data-revision-heading]'))
      .not.toHaveClass(/is-stale/);
    expect(warnings).toEqual([]);
  });

test('empty revision metadata fails closed without breaking the document',
  async ({ page }) => {
    const currentDir = 'j'.repeat(26);
    const warnings = [];
    page.on('console', (message) => {
      if (message.type() === 'warning') warnings.push(message.text());
    });
    await page.route('**/.airplan-versions.json?*', (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'airplan-versions', version: 1,
        chain_id: 'k'.repeat(26), current_revision: 1,
        latest_revision: 0, last_assigned_revision: 0, revisions: [],
      }),
    }));
    await page.goto(`${baseURL}/${currentDir}/plan.html`);
    await expect(page.getByRole('combobox', { name: 'Document revision' }))
      .toHaveCount(0);
    await expect(page.locator('.revision-heading.is-picker')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Browser smoke plan' }))
      .toBeVisible();
    await expect.poll(() => warnings).toContain(
      'airplan: revision metadata is unavailable or invalid',
    );
  });

test('revision Changes view switches and exposes its adjacent raw diff',
  async ({ page }) => {
    const firstDir = 'q'.repeat(26);
    const currentDir = 'r'.repeat(26);
    const revisions = [
      {
        number: 1,
        url: `${baseURL}/${firstDir}/plan.html`,
        created_at: '2026-08-15T10:00:00Z',
      },
      {
        number: 2,
        url: `${baseURL}/${currentDir}/plan.html`,
        created_at: '2026-08-15T10:10:00Z',
        diff_url: `${baseURL}/${currentDir}/.airplan-changes.diff`,
      },
    ];
    await page.route('**/.airplan-versions.json?*', (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'airplan-versions', version: 1,
        chain_id: 's'.repeat(26), current_revision: 2,
        latest_revision: 2, last_assigned_revision: 2, revisions,
      }),
    }));
    await page.goto(revisions[1].url);
    await page.setViewportSize({ width: 700, height: 800 });
    const toolbarLayout = await page.locator('.toolbar').evaluate((element) => {
      const view = element.querySelector('.viewtoggle').getBoundingClientRect();
      const files = element.querySelector('.file-actions').getBoundingClientRect();
      const theme = element.querySelector('.themetoggle').getBoundingClientRect();
      return {
        display: getComputedStyle(element).display,
        viewCenter: view.top + view.height / 2,
        themeCenter: theme.top + theme.height / 2,
        firstRowBottom: Math.max(view.bottom, theme.bottom),
        filesTop: files.top,
      };
    });
    expect(toolbarLayout.display).toBe('grid');
    expect(toolbarLayout.viewCenter).toBeCloseTo(toolbarLayout.themeCenter, 0);
    expect(toolbarLayout.filesTop).toBeGreaterThan(toolbarLayout.firstRowBottom);
    const changesButton = page.getByRole('button', { name: 'Changes view' });
    await expect(changesButton).toBeVisible();
    await changesButton.click();
    await expect(page.locator('#changes')).toBeVisible();
    await expect(page.locator('#rendered')).toBeHidden();
    await expect(page.locator('#changes')).toContainText('Changes from revision 1');
    await expect(page.locator('#changes')).toContainText('-Original');
    await expect(page.locator('#changes')).toContainText('+Revised');
    await expect(page.getByRole('link', { name: 'Open raw diff' }))
      .toHaveAttribute('href', './.airplan-changes.diff');
    await page.emulateMedia({ media: 'print' });
    await expect(page.locator('#changes')).toBeHidden();
    await expect(page.locator('#rendered')).toBeVisible();
    await page.emulateMedia({ media: 'screen' });
    await page.getByRole('button', { name: 'Rendered view' }).click();
    await expect(page.locator('#rendered')).toBeVisible();
    await expect(page.locator('#changes')).toBeHidden();
  });

test('rendered page controls work', async ({ context, page }, testInfo) => {
  await context.grantPermissions(
    ['clipboard-read', 'clipboard-write'],
    { origin: baseURL },
  );
  await page.goto(baseURL);

  const dark = testInfo.project.name.endsWith('-dark');
  await expect(page).toHaveTitle('Browser smoke plan');
  await expect(
    page.getByRole('heading', { level: 1, name: 'Browser smoke plan' }),
  ).toBeVisible();
  await expect(
    page.locator('#rendered').getByText('This fixture verifies'),
  ).toBeVisible();
  const toolbar = page.getByRole('navigation', { name: 'Document controls' });
  const narrow = testInfo.project.name.startsWith('narrow-');
  await expect(toolbar).toHaveCSS(
    'justify-content',
    narrow ? 'stretch' : 'flex-end',
  );
  await expect.poll(() => toolbar.evaluate((element) => (
    Array.from(element.querySelectorAll(
      '.viewtoggle, .copy-source, .download, .raw, .themetoggle',
    ))
      .filter((child) => !child.hidden)
      .map((child) => Array.from(child.classList).find((name) => (
        ['viewtoggle', 'copy-source', 'download', 'raw', 'themetoggle']
          .includes(name)
      )))
  ))).toEqual([
    'viewtoggle',
    'copy-source',
    'themetoggle',
  ]);
  const dividerDisplay = await page.locator('.themetoggle').evaluate(
    (element) => getComputedStyle(element, '::before').display,
  );
  const copyDivider = await page.locator('.copy-source').evaluate(
    (element) => getComputedStyle(element, '::before').content,
  );
  expect(copyDivider).toBe('none');
  if (narrow) {
    const alignment = await toolbar.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const view = element.querySelector('.viewtoggle')
        .getBoundingClientRect();
      const theme = element.querySelector('.themetoggle')
        .getBoundingClientRect();
      const copy = element.querySelector('.copy-source')
        .getBoundingClientRect();
      const fileActions = element.querySelector('.file-actions')
        .getBoundingClientRect();
      return {
        left: view.left - bounds.left,
        leftPadding: Number.parseFloat(styles.paddingLeft),
        right: bounds.right - theme.right,
        rightPadding: Number.parseFloat(styles.paddingRight),
        viewCenter: view.top + view.height / 2,
        themeCenter: theme.top + theme.height / 2,
        firstRowBottom: Math.max(view.bottom, theme.bottom),
        copyTop: copy.top,
        actionsLeft: fileActions.left - bounds.left,
      };
    });
    expect(alignment.left).toBeCloseTo(alignment.leftPadding, 0);
    expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
    expect(alignment.viewCenter).toBeCloseTo(alignment.themeCenter, 0);
    expect(alignment.copyTop).toBeGreaterThan(alignment.firstRowBottom);
    expect(alignment.actionsLeft).toBeCloseTo(alignment.leftPadding, 0);
    expect(dividerDisplay).toBe('none');
  } else {
    const alignment = await toolbar.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const view = element.querySelector('.viewtoggle')
        .getBoundingClientRect();
      const theme = element.querySelector('.themetoggle')
        .getBoundingClientRect();
      return {
        left: view.left - bounds.left,
        leftPadding: Number.parseFloat(styles.paddingLeft),
        right: bounds.right - theme.right,
        rightPadding: Number.parseFloat(styles.paddingRight),
      };
    });
    expect(alignment.left).toBeCloseTo(alignment.leftPadding, 0);
    expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
    expect(dividerDisplay).not.toBe('none');
    const dividerSpacing = await page.locator('.themetoggle').evaluate(
      (element) => {
        const bounds = element.getBoundingClientRect();
        const divider = getComputedStyle(element, '::before');
        const previousAction = element.previousElementSibling.lastElementChild;
        const previousLabel = previousAction.lastElementChild
          .getBoundingClientRect();
        const dividerX = bounds.left + Number.parseFloat(divider.left);
        return {
          before: dividerX - previousLabel.right,
          after: bounds.left - dividerX,
        };
      },
    );
    expect(dividerSpacing.before).toBeCloseTo(dividerSpacing.after, 0);
  }
  expect(
    await page.evaluate((scheme) => (
      window.matchMedia(`(prefers-color-scheme: ${scheme})`).matches
    ), dark ? 'dark' : 'light'),
  ).toBe(true);
  const theme = await page.evaluate(() => {
    const styles = window.getComputedStyle(document.body);
    const brightness = (value) => (
      value.match(/\d+/g).slice(0, 3).map(Number)
        .reduce((sum, channel) => sum + channel, 0)
    );
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

  const lightTheme = page.getByRole('button', { name: 'Light theme' });
  const systemTheme = page.getByRole('button', { name: 'System theme' });
  const darkTheme = page.getByRole('button', { name: 'Dark theme' });
  const diagram = page.locator('pre.mermaid svg').first();
  await expect(systemTheme).toHaveAttribute('aria-pressed', 'true');
  await expect(diagram).toHaveAttribute(
    'data-mermaid-theme', dark ? 'dark' : 'light',
  );

  await lightTheme.click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(lightTheme).toHaveAttribute('aria-pressed', 'true');
  await expect(diagram).toHaveAttribute('data-mermaid-theme', 'light');
  await expect(page.locator('.chroma .nx').first()).toHaveCSS(
    'color', 'rgb(31, 35, 40)',
  );

  await darkTheme.click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(darkTheme).toHaveAttribute('aria-pressed', 'true');
  await expect(diagram).toHaveAttribute('data-mermaid-theme', 'dark');
  await expect(page.locator('.chroma .nx').first()).toHaveCSS(
    'color', 'rgb(230, 237, 243)',
  );
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(darkTheme).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('pre.mermaid svg').first()).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  await systemTheme.click();
  await expect(page.locator('html')).not.toHaveAttribute('data-theme', /.+/);
  await expect(systemTheme).toHaveAttribute('aria-pressed', 'true');

  const inlineToc = page.locator('#toc');
  await inlineToc.getByRole('link', { name: 'Details' }).click();
  await expect(page).toHaveURL(/#details$/);
  await expect(page.getByRole('heading', { name: 'Details' })).toBeVisible();

  if (testInfo.project.name.startsWith('narrow-')) {
    await expect.poll(
      () => inlineToc.evaluate((element) => (
        element.getBoundingClientRect().bottom
      )),
    ).toBeLessThan(0);
    const openToc = page.getByRole('button', {
      name: 'Open table of contents',
    });
    await expect(openToc).toHaveAttribute('aria-hidden', 'false');
    await openToc.click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole('heading', { name: 'Contents' }),
    ).toBeVisible();
    await dialog.getByRole('link', { name: 'Code sample' }).click();
    await expect(dialog).toBeHidden();
    await expect(page).toHaveURL(/#code-sample$/);
  }

  const renderedButton = page.getByRole('button', { name: 'Rendered view' });
  const sourceButton = page.getByRole('button', { name: 'Source view' });
  await expect(renderedButton).toHaveAttribute('aria-pressed', 'true');
  await expect(sourceButton).toHaveAttribute('aria-pressed', 'false');
  await sourceButton.click();
  await expect(sourceButton).toHaveAttribute('aria-pressed', 'true');
  await expect(renderedButton).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('#source')).toBeVisible();
  await expect(page.locator('#rendered')).toBeHidden();

  await page.getByRole('button', { name: 'Copy markdown' }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(fixtureSource);

  await renderedButton.click();
  await expect(page.locator('#rendered')).toBeVisible();
  const copyCode = page.getByRole('button', { name: 'Copy code' }).first();
  await copyCode.click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(expectedCode);
});

test('Mermaid viewer is shared, theme-aware, and responsive',
  async ({ page }, testInfo) => {
    await page.goto(baseURL);

    const triggers = page.getByRole('button', { name: 'Open diagram viewer' });
    await expect(triggers).toHaveCount(2);
    const firstTrigger = triggers.first();
    await page.locator('pre.mermaid').first().hover();
    await expect(firstTrigger).toHaveCSS('opacity', '1');
    await firstTrigger.focus();
    await expect(firstTrigger).toBeFocused();
    await firstTrigger.click();

    const dialog = page.locator('[data-airplan-mermaid-dialog]');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('Mermaid diagram')).toBeVisible();
    await expect(dialog.locator('[data-airplan-mermaid-canvas]'))
      .toBeFocused();
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
      testInfo.project.name.endsWith('-dark')
        ? 'rgb(13, 17, 23)'
        : 'rgb(255, 255, 255)',
    );

    const inlineIDs = await page.locator('pre.mermaid svg').first()
      .locator('[id]').evaluateAll((elements) => elements.map((el) => el.id));
    const clone = dialog.locator('.mermaid-surface > svg');
    const cloneData = await clone.evaluate((svg) => ({
      ids: Array.from(svg.querySelectorAll('[id]'), (el) => el.id),
      marker: svg.querySelector('[marker-end]').getAttribute('marker-end'),
      href: svg.querySelector('a').getAttribute('href'),
      labelledBy: svg.getAttribute('aria-labelledby').split(/\s+/),
      describedBy: svg.getAttribute('aria-describedby').split(/\s+/),
      nestedLabelledBy: svg.querySelector('g').getAttribute('aria-labelledby'),
      style: svg.querySelector('style').textContent,
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
      if (id.endsWith('-node') || id.endsWith('-label')) {
        expect(cloneData.style).toContain(`#${id}`);
      }
    }

    await dialog.getByRole('button', { name: 'Close diagram viewer' }).click();
    await expect(dialog).toBeHidden();
    await expect(firstTrigger).toBeFocused();
    await page.locator('pre.mermaid').nth(1).hover();
    await triggers.nth(1).click();
    await expect(dialog).toBeVisible();
    await expect(page.locator('[data-airplan-mermaid-dialog]')).toHaveCount(1);
    await expect(dialog.locator('.mermaid-surface > svg'))
      .toContainText('Source --> Render');
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
  });

test('Mermaid viewer zooms, pans, and preserves its view across themes',
  async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop-light',
      'one desktop project covers the complete interaction cluster');
    await page.goto(baseURL);
    const trigger = page.getByRole('button', {
      name: 'Open diagram viewer',
    }).first();
    await page.locator('pre.mermaid').first().hover();
    await trigger.click();

    const dialog = page.locator('[data-airplan-mermaid-dialog]');
    const canvas = dialog.locator('[data-airplan-mermaid-canvas]');
    const surface = dialog.locator('[data-airplan-mermaid-surface]');
    const zoom = dialog.locator('[data-airplan-mermaid-zoom-value]');
    const initialPercent = Number.parseInt(await zoom.textContent(), 10);
    expect(initialPercent).toBeLessThanOrEqual(100);

    await dialog.getByRole('button', { name: 'Zoom in' }).click();
    await expect(zoom).toHaveText(`${Math.round(initialPercent * 1.25)}%`);
    await dialog.getByRole('button', { name: 'Zoom out' }).click();
    await expect(zoom).toHaveText(`${initialPercent}%`);

    const bounds = await canvas.boundingBox();
    await canvas.dispatchEvent('wheel', {
      deltaY: -100,
      clientX: bounds.x + bounds.width * 0.75,
      clientY: bounds.y + bounds.height * 0.25,
    });
    await expect(zoom).toHaveText(`${Math.round(initialPercent * 1.2)}%`);
    const anchoredTransform = await surface.getAttribute('style');
    expect(anchoredTransform).not.toContain('translate(0px, 0px)');

    await canvas.focus();
    await page.keyboard.press('ArrowRight');
    const keyboardTransform = await surface.getAttribute('style');
    expect(keyboardTransform).not.toBe(anchoredTransform);
    await page.keyboard.press('Shift+ArrowDown');
    const shiftedTransform = await surface.getAttribute('style');
    expect(shiftedTransform).not.toBe(keyboardTransform);

    await canvas.dispatchEvent('pointerdown', {
      button: 0, pointerId: 7, clientX: 200, clientY: 200,
    });
    await canvas.dispatchEvent('pointermove', {
      pointerId: 7, clientX: 260, clientY: 230,
    });
    await canvas.dispatchEvent('pointerup', {
      pointerId: 7, clientX: 260, clientY: 230,
    });
    const draggedTransform = await surface.getAttribute('style');
    expect(draggedTransform).not.toBe(shiftedTransform);

    const viewerSVG = dialog.locator('.mermaid-surface > svg');
    const svgTransform = await viewerSVG.getAttribute('style');
    const oldCloneID = await viewerSVG.getAttribute('id');
    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'dark';
      window.dispatchEvent(new CustomEvent('airplan:themechange', {
        detail: { theme: 'dark' },
      }));
    });
    await expect(viewerSVG).toHaveAttribute(
      'data-mermaid-theme', 'dark',
    );
    await expect(viewerSVG).not.toHaveAttribute('id', oldCloneID);
    await expect(surface).toHaveAttribute('style', draggedTransform);
    await expect(viewerSVG).toHaveAttribute('style', svgTransform);

    await page.keyboard.press('+');
    await expect(zoom).not.toHaveText(`${Math.round(initialPercent * 1.2)}%`);
    for (let index = 0; index < 40; index += 1) {
      await page.keyboard.press('-');
    }
    await expect(zoom).toHaveText('5%');
    for (let index = 0; index < 40; index += 1) {
      await page.keyboard.press('+');
    }
    await expect(zoom).toHaveText('800%');
    await page.keyboard.press('0');
    await expect(zoom).toHaveText(`${initialPercent}%`);
    await expect(surface).toHaveAttribute(
      'style', 'transform: translate(0px, 0px);',
    );

    await page.mouse.click(2, 2);
    await expect(dialog).toBeHidden();
    await expect(trigger).toBeFocused();
  });

test('Mermaid failures preserve source without viewer controls',
  async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop-light',
      'one desktop project covers the progressive-enhancement fallback');
    await page.unroute(mermaidURL);
    await page.route(mermaidURL, (route) => route.fulfill({
      body: `export default {
        initialize() {},
        async render() { throw new Error('fixture render failure'); }
      };`,
      contentType: 'text/javascript; charset=utf-8',
    }));
    await page.goto(baseURL);

    const diagrams = page.locator('pre.mermaid');
    await expect(diagrams).toHaveCount(2);
    await expect(diagrams.first()).toHaveClass(/mermaid-failed/);
    await expect(diagrams.first()).toContainText('flowchart LR');
    await expect(page.getByRole('button', {
      name: 'Open diagram viewer',
    })).toHaveCount(0);
    await expect(page.locator('[data-airplan-mermaid-dialog]')).toHaveCount(0);

    await page.unroute(mermaidURL);
    await page.route(mermaidURL, (route) => route.fulfill({
      body: 'this is not valid JavaScript',
      contentType: 'text/javascript; charset=utf-8',
    }));
    await page.reload();
    await expect(diagrams.first()).not.toHaveClass(/mermaid-rendered/);
    await expect(diagrams.first()).toContainText('flowchart LR');
    await expect(page.getByRole('button', {
      name: 'Open diagram viewer',
    })).toHaveCount(0);
    await expect(page.locator('[data-airplan-mermaid-dialog]')).toHaveCount(0);
  });

test('Mermaid viewer requires native modal dialog support',
  async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop-light',
      'one desktop project covers dialog feature detection');
    await page.addInitScript(() => {
      HTMLDialogElement.prototype.showModal = undefined;
    });
    await page.goto(baseURL);

    await expect(page.locator('pre.mermaid svg')).toHaveCount(2);
    await expect(page.getByRole('button', {
      name: 'Open diagram viewer',
    })).toHaveCount(0);
    await expect(page.locator('[data-airplan-mermaid-dialog]')).toHaveCount(0);
  });

test('revision 404 filtering does not match unrelated resources', () => {
  expect(isVersionManifestURL(
    'https://plans.example.com/id/.airplan-versions.json?nonce=1',
  )).toBe(true);
  expect(isVersionManifestURL(
    'https://plans.example.com/id/missing.png',
  )).toBe(false);
  expect(isVersionManifestURL(
    'https://plans.example.com/id/missing.png?redirect=/.airplan-versions.json?nonce=1',
  )).toBe(false);
});

test('uploaded source controls share the first row on narrow screens',
  async ({ page }, testInfo) => {
    test.skip(!testInfo.project.name.startsWith('narrow-'),
      'the regression only applies to the narrow toolbar grid');

    await page.goto(sourceURL);

    const toolbar = page.getByRole('navigation', {
      name: 'Document controls',
    });
    await expect(toolbar.locator('.viewtoggle')).toHaveCount(0);
    await expect(toolbar.getByRole('link', { name: 'Download source' }))
      .toBeVisible();
    await expect(toolbar.getByRole('link', { name: 'Open raw source' }))
      .toBeVisible();

    const alignment = await toolbar.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const actions = element.querySelector('.file-actions')
        .getBoundingClientRect();
      const theme = element.querySelector('.themetoggle')
        .getBoundingClientRect();
      return {
        actionsCenter: actions.top + actions.height / 2,
        actionsLeft: actions.left - bounds.left,
        leftPadding: Number.parseFloat(styles.paddingLeft),
        right: bounds.right - theme.right,
        rightPadding: Number.parseFloat(styles.paddingRight),
        themeCenter: theme.top + theme.height / 2,
      };
    });
    expect(alignment.actionsCenter).toBeCloseTo(alignment.themeCenter, 0);
    expect(alignment.actionsLeft).toBeCloseTo(alignment.leftPadding, 0);
    expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
  });

test('print view is compact and expands disclosures', async ({ browser, page },
  testInfo) => {
  test.skip(!testInfo.project.name.startsWith('desktop-'),
    'desktop projects cover both print color schemes');

  await page.goto(baseURL);
  const frontmatter = page.locator('.frontmatter');
  const disclosure = page.locator('#print-disclosure');
  await expect(frontmatter).not.toHaveAttribute('open', '');
  await expect(frontmatter.getByText('Print coverage')).toBeHidden();
  await expect(disclosure).not.toHaveAttribute('open', '');
  await expect(disclosure.getByText('Print must include')).toBeHidden();

  const darkTheme = page.getByRole('button', { name: 'Dark theme' });
  await darkTheme.click();
  await expect(page.locator('pre.mermaid svg').first()).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('.toolbar')).toBeHidden();
  await expect(page.getByRole('button', {
    name: 'Open diagram viewer',
  }).first()).toBeHidden();
  await expect(page.locator('[data-airplan-mermaid-dialog]'))
    .toHaveCSS('display', 'none');
  await expect(frontmatter.getByText('Print coverage')).toBeVisible();
  await expect(disclosure.getByText('Print must include')).toBeVisible();
  await expect(page.locator('body')).toHaveCSS('font-size', '14px');
  await expect(page.locator('body')).toHaveCSS('line-height', '20.3px');
  await expect(page.locator('body')).toHaveCSS(
    'background-color', 'rgb(255, 255, 255)',
  );
  await expect(page.getByRole('heading', {
    level: 1,
    name: 'Browser smoke plan',
  })).toHaveCSS('color', 'rgb(31, 35, 40)');
  await expect(page.locator('.chroma .k, .chroma .kd').first())
    .toHaveCSS('color', 'rgb(207, 34, 46)');
  await expect(page.locator('pre.mermaid svg').first()).toHaveAttribute(
    'data-mermaid-theme', 'light',
  );

  const noJSContext = await browser.newContext({
    colorScheme: testInfo.project.name.endsWith('-dark') ? 'dark' : 'light',
    javaScriptEnabled: false,
  });
  try {
    const noJSPage = await noJSContext.newPage();
    await noJSPage.goto(baseURL);
    await noJSPage.emulateMedia({ media: 'print' });
    await expect(noJSPage.locator('.frontmatter').getByText('Print coverage'))
      .toBeVisible();
    await expect(noJSPage.locator('#print-disclosure')
      .getByText('Print must include')).toBeVisible();
    for (const selector of [
      '[data-print-hidden]',
      '[data-print-script]',
      '[data-print-style]',
    ]) {
      await expect(noJSPage.locator(selector)).toHaveCount(1);
      await expect(noJSPage.locator(selector)).toBeHidden();
    }
  } finally {
    await noJSContext.close();
  }

  await page.emulateMedia({ media: 'screen' });
  await expect(page.locator('pre.mermaid svg').first()).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  const initialStates = await page.locator('details').evaluateAll(
    (details) => details.map((detail) => detail.open),
  );
  await page.evaluate(() => {
    window.printDisclosureStates = [];
    window.addEventListener('beforeprint', () => {
      window.printDisclosureStates.push(
        Array.from(document.querySelectorAll('details'))
          .map((details) => details.open),
      );
    });
    window.addEventListener('afterprint', () => {
      window.printDisclosureStates.push(
        Array.from(document.querySelectorAll('details'))
          .map((details) => details.open),
      );
    });
  });
  await page.pdf({ format: 'Letter', printBackground: true });
  const states = await page.evaluate(() => window.printDisclosureStates);
  expect(states).toHaveLength(2);
  expect(states[0]).toHaveLength(initialStates.length);
  expect(states[0].every((open) => open)).toBe(true);
  expect(states[1]).toEqual(initialStates);
  await expect(page.locator('pre.mermaid svg').first()).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  await expect(frontmatter).not.toHaveAttribute('open', '');
  await expect(disclosure).not.toHaveAttribute('open', '');
  await expect(page.locator('#print-open-disclosure')).toHaveAttribute(
    'open', '',
  );
});
