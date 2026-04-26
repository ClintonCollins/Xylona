import { expect, test } from '@playwright/test'

import { apiLogin, apiUpdateGameStartArgBlocklist, apiUpdateGameStartArgsTemplate } from './api'
import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

const currentPlatform = process.platform === 'win32' ? 'windows' : 'linux'

function buildTemplateJson(heartbeatInterval: string) {
  return JSON.stringify([
    {
      id: 'editable-heartbeat',
      order: 0,
      ownership: 'editable',
      tokens: ['-heartbeat', heartbeatInterval],
      label: 'Heartbeat interval',
    },
    {
      id: 'locked-startup-delay',
      order: 1,
      ownership: 'locked',
      tokens: ['-startup-delay', '0s'],
      label: 'Startup delay',
    },
  ])
}

test.describe('Game definitions and start args', () => {
  test('seeded game definition opens in the editor', async ({ page }) => {
    const state = requireTestState()
    await gotoAppPage(page, `/games/${state.gameId}/edit`)
    await expect(page.locator('body')).toContainText('E2E Test Game')
  })

  test('template updates flow through to the server Start Command page', async ({ page }) => {
    const state = requireTestState()
    const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')

    await apiUpdateGameStartArgsTemplate(cookies, {
      gameId: state.gameId,
      platform: currentPlatform,
      startArgsTemplate: buildTemplateJson('2s'),
      baseCommand: state.dummyServerPath.replaceAll('\\', '/'),
      allowStartArgEditing: true,
    })

    await gotoAppPage(page, `/game-servers/${state.gameServerId}/start-command`)
    await expect(page.locator('[data-testid="arg-row-editable-heartbeat"]')).toContainText(
      '-heartbeat 2s',
      { timeout: 10_000 },
    )
    await expect(page.locator('[data-testid="resolved-command-preview"]')).toContainText('2s')
  })

  test('start arg blocklist violations are shown inline', async ({ page }) => {
    const state = requireTestState()
    const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')

    await apiUpdateGameStartArgBlocklist(
      cookies,
      state.gameId,
      JSON.stringify([{ pattern: '^-javaagent:', reason: 'Java agents are blocked in E2E.' }]),
    )

    await gotoAppPage(page, `/game-servers/${state.gameServerId}/start-command`)
    await page.locator('[data-testid="add-arg-button"]').click()
    await page.locator('[data-testid="tokens-input"]').fill('-javaagent:evil.jar')
    await page.locator('[data-testid="save-arg-button"]').click()

    await expect(page.getByText('Java agents are blocked in E2E.')).toBeVisible()
  })
})
