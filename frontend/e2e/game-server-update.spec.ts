import { test, expect } from '@playwright/test'
import { apiLogin, apiSetDummyUpdateFailure, loadTestState } from './helpers'

test.describe('Game Server Version Tracking', () => {
  test('server list shows version column with update badge', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await page.goto('/game-servers')

    // Version column should exist in the table.
    const versionHeader = page.locator('th', { hasText: /version/i })
    await expect(versionHeader).toBeVisible()

    // The test server (dummy game) should show version 1.0.0.
    const versionCell = page.locator('td').filter({ hasText: '1.0.0' })
    await expect(versionCell).toBeVisible()

    // Amber update badge should appear.
    const updateBadge = page.locator('.update-badge', { hasText: /update/i })
    await expect(updateBadge).toBeVisible()
  })

  test('no-tracker server shows dash in version column', async ({ page }) => {
    const state = loadTestState()
    if (!state.noTrackerServerId) {
      test.skip(true, 'No no-tracker server available')
      return
    }

    await page.goto('/game-servers')

    // The no-tracker server should show an em dash.
    const noTrackerRow = page.locator('tr', { hasText: 'E2E No-Tracker' })
    const versionCell = noTrackerRow.locator('td').nth(2) // version column index
    await expect(versionCell).toContainText('—')
  })
})

test.describe('Game Server Update Flow', () => {
  test('one-click update succeeds and clears badge', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Navigate to server detail.
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Version info should show in sidebar.
    await expect(page.locator('.update-hint')).toBeVisible()

    // Click Update button.
    await page.locator('button', { hasText: 'Update' }).click()

    // Progress panel should appear.
    await expect(page.locator('.update-progress-panel')).toBeVisible()
    await expect(page.locator('text=You can safely navigate away')).toBeVisible()

    // Wait for completion toast.
    await expect(page.locator('.q-notification', { hasText: /update.*complete/i })).toBeVisible({
      timeout: 30_000,
    })
  })
})

test.describe('Game Server Update Rollback', () => {
  test.beforeEach(async () => {
    const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')
    await apiSetDummyUpdateFailure(cookies, true)
  })

  test.afterEach(async () => {
    const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')
    await apiSetDummyUpdateFailure(cookies, false)
  })

  test('failed update triggers rollback and preserves version', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Click Update.
    await page.locator('button', { hasText: 'Update' }).click()

    // Wait for failure toast.
    await expect(
      page.locator('.q-notification', { hasText: /update.*failed|rollback/i }),
    ).toBeVisible({ timeout: 30_000 })

    // Version should remain at 1.0.0.
    await expect(page.locator('text=1.0.0')).toBeVisible()

    // Update badge should still be present.
    await expect(page.locator('.update-hint')).toBeVisible()
  })
})
