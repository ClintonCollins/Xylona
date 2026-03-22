import { defineConfig, devices } from '@playwright/test'

const BASE_URL = 'https://localhost:9002'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env['CI'],
  retries: process.env['CI'] ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { outputFolder: 'e2e/playwright-report', open: 'never' }]],
  outputDir: 'e2e/test-results',
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    actionTimeout: 10_000,
    ignoreHTTPSErrors: true,
  },
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: './e2e/.auth/user.json',
        ignoreHTTPSErrors: true,
      },
      dependencies: ['setup'],
    },
  ],
  webServer: {
    command: 'pnpm exec quasar dev -p 9002',
    url: BASE_URL,
    reuseExistingServer: false,
    ignoreHTTPSErrors: true,
    timeout: 120_000,
    env: {
      BACKEND_URL: 'http://localhost:9091',
    },
  },
})
