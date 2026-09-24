import { expect, test } from '@playwright/test'

import { loadTestState } from './fixtures'
import { gotoAppPage } from './pages'

test.describe('Node alerts', () => {
  test('node page creates a node alert that Notifications lists and edits in scope', async ({
    page,
  }) => {
    const nodeId = loadTestState().targetNodeId
    if (!nodeId) throw new Error('E2E test state has no node')
    // CI retries against the same database, so each attempt uses its own channel and
    // finds its rule by that channel instead of by event type alone.
    const channelName = `E2E Node Alerts ${Date.now()}`

    await gotoAppPage(page, '/notifications')
    await page.getByRole('button', { name: /Add Channel/i }).click()
    const channelDialog = page.locator('.q-dialog').first()
    await channelDialog.getByLabel('Channel name').fill(channelName)
    await channelDialog.getByLabel('Webhook URL').fill('https://discord.com/api/webhooks/test/node')
    await channelDialog.getByRole('button', { name: /Create/i }).click()
    await expect(channelDialog).not.toBeVisible({ timeout: 10_000 })

    await gotoAppPage(page, `/nodes/${nodeId}`)
    // Add alert only renders once the node has loaded; before that the title reads "Loading node…".
    const addAlert = page.getByRole('button', { name: 'Add alert' })
    await expect(addAlert).toBeVisible({ timeout: 10_000 })
    const nodeName = (await page.locator('h1.xy-page-title').innerText()).trim()
    await addAlert.click()
    const dialog = page.locator('.q-dialog').filter({ hasText: 'Create Alert Rule' })
    await expect(dialog).toContainText(nodeName)
    await dialog.locator('.q-select').filter({ hasText: 'Event Type' }).click()
    await expect(page.getByRole('option')).toHaveText(['Node CPU', 'Node Memory', 'Node Disk'])
    await page.getByRole('option', { name: 'Node Disk' }).click()
    await dialog.locator('.q-select').filter({ hasText: 'Notification Channel' }).click()
    await page.getByRole('option', { name: channelName, exact: true }).click()
    await dialog.getByRole('button', { name: 'Create' }).click()
    await expect(dialog).not.toBeVisible({ timeout: 10_000 })

    // The dialog's help link opens this tab through ?tab=rules.
    await gotoAppPage(page, '/notifications?tab=rules')
    await expect(page.getByRole('tab', { name: /Alert Rules/i })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    const row = page
      .getByRole('row')
      .filter({ hasText: 'Node Disk' })
      .filter({ hasText: channelName })
    await expect(row).toContainText(nodeName)
    await expect(row).toContainText('>= 80%')
    await row.getByRole('button', { name: 'Edit Node Disk alert rule' }).click()
    const editDialog = page.locator('.q-dialog').filter({ hasText: 'Edit Alert Rule' })
    await editDialog.locator('.q-select').filter({ hasText: 'Event Type' }).click()
    await expect(page.getByRole('option')).toHaveText(['Node CPU', 'Node Memory', 'Node Disk'])
  })
})
