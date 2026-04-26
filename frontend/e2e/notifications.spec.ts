import { expect, test } from '@playwright/test'

import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

test.describe('Notifications and alerts smoke', () => {
  test('notifications page can create a webhook channel', async ({ page }) => {
    await gotoAppPage(page, '/notifications')

    await expect(page.getByRole('tab', { name: /Channels/i })).toBeVisible()
    await page.getByRole('button', { name: /Add Channel/i }).click()

    const dialog = page.locator('.q-dialog').first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    await dialog.getByLabel('Channel name').fill('E2E Discord Channel')
    await dialog.getByLabel('Webhook URL').fill('https://discord.com/api/webhooks/test/e2e')
    await dialog.getByRole('button', { name: /Create/i }).click()

    await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    await expect(page.locator('body')).toContainText('E2E Discord Channel', { timeout: 10_000 })
  })

  test('server alerts tab loads for the seeded server', async ({ page }) => {
    const state = requireTestState()
    await gotoAppPage(page, `/game-servers/${state.gameServerId}/alerts`)
    await expect(page.getByRole('tab', { name: /Alert Rules/i })).toBeVisible()
    await expect(page.getByRole('tab', { name: /Alert History/i })).toBeVisible()
  })
})
