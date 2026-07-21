import { execFile } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { expect, test as base } from '@playwright/test';

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..');
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
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="40"' +
        ' role="img" data-mermaid-theme="' + name + '" id="' + id + '">' +
        '<text x="8" y="24">' + escapeHTML(source) + '</text></svg>',
    };
  },
};
`;

let baseURL;
let collectionURL;
let fixtureSource;
let mermaidURL;
let server;
let sourceURL;
let tempRoot;
let collectionHTML;

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
  const collectionOutputPath = join(tempRoot, 'collection.html');
  const configRoot = join(tempRoot, 'config');
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([name]) => !name.startsWith('AIRPLAN_')),
  );
  env.XDG_CONFIG_HOME = configRoot;
  // Invoke the toolchain binary directly so a mise shim cannot restore the
  // AIRPLAN_* variables removed above.
  const { stdout: goRoot } = await execFileAsync(
    'go', ['env', 'GOROOT'], { cwd: repoRoot, env },
  );
  const goPath = join(
    goRoot.trim(),
    'bin',
    process.platform === 'win32' ? 'go.exe' : 'go',
  );

  await execFileAsync(
    goPath,
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
  await writeFile(join(tempRoot, 'shot.svg'),
    '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">' +
    '<rect width="20" height="20" fill="green"/></svg>');
  await writeFile(join(tempRoot, 'demo.webm'), 'video fixture');
  await writeFile(join(tempRoot, 'sound.ogg'), 'audio fixture');
  await writeFile(join(tempRoot, 'notes.bin'), 'generic fixture');
  await execFileAsync(
    goPath,
    [
      'run', '.', 'preview', '--files', '--repo', 'none', '--title',
      '<Evidence & results>', '--output', collectionOutputPath,
      join(tempRoot, 'shot.svg'), join(tempRoot, 'demo.webm'),
      join(tempRoot, 'sound.ogg'), join(tempRoot, 'notes.bin'),
    ],
    { cwd: repoRoot, env },
  );
  const html = await readFile(outputPath);
  collectionHTML = await readFile(collectionOutputPath);
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
    } else if (request.url === '/collection') {
      body = collectionHTML;
    } else if (request.url === '/shot.svg') {
      body = await readFile(join(tempRoot, 'shot.svg'));
      response.writeHead(200, { 'Content-Type': 'image/svg+xml' });
      response.end(body);
      return;
    } else if (request.url === '/demo.webm' ||
      request.url === '/sound.ogg' || request.url === '/notes.bin') {
      body = await readFile(join(tempRoot, request.url.slice(1)));
      response.writeHead(200, { 'Content-Type': 'application/octet-stream' });
      response.end(body);
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
    await expect(page.locator('video[controls]:not([autoplay])')).toHaveCount(1);
    await expect(page.locator('audio[controls]:not([autoplay])')).toHaveCount(1);
    await expect(page.locator('.preview--file')).toHaveCount(1);
    await expect.poll(() => page.locator('.preview--file').evaluate(
      (element) => getComputedStyle(element, '::after').content,
    )).toBe('"ATTACHED FILE"');
    await expect(page.getByRole('heading', { name: 'notes.bin' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Open' })).toHaveCount(4);
    await expect(page.getByRole('link', { name: 'Download' })).toHaveCount(4);
    await page.keyboard.press('Tab');
    const overviewCopy = page.getByRole('button', {
      name: 'Copy overview URL',
    });
    await expect(overviewCopy).toBeFocused();
    await expect(overviewCopy).toHaveCSS('outline-style', 'solid');
    await expect(overviewCopy).toHaveCSS('outline-width', '3px');
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
