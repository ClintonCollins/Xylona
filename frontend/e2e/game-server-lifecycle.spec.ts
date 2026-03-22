import { test, expect, type Page } from '@playwright/test'
import { apiLogin, apiCreateGameServer, apiListGames, loadTestState } from './helpers'

async function gotoConsolePage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/console`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
}

async function currentControlState(page: Page) {
  const startDisabled = await page
    .getByRole('button', { name: /^Start$/i })
    .first()
    .isDisabled()
  const stopDisabled = await page
    .getByRole('button', { name: /^Stop$/i })
    .first()
    .isDisabled()

  return `${startDisabled ? 'start-disabled' : 'start-enabled'}:${stopDisabled ? 'stop-disabled' : 'stop-enabled'}`
}

async function waitForControlState(page: Page, gameServerId: string, expectedState: string) {
  await expect
    .poll(
      async () => {
        await gotoConsolePage(page, gameServerId)
        return currentControlState(page)
      },
      { timeout: 30_000 },
    )
    .toBe(expectedState)
}

test.describe('Game server lifecycle', () => {
  test('can stop a running game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await gotoConsolePage(page, state.gameServerId)

    const stopButton = page.getByRole('button', { name: /^Stop$/i }).first()
    await expect(stopButton).toBeVisible({ timeout: 10_000 })
    await expect(stopButton).toBeEnabled({ timeout: 5_000 })
    await stopButton.click()

    await waitForControlState(page, state.gameServerId, 'start-enabled:stop-disabled')
  })

  test('can start a stopped game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    await gotoConsolePage(page, state.gameServerId)

    const startButton = page.getByRole('button', { name: /^Start$/i }).first()
    await expect(startButton).toBeVisible({ timeout: 10_000 })
    await expect(startButton).toBeEnabled({ timeout: 5_000 })
    await startButton.click()

    await waitForControlState(page, state.gameServerId, 'start-disabled:stop-enabled')
  })

  test('can delete a game server via the delete dialog', async ({ page }) => {
    const adminCookies = await apiLogin(
      process.env['E2E_ADMIN_USERNAME'] ?? 'admin',
      process.env['E2E_ADMIN_PASSWORD'] ?? 'admin',
    )

    const games = await apiListGames(adminCookies)
    if (games.length === 0) {
      test.skip(true, 'No game definitions available')
      return
    }

    const tempServerName = `E2E Temp Delete Server ${Date.now()}`
    await apiCreateGameServer(adminCookies, {
      name: tempServerName,
      gameId: games[0]!.id,
      startCommand: 'echo test',
      directory: '.',
      port: 25598,
      queryPort: 25598,
    })

    // Navigate to game servers list and verify the temp server is visible.
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    const searchInput = page.getByLabel('Search game servers')
    await searchInput.fill(tempServerName)
    await expect(page.getByText(tempServerName)).toBeVisible({ timeout: 10_000 })

    // Filter down to the temp server and delete it through the list action.
    const deleteBtn = page.locator('button[aria-label="Delete game server"]').first()
    await expect(deleteBtn).toBeVisible({ timeout: 5_000 })
    await deleteBtn.click()

    // Confirm deletion in the dialog.
    const dialog = page.locator('.q-dialog').filter({ hasText: 'Delete Game Server' }).first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    const confirmDeleteBtn = dialog.getByRole('button', { name: /delete/i })
    await expect(confirmDeleteBtn).toBeVisible()
    await confirmDeleteBtn.click()

    // Wait for the dialog to close and the filtered list to clear.
    await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('No game servers')).toBeVisible({ timeout: 10_000 })
  })
})
