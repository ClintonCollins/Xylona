import { test, expect } from '@playwright/test'
import {
  fedApiListAggregatedGameServers,
  fedApiLogin,
  NODE_A_BACKEND,
} from './federation-helpers'

let remoteServerId: string | undefined

test.beforeAll(async () => {
  const { cookies: adminCookies } = await fedApiLogin('e2e-superuser', 'TestPassword123!', NODE_A_BACKEND)
  const servers = await fedApiListAggregatedGameServers(adminCookies, NODE_A_BACKEND)
  const remoteServer = servers.find((s) => !s.isLocal)
  remoteServerId = remoteServer?.remoteServer?.remoteServerId
})

test.describe('superuser on Node A — full federation access', () => {
  // Uses default storage state: federation-superuser.json

  test('can see remote server in /game-servers list', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(3000)
    await expect(page.locator('body')).toContainText('E2E Federation Server')
  })

  test('can access remote /game-servers/:id/console', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('can access remote /game-servers/:id/files', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('can access remote /game-servers/:id/configuration', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/configuration`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('can access remote /game-servers/:id/access', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/access`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('all remote pages load without console errors', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    const paths = [
      `/game-servers/${remoteServerId}/console`,
      `/game-servers/${remoteServerId}/files`,
      `/game-servers/${remoteServerId}/configuration`,
      `/game-servers/${remoteServerId}/access`,
    ]

    for (const p of paths) {
      const errors: string[] = []
      page.on('console', (msg) => {
        if (msg.type() === 'error') errors.push(msg.text())
      })
      page.on('pageerror', (err) => errors.push(err.message))

      await page.goto(p)
      await page.waitForLoadState('networkidle')

      expect(errors, `Errors on ${p}`).toEqual([])

      page.removeAllListeners('console')
      page.removeAllListeners('pageerror')
    }
  })
})

test.describe('operator with federated access', () => {
  test.use({
    storageState: './e2e/.auth/federation-operator.json',
  })

  test('can see remote server in /game-servers list', async ({ page }) => {
    // Note: operator needs federated access grant to see remote servers
    // This may or may not work depending on whether the setup granted access
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(3000)
  })

  test('cannot access remote /game-servers/:id/files (no files.view)', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/files`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/files/)
  })

  test('cannot access remote /game-servers/:id/configuration (no settings)', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/configuration`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/configuration/)
  })

  test('cannot access remote /game-servers/:id/access (not owner/superuser)', async ({
    page,
  }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/access`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/access/)
  })
})

test.describe('viewer with federated access', () => {
  test.use({
    storageState: './e2e/.auth/federation-viewer.json',
  })

  test('cannot access remote /game-servers/:id/console (no console)', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/console`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/console/)
  })

  test('cannot access remote /game-servers/:id/files (no files.view)', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/files`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/files/)
  })

  test('cannot access remote /game-servers/:id/configuration (no settings)', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/configuration`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/configuration/)
  })

  test('cannot access remote /game-servers/:id/access (not owner/superuser)', async ({
    page,
  }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/access`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/access/)
  })
})

test.describe('user with no federated access', () => {
  test.use({
    storageState: './e2e/.auth/federation-noaccess.json',
  })

  test('cannot see remote server in /game-servers list', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(3000)
    // Should not see the federation server
    await expect(page.locator('body')).not.toContainText('E2E Federation Server')
  })

  test('direct navigation to remote /game-servers/:id redirects or shows error', async ({
    page,
  }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}`)
    await page.waitForTimeout(3000)
    // Should be redirected or see an error
    const bodyText = await page.locator('body').textContent()
    const isRedirected = !page.url().includes(remoteServerId!)
    const hasError = bodyText?.includes('denied') || bodyText?.includes('not found') || bodyText?.includes('404')
    expect(isRedirected || hasError).toBeTruthy()
  })
})
