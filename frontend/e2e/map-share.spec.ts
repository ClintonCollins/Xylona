import { expect, test } from '@playwright/test'
import * as path from 'path'

import { apiCreateGameServer, apiGetGame, apiLogin, apiRemoveGameServer } from './api'
import { loadTestUsers, requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

test('owner can publish, rename, reuse, and disable one public game map link', async ({
  baseURL,
  browser,
  page,
}) => {
  test.setTimeout(90_000)
  const state = requireTestState()
  const cookies = await apiLogin('e2e-superuser', 'TestPassword123!')
  const mapGame = await apiGetGame(cookies, '7_days_to_die')
  const owner = loadTestUsers().find((user) => user.username === 'e2e-superuser')
  if (!owner) throw new Error('E2E superuser fixture is unavailable')
  let gameServerId = ''
  let reuseServerId = ''
  let publicContext: Awaited<ReturnType<typeof browser.newContext>> | undefined

  try {
    gameServerId = await apiCreateGameServer(cookies, {
      name: '7 Days Map E2E',
      gameId: mapGame.id,
      userId: owner.id,
      directory: path.join(path.dirname(state.gameServerDir), 'seven-days-map-share'),
      nodeId: state.targetNodeId,
      port: 25799,
      queryPort: 25800,
    })

    await gotoAppPage(page, `/game-servers/${gameServerId}/map`)
    await page.getByRole('button', { name: 'Public link' }).click()
    const settings = page.locator('.map-share-settings')
    await expect(settings.getByRole('heading', { name: 'Public live map' })).toBeVisible()
    await settings.getByLabel('Public identifier').fill('E2E_7DTD_Map')
    const enabled = settings.getByLabel('Enable public live map')
    if (!(await enabled.isChecked())) await enabled.click()
    await settings.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Public map link settings saved.')).toBeVisible()

    publicContext = await browser.newContext({ baseURL })
    const publicPage = await publicContext.newPage()
    await publicPage.goto('/maps/E2E_7DTD_Map')
    await expect(publicPage.getByText('7 Days to Die live map')).toBeVisible()

    await settings.getByLabel('Public identifier').fill('E2E_7DTD_Renamed')
    await settings.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Public map link settings saved.')).toBeVisible()

    await publicPage.goto('/maps/E2E_7DTD_Map')
    await expect(
      publicPage.getByRole('heading', { name: 'This map link is not available' }),
    ).toBeVisible()
    await publicPage.goto('/maps/E2E_7DTD_Renamed')
    await expect(publicPage.getByText('7 Days to Die live map')).toBeVisible()

    reuseServerId = await apiCreateGameServer(cookies, {
      name: '7 Days Map Reuse E2E',
      gameId: mapGame.id,
      userId: owner.id,
      directory: path.join(path.dirname(state.gameServerDir), 'seven-days-map-share-reuse'),
      nodeId: state.targetNodeId,
      port: 25801,
      queryPort: 25802,
    })
    await gotoAppPage(page, `/game-servers/${reuseServerId}/map`)
    await page.getByRole('button', { name: 'Public link' }).click()
    const reuseSettings = page.locator('.map-share-settings')
    await reuseSettings.getByLabel('Public identifier').fill('E2E_7DTD_Map')
    const reuseEnabled = reuseSettings.getByLabel('Enable public live map')
    if (!(await reuseEnabled.isChecked())) await reuseEnabled.click()
    await reuseSettings.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Public map link settings saved.')).toBeVisible()

    await publicPage.goto('/maps/E2E_7DTD_Map')
    await expect(publicPage.getByText('7 Days to Die live map')).toBeVisible()

    await gotoAppPage(page, `/game-servers/${gameServerId}/map`)
    await page.getByRole('button', { name: 'Public link' }).click()
    const renamedSettings = page.locator('.map-share-settings')
    const renamedEnabled = renamedSettings.getByLabel('Enable public live map')
    await expect(renamedEnabled).toBeChecked()
    await renamedEnabled.click()
    await renamedSettings.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Public map link settings saved.')).toBeVisible()

    await publicPage.goto('/maps/E2E_7DTD_Renamed')
    await expect(
      publicPage.getByRole('heading', { name: 'This map link is not available' }),
    ).toBeVisible()
  } finally {
    await publicContext?.close()
    if (reuseServerId !== '') await apiRemoveGameServer(cookies, reuseServerId)
    if (gameServerId !== '') await apiRemoveGameServer(cookies, gameServerId)
  }
})
