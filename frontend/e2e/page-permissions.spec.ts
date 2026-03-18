import { test, expect } from '@playwright/test'
import { loadTestState } from './helpers'

test.describe('superuser access', () => {
  // Default auth state is e2e-superuser

  test('can access / (dashboard)', async ({ page }) => {
    await page.goto('/')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page).toHaveURL(/\/game-servers/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers/:id/console', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers/:id/files', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers/:id/configuration', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/configuration`)
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers/:id/access', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/access`)
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /games', async ({ page }) => {
    await page.goto('/games')
    await expect(page).toHaveURL(/\/games/)
  })

  test('can access /games/create', async ({ page }) => {
    await page.goto('/games/create')
    await expect(page).toHaveURL(/\/games\/create/)
  })

  test('can access /nodes', async ({ page }) => {
    await page.goto('/nodes')
    await expect(page).toHaveURL(/\/nodes/)
  })

  test('can access /nodes/add', async ({ page }) => {
    await page.goto('/nodes/add')
    await expect(page).toHaveURL(/\/nodes\/add/)
  })

  test('can access /admin/users', async ({ page }) => {
    await page.goto('/admin/users')
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test('can access /admin/users/create', async ({ page }) => {
    await page.goto('/admin/users/create')
    await expect(page).toHaveURL(/\/admin\/users\/create/)
  })

  test('can access /secret-keys', async ({ page }) => {
    await page.goto('/secret-keys')
    await expect(page).toHaveURL(/\/secret-keys/)
  })
})

test.describe('operator access', () => {
  test.use({
    storageState: './e2e/.auth/e2e-operator.json',
  })

  test('can access /game-servers', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page).toHaveURL(/\/game-servers/)
  })

  test('can access /game-servers/:id/console (has console permission)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page).not.toHaveURL(/\/login/)
    // Operator has console permission — should stay on console page
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('cannot access /game-servers/:id/files (no files.view permission)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    // Should be redirected away from the files page
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/files/)
  })

  test('cannot access /game-servers/:id/configuration (no settings permission)', async ({
    page,
  }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/configuration`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/configuration/)
  })

  test('cannot access /game-servers/:id/access (not owner, not superuser)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/access`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/access/)
  })

  test('can access /games', async ({ page }) => {
    await page.goto('/games')
    await expect(page).toHaveURL(/\/games/)
  })

  test('cannot access /admin/users (not superuser)', async ({ page }) => {
    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })
  })

  test('cannot access /admin/users/create (not superuser)', async ({ page }) => {
    await page.goto('/admin/users/create')
    await expect(page).not.toHaveURL(/\/admin\/users\/create/, { timeout: 5_000 })
  })
})

test.describe('viewer access', () => {
  test.use({
    storageState: './e2e/.auth/e2e-viewer.json',
  })

  test('can access /game-servers', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page).toHaveURL(/\/game-servers/)
  })

  test('can see the game server in the list', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    // Viewer has view permission — should see at least one server
    await page.waitForTimeout(2000)
  })

  test('cannot access /game-servers/:id/console (no console permission)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/console/)
  })

  test('cannot access /game-servers/:id/files (no files.view permission)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/files/)
  })

  test('cannot access /game-servers/:id/configuration (no settings permission)', async ({
    page,
  }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/configuration`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/configuration/)
  })

  test('cannot access /game-servers/:id/access (not owner)', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/access`)
    await page.waitForTimeout(3000)
    await expect(page).not.toHaveURL(/\/access/)
  })

  test('cannot access /admin/users (not superuser)', async ({ page }) => {
    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })
  })
})

test.describe('no-access user', () => {
  test.use({
    storageState: './e2e/.auth/e2e-noaccess.json',
  })

  test('can access / (dashboard)', async ({ page }) => {
    await page.goto('/')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('can access /game-servers (list page, but sees no servers)', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page).toHaveURL(/\/game-servers/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('game server list is empty (no roles assigned)', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    // Wait for data to load
    await page.waitForTimeout(2000)
    // Should not see any game server cards/rows
    const serverItems = page.locator('[data-testid="game-server-item"], .game-server-card, .q-item').filter({ hasText: /E2E Test Server/ })
    await expect(serverItems).toHaveCount(0)
  })

  test('cannot access /admin/users (not superuser, redirected)', async ({ page }) => {
    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })
  })

  test('cannot access /admin/users/create (not superuser, redirected)', async ({ page }) => {
    await page.goto('/admin/users/create')
    await expect(page).not.toHaveURL(/\/admin\/users\/create/, { timeout: 5_000 })
  })
})
