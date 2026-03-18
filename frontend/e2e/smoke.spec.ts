import { test, expect } from '@playwright/test'

// Uses the default auth state (e2e-superuser saved by auth.setup.ts)
test.describe('Smoke tests — authenticated navigation', () => {
  test('dashboard loads at /', async ({ page }) => {
    await page.goto('/')
    await expect(page).not.toHaveURL(/\/login/)
    // Main layout renders the sidebar/nav — q-drawer or q-layout is a reliable signal
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  })

  test('game servers page loads at /game-servers', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    // The page should not show an unhandled error state
    await expect(page.locator('body')).not.toContainText('404')
  })

  test('games page loads at /games', async ({ page }) => {
    await page.goto('/games')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('body')).not.toContainText('404')
  })

  test('nodes page loads at /nodes', async ({ page }) => {
    await page.goto('/nodes')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('body')).not.toContainText('404')
  })

  test('admin users page loads at /admin/users', async ({ page }) => {
    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('body')).not.toContainText('404')
  })
})
