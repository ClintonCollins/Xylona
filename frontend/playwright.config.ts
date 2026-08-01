import { defineConfig, devices } from '@playwright/test'

const REMOTE_NODE = process.env['E2E_MODE'] === 'remote-node'
const WEB_PORT = process.env['E2E_WEB_PORT'] ?? (REMOTE_NODE ? '9003' : '9002')
const HTTP_PORT = process.env['E2E_HTTP_PORT'] ?? (REMOTE_NODE ? '19092' : '9091')
const BASE_URL = `http://localhost:${WEB_PORT}`

if (REMOTE_NODE) {
  process.env['E2E_HTTP_PORT'] = HTTP_PORT
  process.env['E2E_NODE_PORT'] ??= '19502'
  process.env['E2E_WEB_PORT'] = WEB_PORT
}

export default defineConfig({
  testDir: './e2e',
  testIgnore: REMOTE_NODE ? [] : ['**/remote-node.spec.ts'],
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
    command: `bun run dev -- -p ${WEB_PORT}`,
    url: BASE_URL,
    reuseExistingServer: false,
    ignoreHTTPSErrors: true,
    timeout: 120_000,
    env: {
      BACKEND_URL: `http://localhost:${HTTP_PORT}`,
    },
  },
})
