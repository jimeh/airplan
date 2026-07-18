import { execFile } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { expect, test as base } from '@playwright/test';

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..');
const fixturePath = join(here, 'testdata', 'smoke.md');
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
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="40"' +
        ' role="img" data-mermaid-theme="' + name + '" id="' + id + '">' +
        '<text x="8" y="24">' + escapeHTML(source) + '</text></svg>',
    };
  },
};
`;

let baseURL;
let fixtureSource;
let mermaidURL;
let server;
let tempRoot;

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
    page.on('console', (message) => {
      if (message.type() === 'error') {
        errors.push(`console error: ${message.text()}`);
      }
    });

    await use(page);
    expect(errors, 'the rendered page emitted browser errors').toEqual([]);
  },
});

test.beforeAll(async () => {
  tempRoot = await mkdtemp(join(tmpdir(), 'airplan-browser-'));
  fixtureSource = await readFile(fixturePath, 'utf8');
  const outputPath = join(tempRoot, 'index.html');
  const configRoot = join(tempRoot, 'config');
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([name]) => !name.startsWith('AIRPLAN_')),
  );
  env.XDG_CONFIG_HOME = configRoot;
  // Bypass the mise shim so worktree-local env cannot restore
  // AIRPLAN_* variables removed above.
  const { stdout: goPath } = await execFileAsync(
    'mise', ['which', 'go'], { cwd: repoRoot },
  );

  await execFileAsync(
    goPath.trim(),
    [
      'run',
      '.',
      'preview',
      '--repo',
      'none',
      '--output',
      outputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  const html = await readFile(outputPath);
  const match = html.toString().match(/await import\("([^"]+)"\)/);
  if (!match) throw new Error('rendered fixture has no Mermaid module URL');
  [, mermaidURL] = match;

  server = createServer((request, response) => {
    if (request.url !== '/') {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    response.end(html);
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const address = server.address();
  baseURL = `http://127.0.0.1:${address.port}`;
});

test.afterAll(async () => {
  if (server) {
    await new Promise((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
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
        actionCenter: fileActions.left + fileActions.width / 2,
        toolbarCenter: bounds.left + bounds.width / 2,
      };
    });
    expect(alignment.left).toBeCloseTo(alignment.leftPadding, 0);
    expect(alignment.right).toBeCloseTo(alignment.rightPadding, 0);
    expect(alignment.viewCenter).toBeCloseTo(alignment.themeCenter, 0);
    expect(alignment.copyTop).toBeGreaterThan(alignment.firstRowBottom);
    expect(alignment.actionCenter).toBeCloseTo(alignment.toolbarCenter, 0);
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
  const diagram = page.locator('pre.mermaid svg');
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
  await expect(page.locator('pre.mermaid svg')).toHaveAttribute(
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
  await expect(page.locator('pre.mermaid svg')).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('.toolbar')).toBeHidden();
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
  await expect(page.locator('pre.mermaid svg')).toHaveAttribute(
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
  await expect(page.locator('pre.mermaid svg')).toHaveAttribute(
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
  await expect(page.locator('pre.mermaid svg')).toHaveAttribute(
    'data-mermaid-theme', 'dark',
  );
  await expect(frontmatter).not.toHaveAttribute('open', '');
  await expect(disclosure).not.toHaveAttribute('open', '');
  await expect(page.locator('#print-open-disclosure')).toHaveAttribute(
    'open', '',
  );
});
