import { defineConfig, devices } from '@playwright/test';

const desktop = {
  ...devices['Desktop Chrome'],
  colorScheme: 'light',
};
const narrow = {
  viewport: { width: 390, height: 844 },
  colorScheme: 'light',
};

export default defineConfig({
  testDir: './tests/browser',
  fullyParallel: false,
  workers: 1,
  reporter: [
    ['line'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  outputDir: 'test-results',
  use: {
    browserName: 'chromium',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop-light', use: desktop },
    {
      name: 'desktop-dark',
      use: { ...desktop, colorScheme: 'dark' },
    },
    { name: 'narrow-light', use: narrow },
    {
      name: 'narrow-dark',
      use: { ...narrow, colorScheme: 'dark' },
    },
  ],
});
