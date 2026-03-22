import { defineConfig, devices } from '@playwright/test'

const BASE_URL = 'https://localhost:9001'

export default defineConfig({
  testDir: './e2e',
  testMatch: /federation.*\.spec\.ts/,
  fullyParallel: false,
  forbidOnly: !!process.env['CI'],
  retries: process.env['CI'] ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { outputFolder: 'e2e/playwright-report-federation', open: 'never' }]],
  outputDir: 'e2e/test-results-federation',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    actionTimeout: 10_000,
    ignoreHTTPSErrors: true,
  },
  globalSetup: './e2e/federation-setup.ts',
  globalTeardown: './e2e/federation-teardown.ts',
  projects: [
    {
      name: 'federation-auth',
      testMatch: /federation-auth\.setup\.ts/,
    },
    {
      name: 'federation',
      use: {
        ...devices['Desktop Chrome'],
        storageState: './e2e/.auth/federation-superuser.json',
        ignoreHTTPSErrors: true,
      },
      dependencies: ['federation-auth'],
    },
  ],
  webServer: {
    command: `pnpm exec quasar dev -p 9001`,
    url: BASE_URL,
    reuseExistingServer: true,
    ignoreHTTPSErrors: true,
    timeout: 120_000,
    env: {
      BACKEND_URL: 'http://localhost:9081',
    },
  },
})
