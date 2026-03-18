import { test, expect } from '@playwright/test'
import { apiLogin, apiListGameServers } from './helpers'

test.describe('RBAC permission-based access', () => {
  test('viewer can navigate to game servers list', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-viewer.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page).toHaveURL(/\/game-servers/)

    await ctx.close()
  })

  test('operator can navigate to game servers list', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-operator.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page).toHaveURL(/\/game-servers/)

    await ctx.close()
  })

  test('no-access user is authenticated but cannot reach admin pages', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-noaccess.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)

    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })

    await ctx.close()
  })

  test('viewer cannot reach admin user management', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-viewer.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })

    await ctx.close()
  })

  test('operator cannot reach admin user management', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-operator.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })

    await ctx.close()
  })

  test('game server detail is accessible for viewer if assigned', async ({ browser }) => {
    const adminCookies = await apiLogin(
      process.env['E2E_ADMIN_USERNAME'] ?? 'admin',
      process.env['E2E_ADMIN_PASSWORD'] ?? 'admin',
    )
    const gameServers = await apiListGameServers(adminCookies)
    if (gameServers.length === 0) {
      test.skip(true, 'No game servers available — skipping game server detail access test')
      return
    }

    const firstServer = gameServers[0]!
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-viewer.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto(`/game-servers/${firstServer.id}`)
    await expect(page).not.toHaveURL(/\/login/)

    await ctx.close()
  })
})
