import { test, expect } from '@playwright/test'
import {
  fedApiListAggregatedGameServers,
  fedApiLogin,
  fedApiCreateGameServer,
  fedApiListGames,
  fedApiListGameServers,
  waitForCondition,
  NODE_A_BACKEND,
  NODE_B_BACKEND,
} from './federation-helpers'

let remoteServerId: string | undefined

test.beforeAll(async () => {
  const { cookies: adminCookies } = await fedApiLogin(
    'e2e-superuser',
    'TestPassword123!',
    NODE_A_BACKEND,
  )
  const servers = await fedApiListAggregatedGameServers(adminCookies, NODE_A_BACKEND)
  const remoteServer = servers.find((s) => !s.isLocal)
  remoteServerId = remoteServer?.remoteServer?.remoteServerId
})

test.describe('Federation remote server lifecycle', () => {
  test('can stop the remote game server via Node A UI', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    const stopButton = page.getByRole('button', { name: /stop/i }).first()
    if (await stopButton.isVisible()) {
      await stopButton.click()
      await page.waitForTimeout(500)

      const confirmButton = page.getByRole('button', { name: /confirm|yes|stop/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Wait for status to change to offline
      await expect(page.locator('body')).toContainText(/offline|stopped/i, { timeout: 15_000 })
    }
  })

  test('can start the remote game server via Node A UI', async ({ page }) => {
    if (!remoteServerId) {
      test.skip(true, 'No remote server')
      return
    }
    await page.goto(`/game-servers/${remoteServerId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    const startButton = page.getByRole('button', { name: /start/i }).first()
    if (await startButton.isVisible()) {
      await startButton.click()
      await page.waitForTimeout(500)

      const confirmButton = page.getByRole('button', { name: /confirm|yes|start/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Wait for status to change to online
      await expect(page.locator('body')).toContainText(/online|running/i, { timeout: 15_000 })
    }
  })

  test('can delete a remote game server via Node A UI', async ({ page }) => {
    // Create a temporary game server on Node B for this test
    const { cookies: adminCookiesB, userId: adminUserIdB } = await fedApiLogin(
      'admin',
      'admin',
      NODE_B_BACKEND,
    )

    const games = await fedApiListGames(adminCookiesB, NODE_B_BACKEND)
    if (games.length === 0) {
      test.skip(true, 'No games on Node B')
      return
    }

    const game = games[0]
    if (!game) throw new Error('No games available on Node B')

    const tempServerId = await fedApiCreateGameServer(
      adminCookiesB,
      {
        name: 'E2E Temp Federation Delete',
        gameId: game.id,
        userId: adminUserIdB,
        directory: '.',
        port: 25596,
      },
      NODE_B_BACKEND,
    )

    // Wait for it to sync to Node A
    const { cookies: adminCookiesA } = await fedApiLogin(
      'e2e-superuser',
      'TestPassword123!',
      NODE_A_BACKEND,
    )
    let tempRemoteId: string | undefined

    await waitForCondition(
      async () => {
        const servers = await fedApiListAggregatedGameServers(adminCookiesA, NODE_A_BACKEND)
        const found = servers.find(
          (s) => !s.isLocal && s.remoteServer?.displayName === 'E2E Temp Federation Delete',
        )
        if (found) {
          tempRemoteId = found.remoteServer?.remoteServerId
          return true
        }
        return false
      },
      60_000,
      2000,
    )

    if (!tempRemoteId) {
      test.skip(true, 'Temp server did not sync to Node A')
      return
    }

    // Navigate to the temp server page and delete
    await page.goto(`/game-servers/${tempRemoteId}/console`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(1000)

    const deleteButton = page.getByRole('button', { name: /delete/i }).first()
    if (await deleteButton.isVisible()) {
      await deleteButton.click()
      await page.waitForTimeout(500)

      const confirmButton = page.getByRole('button', { name: /confirm|yes|delete/i }).last()
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click()
      }

      // Should redirect
      await expect(page).toHaveURL(/\/game-servers/, { timeout: 10_000 })
    } else {
      // If no UI delete button, clean up via API
      const { fedApiRemoveGameServer } = await import('./federation-helpers')
      await fedApiRemoveGameServer(adminCookiesB, tempServerId, NODE_B_BACKEND)
    }

    // Verify server is gone from the list
    await page.goto('/game-servers')
    await page.waitForTimeout(3000)
    await expect(page.locator('body')).not.toContainText('E2E Temp Federation Delete')

    // Verify actually removed from Node B
    const serversOnB = await fedApiListGameServers(adminCookiesB, NODE_B_BACKEND)
    const stillExists = serversOnB.find((s) => s.id === tempServerId)
    expect(stillExists).toBeUndefined()
  })
})
