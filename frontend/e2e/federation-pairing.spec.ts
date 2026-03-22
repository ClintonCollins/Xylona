import { test, expect } from '@playwright/test'

test.describe('Federation node pairing & discovery', () => {
  test('both nodes are paired and visible on /nodes', async ({ page }) => {
    await page.goto('/nodes')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Should see at least 2 nodes (local + paired remote)
    await expect(page.locator('body')).toContainText('Node A', { timeout: 15_000 })
    await expect(page.locator('body')).toContainText('Node B', { timeout: 15_000 })
  })

  test('Node B shows as healthy/connected on Node A', async ({ page }) => {
    await page.goto('/nodes')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Look for a health indicator showing Node B is connected
    // This could be a status badge, icon, or text
    const nodeBRow = page.locator('tr, .q-item, .q-card').filter({ hasText: 'Node B' }).first()
    await expect(nodeBRow).toBeVisible({ timeout: 15_000 })
  })

  test('remote game server from Node B appears in /game-servers on Node A', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // The remote server should appear in the list
    await expect(page.locator('body')).toContainText('E2E Federation Server', { timeout: 15_000 })
  })

  test('remote server shows correct node indicator', async ({ page }) => {
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // The remote server should show it's from Node B (not local)
    const serverEntry = page
      .locator('tr, .q-item, .q-card')
      .filter({ hasText: 'E2E Federation Server' })
      .first()
    await expect(serverEntry).toBeVisible({ timeout: 15_000 })
    // Should show the node name "Node B" somewhere in the entry
    await expect(serverEntry).toContainText('Node B')
  })
})
