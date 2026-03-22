import { test, expect, type Page } from '@playwright/test'
import { loadTestState } from './helpers'

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
    const tempServerName = `E2E Temp Delete Server ${Date.now()}`
    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.getByRole('link', { name: 'Create Game Server' }).click()
    await expect(page.getByText('Create Game Server')).toBeVisible({ timeout: 10_000 })
    await page.getByLabel('Name').fill(tempServerName)
    await page.getByLabel('Game').click()
    const gameMenu = page.locator('.q-menu').last()
    await expect(gameMenu).toBeVisible({ timeout: 5_000 })
    await gameMenu.getByText('E2E Test Game', { exact: true }).click()
    await page.getByRole('textbox', { name: /^Port$/ }).fill('25597')
    await page.getByRole('textbox', { name: /^Query Port$/ }).fill('25598')
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page).toHaveURL(/\/game-servers\/[^/]+\/console$/, { timeout: 20_000 })

    await page.goto('/game-servers')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    const searchInput = page.getByLabel('Search game servers')
    await searchInput.fill(tempServerName)
    const serverRow = page.getByRole('row', { name: new RegExp(tempServerName) })
    await expect(serverRow).toBeVisible({ timeout: 10_000 })

    // Trigger the row delete action directly so the test depends on the UI handler, not API cleanup.
    const deleteBtn = serverRow.getByRole('button', { name: 'Delete game server' })
    await expect(deleteBtn).toBeVisible({ timeout: 5_000 })
    await deleteBtn.evaluate((button: HTMLElement) => button.click())

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
