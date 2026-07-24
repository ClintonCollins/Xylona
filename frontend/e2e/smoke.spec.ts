import { test, expect } from '@playwright/test'
import { requireTestState } from './fixtures'
import { collectConsoleErrors, gotoAppPage } from './pages'

const smokeRoutes = [
  '/',
  '/game-servers',
  '/games',
  '/nodes',
  '/admin/users',
  '/admin/updates',
  '/notifications',
]

test.describe('Smoke navigation @smoke', () => {
  for (const route of smokeRoutes) {
    test(`${route} loads without browser errors`, async ({ page }) => {
      const consoleErrors = collectConsoleErrors(page)
      await gotoAppPage(page, route)
      expect(consoleErrors).toEqual([])
    })
  }

  test('System Updates stays usable at a narrow viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await gotoAppPage(page, '/admin/updates')

    await expect(page.getByRole('heading', { name: 'System Updates', exact: true })).toBeVisible()
    await expect(page.getByLabel('System update connection status')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Active updates' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Available targets' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Update history' })).toBeVisible()

    const hasPageOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    )
    expect(hasPageOverflow).toBe(false)
  })

  test('seeded game server detail pages load', async ({ page }) => {
    const state = requireTestState()
    for (const tab of ['console', 'files', 'backups', 'alerts']) {
      await gotoAppPage(page, `/game-servers/${state.gameServerId}/${tab}`)
    }
  })

  test('live connection stays stale until a fresh websocket opens', async ({ page, context }) => {
    let resolveReconnectGate: () => void = () => undefined
    const reconnectGate = new Promise<void>((resolve) => {
      resolveReconnectGate = () => resolve()
    })
    let reconnectGateReleased = false
    const releaseReconnectGate = () => {
      if (reconnectGateReleased) return
      reconnectGateReleased = true
      resolveReconnectGate()
    }
    let routedReconnects = 0
    let gateReconnects = false

    await page.routeWebSocket('**/api/websocket', async (websocketRoute) => {
      if (!gateReconnects) {
        websocketRoute.connectToServer()
        return
      }

      routedReconnects++
      await reconnectGate
      websocketRoute.connectToServer()
    })

    await gotoAppPage(page, '/game-servers')

    const banner = page.locator('.live-connection-banner')
    await expect(banner).toHaveCount(0, { timeout: 15_000 })
    gateReconnects = true

    try {
      await context.setOffline(true)

      await expect(banner).toContainText("You're offline")
      await expect(banner).toContainText('Live server state is paused and may be stale')
      await expect(banner.getByRole('button', { name: 'Retry now' })).toHaveCount(0)
      await expect(banner.locator('[aria-label="Reconnecting"]')).toHaveCount(0)

      await context.setOffline(false)

      await expect.poll(() => routedReconnects, { timeout: 5_000 }).toBeGreaterThan(0)
      await expect(banner).toContainText('Live updates interrupted')
      await expect(banner).toContainText('Displayed server state may be stale')
      await expect(banner.getByRole('button', { name: 'Retry now' })).toBeVisible()

      releaseReconnectGate()
      await expect(banner).toHaveCount(0, { timeout: 15_000 })
    } finally {
      releaseReconnectGate()
      await context.setOffline(false)
    }
  })
})
