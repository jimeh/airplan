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

let baseURL;
let fixtureSource;
let server;
let tempRoot;

const test = base.extend({
  page: async ({ page }, use) => {
    const errors = [];
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

  await execFileAsync(
    'go',
    [
      'run',
      '.',
      'preview',
      '--no-external-assets',
      '--repo',
      'none',
      '--output',
      outputPath,
      fixturePath,
    ],
    { cwd: repoRoot, env },
  );
  const html = await readFile(outputPath);

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
  expect(
    await page.evaluate((scheme) => (
      window.matchMedia(`(prefers-color-scheme: ${scheme})`).matches
    ), dark ? 'dark' : 'light'),
  ).toBe(true);

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
