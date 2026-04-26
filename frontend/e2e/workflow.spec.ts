import { expect, test, type Page } from '@playwright/test'

import {
  apiCreateBackup,
  apiCreateFile,
  apiDeleteBackup,
  apiListBackups,
  apiLogin,
  apiRestoreBackup,
  apiStartGameServer,
  apiStopGameServer,
} from './api'
import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

async function waitForControlState(page: Page, expectedState: string) {
  await expect
    .poll(
      async () => {
        const startDisabled = await page
          .getByRole('button', { name: /^Start$/i })
          .first()
          .isDisabled()
        const stopDisabled = await page
          .getByRole('button', { name: /^Stop$/i })
          .first()
          .isDisabled()
        return `${startDisabled ? 'start-disabled' : 'start-enabled'}:${stopDisabled ? 'stop-disabled' : 'stop-enabled'}`
      },
      { timeout: 30_000 },
    )
    .toBe(expectedState)
}

test.describe('Critical game server workflows', () => {
  test('can stop and start the seeded dummy server', async ({ page }) => {
    const state = requireTestState()
    await gotoAppPage(page, `/game-servers/${state.gameServerId}/console`)

    const stopButton = page.getByRole('button', { name: /^Stop$/i }).first()
    await expect(stopButton).toBeEnabled({ timeout: 10_000 })
    await stopButton.click()
    await waitForControlState(page, 'start-enabled:stop-disabled')

    const startButton = page.getByRole('button', { name: /^Start$/i }).first()
    await expect(startButton).toBeEnabled({ timeout: 10_000 })
    await startButton.click()
    await waitForControlState(page, 'start-disabled:stop-enabled')

    await expect(page.getByLabel('Game server console output')).toContainText('started pid=', {
      timeout: 10_000,
    })
  })

  test('can browse files and create a file from the UI', async ({ page }) => {
    const state = requireTestState()
    await gotoAppPage(page, `/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('#file-list')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('body')).toContainText('e2e-test-config.cfg')

    const fileName = `e2e-ui-created-${Date.now()}.txt`
    await page.getByRole('button', { name: 'Create' }).first().click()
    const dialog = page
      .locator('.q-dialog')
      .filter({ hasText: 'Create new file or directory' })
      .first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    await dialog.getByLabel('Name').fill(fileName)
    await dialog.getByRole('button', { name: 'Submit' }).click()

    await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    await expect(page.locator(`[data-file-name="${fileName}"]`)).toBeVisible({ timeout: 10_000 })
  })

  test('can create, restore, and delete a backup through the running API', async ({ page }) => {
    const state = requireTestState()
    const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')

    await apiStopGameServer(cookies, state.gameServerId)
    await apiCreateFile(
      cookies,
      state.gameServerId,
      `backup-proof-${Date.now()}.txt`,
      'backup proof\n',
    )

    const backup = await apiCreateBackup(
      cookies,
      state.gameServerId,
      `e2e-backup-${Date.now()}.zip`,
    )
    await gotoAppPage(page, `/game-servers/${state.gameServerId}/backups`)
    await expect(page.locator('[data-testid="backup-history-summary"]')).toBeVisible()
    await expect(page.locator('body')).toContainText('.zip', { timeout: 10_000 })

    await apiRestoreBackup(cookies, state.gameServerId, backup.id)
    await apiDeleteBackup(cookies, state.gameServerId, backup.id)
    const backups = await apiListBackups(cookies, state.gameServerId)
    expect(backups.some((current) => current.id === backup.id)).toBe(false)

    await apiStartGameServer(cookies, state.gameServerId)
  })
})
