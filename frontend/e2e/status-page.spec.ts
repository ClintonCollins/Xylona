import { expect, test } from '@playwright/test'

import { gotoAppPage } from './pages'

test('owner can publish, rename, and disable a public status page', async ({
  baseURL,
  browser,
  page,
}) => {
  await gotoAppPage(page, '/game-servers')
  await page.getByRole('button', { name: 'Public status page', exact: true }).click()

  let panel = page.locator('.status-settings-panel')
  await expect(panel.getByRole('heading', { name: 'Public status page' })).toBeVisible()
  await panel.getByLabel('Page title').fill('E2E game servers')
  await panel.getByLabel('Public identifier').fill('E2E_Fleet')
  const enabled = panel.getByLabel('Enable public status page')
  if (!(await enabled.isChecked())) await enabled.click()
  await panel.getByRole('button', { name: 'Save settings' }).click()
  await expect(page.getByText('Status page settings saved')).toBeVisible()

  const publicContext = await browser.newContext({ baseURL })
  const publicPage = await publicContext.newPage()
  await publicPage.goto('/status/E2E_Fleet')
  await expect(publicPage.getByRole('heading', { name: 'E2E game servers' })).toBeVisible()
  await expect(publicPage.getByRole('heading', { name: 'E2E Test Server' })).toBeVisible()

  await gotoAppPage(page, '/game-servers')
  await page.getByRole('button', { name: 'Public status page', exact: true }).click()
  panel = page.locator('.status-settings-panel')
  await panel.getByLabel('Public identifier').fill('E2E_Fleet_Renamed')
  await panel.getByRole('button', { name: 'Save settings' }).click()
  await expect(page.getByText('Status page settings saved')).toBeVisible()

  await publicPage.goto('/status/E2E_Fleet')
  await expect(
    publicPage.getByRole('heading', { name: 'This status page is not available' }),
  ).toBeVisible()
  await publicPage.goto('/status/E2E_Fleet_Renamed')
  await expect(publicPage.getByRole('heading', { name: 'E2E game servers' })).toBeVisible()

  await gotoAppPage(page, '/game-servers')
  await page.getByRole('button', { name: 'Public status page', exact: true }).click()
  panel = page.locator('.status-settings-panel')
  await panel.getByLabel('Enable public status page').click()
  await panel.getByRole('button', { name: 'Save settings' }).click()
  await expect(page.getByText('Status page settings saved')).toBeVisible()

  await publicPage.goto('/status/E2E_Fleet_Renamed')
  await expect(
    publicPage.getByRole('heading', { name: 'This status page is not available' }),
  ).toBeVisible()
  await publicContext.close()
})
