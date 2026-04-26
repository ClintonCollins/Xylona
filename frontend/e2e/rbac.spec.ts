import { expect, test } from '@playwright/test'

import { storageStatePath } from './auth'
import { requireTestState } from './fixtures'

test.describe('RBAC access boundaries', () => {
  test('viewer can see assigned server but cannot open files', async ({ browser }) => {
    const state = requireTestState()
    const context = await browser.newContext({
      storageState: storageStatePath('e2e-viewer.json'),
      ignoreHTTPSErrors: true,
    })
    const page = await context.newPage()

    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('body')).toContainText('E2E Test Server', { timeout: 10_000 })

    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page).not.toHaveURL(/\/files/, { timeout: 10_000 })

    await context.close()
  })

  test('operator can open console but cannot open admin user management', async ({ browser }) => {
    const state = requireTestState()
    const context = await browser.newContext({
      storageState: storageStatePath('e2e-operator.json'),
      ignoreHTTPSErrors: true,
    })
    const page = await context.newPage()

    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 10_000 })

    await context.close()
  })

  test('no-access user reaches the app shell but sees no assigned server', async ({ browser }) => {
    const context = await browser.newContext({
      storageState: storageStatePath('e2e-noaccess.json'),
      ignoreHTTPSErrors: true,
    })
    const page = await context.newPage()

    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('body')).not.toContainText('E2E Test Server')

    await context.close()
  })
})
