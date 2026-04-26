import { expect, test } from '@playwright/test'

import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

test.describe('Remote node E2E smoke', () => {
  test('fixture server is assigned to the paired remote node', async ({ page }) => {
    const state = requireTestState()
    expect(state.mode).toBe('remote-node')
    expect(state.remoteNodeId).toBeTruthy()
    expect(state.targetNodeId).toBe(state.remoteNodeId)
    expect(state.nodeHomeDir).toBeTruthy()
    expect(state.gameServerDir).toContain(state.nodeHomeDir ?? '')
    expect(state.remoteNodePid).toBeGreaterThan(0)

    await gotoAppPage(page, `/game-servers/${state.gameServerId}/console`)
    await expect(page.getByLabel('Game server console output')).toContainText(
      `parent-pid=${state.remoteNodePid}`,
      { timeout: 10_000 },
    )
  })
})
