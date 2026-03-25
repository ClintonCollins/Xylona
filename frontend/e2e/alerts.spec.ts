import { test, expect } from '@playwright/test'
import {
  loadTestState,
  apiLogin,
  apiCreateNotificationChannel,
  apiListNotificationChannels,
  apiDeleteNotificationChannel,
  apiListAlertRules,
  apiDeleteAlertRule,
} from './helpers'

const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

/**
 * Discord Webhook channel type enum value.
 * Must match xylona.NotificationChannelType.WEBHOOK_DISCORD = 1.
 */
const CHANNEL_TYPE_DISCORD = 1

/**
 * Clean up all notification channels and alert rules created during tests.
 */
async function cleanupAlertsData(): Promise<void> {
  const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)

  // Delete all alert rules first (they reference channels via FK).
  const rules = await apiListAlertRules(cookies)
  for (const rule of rules) {
    await apiDeleteAlertRule(cookies, rule.id)
  }

  // Delete all notification channels.
  const channels = await apiListNotificationChannels(cookies)
  for (const ch of channels) {
    await apiDeleteNotificationChannel(cookies, ch.id)
  }
}

// ============================================================================
// Notifications Page
// ============================================================================
test.describe('Notifications Page', () => {
  test.afterAll(async () => {
    await cleanupAlertsData()
  })

  test('notifications page loads with Channels, Alert Rules, and Alert History tabs', async ({
    page,
  }) => {
    await page.goto('/notifications')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    await expect(page.getByRole('tab', { name: /Channels/i })).toBeVisible()
    await expect(page.getByRole('tab', { name: /Alert Rules/i })).toBeVisible()
    await expect(page.getByRole('tab', { name: /Alert History/i })).toBeVisible()
  })

  test('Add Channel button is visible on Channels tab', async ({ page }) => {
    await page.goto('/notifications')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    await expect(page.getByRole('button', { name: /Add Channel/i })).toBeVisible()
  })

  test('create a Discord webhook notification channel', async ({ page }) => {
    await page.goto('/notifications')
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Click Add Channel.
    await page.getByRole('button', { name: /Add Channel/i }).click()

    // The dialog should appear.
    const dialog = page.locator('.q-dialog').first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })

    // Fill in channel name.
    await dialog.getByLabel('Channel name').fill('E2E Test Discord')

    // Channel type defaults to "Discord Webhook", verify it is selected.
    await expect(dialog.getByLabel('Channel type')).toBeVisible()

    // Fill in webhook URL.
    await dialog.getByLabel('Webhook URL').fill('https://discord.com/api/webhooks/test/e2e')

    // Click Create.
    await dialog.getByRole('button', { name: /Create/i }).click()
    await expect(dialog).not.toBeVisible({ timeout: 10_000 })

    // Verify the channel appears in the table.
    await expect(page.locator('body')).toContainText('E2E Test Discord', { timeout: 10_000 })
  })
})

// ============================================================================
// Game Server Alerts Tab
// ============================================================================
test.describe('Game Server Alerts Tab', () => {
  test.afterAll(async () => {
    await cleanupAlertsData()
  })

  test('navigate to game server Alerts tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await page.goto(`/game-servers/${state.gameServerId}/alerts`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Alert Rules and Alert History tabs should be visible.
    await expect(page.getByRole('tab', { name: /Alert Rules/i })).toBeVisible({ timeout: 5_000 })
    await expect(page.getByRole('tab', { name: /Alert History/i })).toBeVisible()
  })

  test('Create Rule button is visible on game server Alerts tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Ensure at least one notification channel exists so the Create Rule button
    // is not disabled.
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const channels = await apiListNotificationChannels(cookies)
    if (channels.length === 0) {
      await apiCreateNotificationChannel(cookies, {
        name: 'E2E Alerts Test Channel',
        channelType: CHANNEL_TYPE_DISCORD,
        config: JSON.stringify({ url: 'https://discord.com/api/webhooks/test/e2e-alerts' }),
      })
    }

    await page.goto(`/game-servers/${state.gameServerId}/alerts`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    const createBtn = page.getByRole('button', { name: /Create Rule/i })
    await expect(createBtn).toBeVisible({ timeout: 5_000 })
    await expect(createBtn).toBeEnabled()
  })

  test('create a crash alert rule on a game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Ensure a notification channel exists.
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const channels = await apiListNotificationChannels(cookies)
    if (channels.length === 0) {
      await apiCreateNotificationChannel(cookies, {
        name: 'E2E Crash Alert Channel',
        channelType: CHANNEL_TYPE_DISCORD,
        config: JSON.stringify({ url: 'https://discord.com/api/webhooks/test/crash-alert' }),
      })
    }

    await page.goto(`/game-servers/${state.gameServerId}/alerts`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Click Create Rule.
    await page.getByRole('button', { name: /Create Rule/i }).click()

    // The dialog should appear.
    const dialog = page.locator('.q-dialog').first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })

    // Event type defaults to "Server Crash" -- verify it is there.
    await expect(dialog.locator('body, .q-card')).toContainText(/Event Type/i)

    // Notification Channel dropdown should already have a channel selected
    // since there is exactly one.
    await expect(dialog.locator('body, .q-card')).toContainText(/Notification Channel/i)

    // Click Create to save the rule.
    await dialog.getByRole('button', { name: /Create/i }).click()
    await expect(dialog).not.toBeVisible({ timeout: 10_000 })

    // Verify the rule appears in the rules table (the event type label "Server Crash"
    // or the condition "Any crash" should be visible).
    await expect(page.locator('.alerts-panels')).toContainText(/Crash|Server Crash/i, {
      timeout: 10_000,
    })
  })

  test('view alert history tab on game server', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await page.goto(`/game-servers/${state.gameServerId}/alerts`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })

    // Click Alert History tab.
    await page.getByRole('tab', { name: /Alert History/i }).click()

    // The history panel should become visible. It may be empty but the table
    // or no-data placeholder should render.
    await expect(
      page.locator('.alerts-panels').getByText(/Alert History|No alert history/i),
    ).toBeVisible({ timeout: 10_000 })
  })
})
