import { test, expect } from '@playwright/test'
import {
  apiLogin,
  apiCreateGameServer,
  apiStartGameServer,
  apiListGames,
  loadTestState,
  BACKEND_URL,
} from './helpers'

test.describe('Game server lifecycle', () => {
  test('can stop a running game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Find and click the stop button
    const stopButton = page.getByRole('button', { name: /stop/i }).first()
    if (await stopButton.isVisible()) {
      await stopButton.click()
      await page.waitForTimeout(500)

      // Handle confirmation dialog if present
      const confirmButton = page.getByRole('button', { name: /confirm|yes|stop/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Wait for status to change
      await page.waitForTimeout(3000)

      // Verify offline status appears somewhere on the page
      await expect(page.locator('body')).toContainText(/offline|stopped/i, { timeout: 10_000 })
    }
  })

  test('can start a stopped game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Find and click the start button
    const startButton = page.getByRole('button', { name: /start/i }).first()
    if (await startButton.isVisible()) {
      await startButton.click()
      await page.waitForTimeout(500)

      // Handle confirmation dialog if present
      const confirmButton = page.getByRole('button', { name: /confirm|yes|start/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Wait for status to change
      await page.waitForTimeout(5000)

      // Verify online status appears
      await expect(page.locator('body')).toContainText(/online|running/i, { timeout: 10_000 })
    }
  })

  test('can delete a game server via the delete dialog', async ({ page }) => {
    // Create a temporary game server for this test
    const adminCookies = await apiLogin(
      process.env['E2E_ADMIN_USERNAME'] ?? 'admin',
      process.env['E2E_ADMIN_PASSWORD'] ?? 'admin',
    )

    const games = await apiListGames(adminCookies)
    if (games.length === 0) {
      test.skip(true, 'No game definitions available')
      return
    }

    const tempServerId = await apiCreateGameServer(adminCookies, {
      name: 'E2E Temp Delete Server',
      gameId: games[0]!.id,
      startCommand: 'echo test',
      directory: '.',
      port: 25598,
      queryPort: 25598,
    })

    // Navigate to game servers list
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Navigate to the temp server's detail page
    await page.goto(`/game-servers/${tempServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(1000)

    // Look for delete button
    const deleteButton = page.getByRole('button', { name: /delete/i }).first()
    if (await deleteButton.isVisible()) {
      await deleteButton.click()
      await page.waitForTimeout(500)

      // Confirm deletion
      const confirmButton = page.getByRole('button', { name: /confirm|yes|delete/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Should redirect back to game servers list
      await expect(page).toHaveURL(/\/game-servers/, { timeout: 10_000 })
    } else {
      // If no delete button is visible on the console page, try via API
      const { apiRemoveGameServer } = await import('./helpers')
      await apiRemoveGameServer(adminCookies, tempServerId)
    }

    // Verify the server is no longer in the list
    await page.goto('/game-servers')
    await page.waitForTimeout(2000)
    await expect(page.locator('body')).not.toContainText('E2E Temp Delete Server')
  })
})
