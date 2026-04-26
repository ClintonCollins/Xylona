import { test, expect } from '@playwright/test'
import { requireTestState } from './fixtures'
import { collectConsoleErrors, gotoAppPage } from './pages'

const smokeRoutes = ['/', '/game-servers', '/games', '/nodes', '/admin/users', '/notifications']

test.describe('Smoke navigation @smoke', () => {
  for (const route of smokeRoutes) {
    test(`${route} loads without browser errors`, async ({ page }) => {
      const consoleErrors = collectConsoleErrors(page)
      await gotoAppPage(page, route)
      expect(consoleErrors).toEqual([])
    })
  }

  test('seeded game server detail pages load', async ({ page }) => {
    const state = requireTestState()
    for (const tab of ['console', 'files', 'backups', 'alerts']) {
      await gotoAppPage(page, `/game-servers/${state.gameServerId}/${tab}`)
    }
  })
})
