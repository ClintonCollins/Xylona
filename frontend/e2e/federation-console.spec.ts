import { test, expect } from '@playwright/test'
import { fedApiListAggregatedGameServers, fedApiLogin, NODE_A_BACKEND } from './federation-helpers'

test.describe('Federation remote console streaming', () => {
  let remoteServerId: string | undefined

  test.beforeAll(async () => {
    const { cookies: adminCookies } = await fedApiLogin(
      'e2e-superuser',
      'TestPassword123!',
      NODE_A_BACKEND,
    )
    const servers = await fedApiListAggregatedGameServers(adminCookies, NODE_A_BACKEND)
    const remoteServer = servers.find((s) => !s.isLocal)
    remoteServerId = remoteServer?.remoteServer?.remoteServerId
  })

  test('remote game server console page loads without errors', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server available')
      return
    }

    const errors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForLoadState('networkidle')

    expect(errors).toEqual([])
  })

  test('console shows heartbeat output from the dummy server', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server available')
      return
    }

    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Wait for heartbeat output to appear (dummy server sends heartbeats every 5s)
    await expect(page.locator('body')).toContainText('heartbeat', { timeout: 15_000 })
  })

  test('can send a console command and see the response', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server available')
      return
    }

    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Find the console input
    const consoleInput = page.locator('input[type="text"], textarea').filter({ hasText: '' }).last()
    if (await consoleInput.isVisible()) {
      await consoleInput.fill('echo hello-from-e2e')
      await consoleInput.press('Enter')

      // Wait for the echoed response
      await expect(page.locator('body')).toContainText('hello-from-e2e', { timeout: 10_000 })
    }
  })

  test('console output updates in real time', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server available')
      return
    }

    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Wait for first heartbeat
    await expect(page.locator('body')).toContainText('heartbeat', { timeout: 15_000 })

    // Count current heartbeat lines, then wait for a new one
    const initialText = await page.locator('body').textContent()
    const initialCount = (initialText?.match(/heartbeat/g) ?? []).length

    // Wait for at least one more heartbeat (5s interval)
    await page.waitForTimeout(6000)

    const updatedText = await page.locator('body').textContent()
    const updatedCount = (updatedText?.match(/heartbeat/g) ?? []).length

    expect(updatedCount).toBeGreaterThan(initialCount)
  })
})
