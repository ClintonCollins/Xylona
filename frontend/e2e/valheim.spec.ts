import { create, fromJson, toJson } from '@bufbuild/protobuf'
import { expect, test, type Page } from '@playwright/test'
import { ModProfileSchema, Status } from '@/proto/shared_pb'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'
import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

async function valheimServer(page: Page) {
  await page.route('**/xylona.Xylona/GetGameServer', async (route) => {
    const original = await route.fetch()
    const response = fromJson(GetGameServerResponseSchema, await original.json())
    if (!response.gameServer) throw new Error('Expected seeded game server')
    response.gameServer.gameId = 'valheim'
    response.gameServer.resolvedHasModSupport = true
    response.gameServer.resolvedModProfile = create(ModProfileSchema, {
      installPath: 'BepInEx/plugins',
      sources: [{ id: 'thunderstore', searchParamsJson: '{"community":"valheim"}' }],
    })
    response.gameServer.status = Status.UNKNOWN
    response.gameServer.effectivePermissions = [
      'game_server.view',
      'game_server.settings',
      'game_server.mods',
      'game_server.backup',
      'game_server.console',
      'game_server.players.manage',
    ]
    await route.fulfill({ response: original, json: toJson(GetGameServerResponseSchema, response) })
  })
}

test('mobile stored access requires exact identity and review; join password stays write-only', async ({
  page,
}, testInfo) => {
  await valheimServer(page)
  await page.setViewportSize({ width: 390, height: 844 })
  const serverId = requireTestState().gameServerId
  const changes: Record<string, unknown>[] = []
  const revision = 'a'.repeat(64)
  await page.route('**/xylona.Xylona/ListGameServerOperations', (route) =>
    route.fulfill({
      json: {
        operations: ['list', 'add'].map((action) => ({
          id: `valheim.access.administrators.${action}`,
          name: action === 'add' ? 'Add administrator' : 'List administrators',
          available: true,
          fields: action === 'add' ? [{ id: 'player' }, { id: 'expected_revision' }] : [],
        })),
      },
    }),
  )
  await page.route('**/xylona.Xylona/ExecuteGameServerOperation', (route) => {
    const request = route.request().postDataJSON() as Record<string, unknown>
    changes.push(request)
    return route.fulfill({
      json: {
        result: {
          classification: 'GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED',
          message: 'Stored file read back. Applies on next start.',
          valheimAccessList: { revision, identities: ['Steam_76561198000000001'] },
        },
      },
    })
  })
  await gotoAppPage(page, `/game-servers/${serverId}/operations`)
  await expect(page.getByRole('tab', { name: 'Operations', exact: true })).toBeVisible()
  const access = page.getByRole('region', { name: 'Stored Valheim access' })
  await expect(access).toContainText('In-game enforcement is not verified')
  await access.getByLabel('Exact platform identity').fill('Display name')
  await expect(
    access.getByRole('button', { name: 'Add administrator', exact: true }),
  ).toBeDisabled()
  await access.getByLabel('Exact platform identity').fill('Steam_76561198000000002')
  await access.getByRole('button', { name: 'Add administrator', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('Steam_76561198000000002')
  await expect(page.getByRole('dialog')).toContainText('Applies on next start')
  await page.getByRole('button', { name: 'Confirm stored change' }).click()
  await expect(access).toContainText('Confirmed: stored file read back')
  await expect.poll(() => changes.at(-1)?.operationId).toBe('valheim.access.administrators.add')
  expect(changes.at(-1)).toMatchObject({
    operationId: 'valheim.access.administrators.add',
    values: [
      { fieldId: 'player', stringValue: 'Steam_76561198000000002' },
      { fieldId: 'expected_revision', stringValue: revision },
    ],
  })
  await page.screenshot({ path: testInfo.outputPath('valheim-access-mobile.png'), fullPage: true })
  await page.route('**/xylona.Xylona/GetJoinPasswordState', (route) =>
    route.fulfill({ json: { state: { supported: true, configured: true } } }),
  )
  let saved = false
  await page.route('**/xylona.Xylona/SetJoinPassword', (route) => {
    saved = route.request().postDataJSON().password === 'New-test-password'
    return route.fulfill({ json: { state: { supported: true, configured: true } } })
  })
  await gotoAppPage(page, `/game-servers/${serverId}/settings`)
  const password = page.getByLabel('New join password')
  await expect(password).toHaveAttribute('type', 'password')
  await expect(password).toHaveValue('')
  await password.fill('New-test-password')
  await page.getByRole('button', { name: 'Replace password' }).click()
  const settings = page.getByRole('region', { name: 'Join password', exact: true })
  await expect(settings).toContainText('Join password configured. Applies on next start.')
  await expect(settings).not.toContainText('New-test-password')
  await expect(password).toHaveValue('')
  expect(saved).toBe(true)
  await page.route('**/xylona.Xylona/ClearJoinPassword', (route) =>
    route.fulfill({ json: { state: { supported: true, configured: false } } }),
  )
  const cleared = page.waitForRequest('**/xylona.Xylona/ClearJoinPassword')
  await settings.getByRole('button', { name: 'Disable password protection' }).click()
  expect((await cleared).postDataJSON()).toEqual({ serverId })
  await expect(settings).toContainText('Password protection disabled. Applies on next start.')
  await expect(settings.getByRole('button', { name: 'Set password', exact: true })).toBeVisible()
})

test('Valheim retains the installed inventory and Thunderstore browser', async ({
  page,
}, testInfo) => {
  await valheimServer(page)
  await page.route('**/xylona.Xylona/ListInstalledMods', (route) =>
    route.fulfill({ json: { installedMods: [] } }),
  )
  await page.route('**/xylona.Xylona/GetModCategories', (route) =>
    route.fulfill({ json: { categories: [] } }),
  )
  await page.route('**/xylona.Xylona/GetUpdateTargets', (route) =>
    route.fulfill({ json: { targets: [] } }),
  )
  await page.route('**/xylona.Xylona/SearchMods', (route) =>
    route.fulfill({
      json: {
        results: [
          {
            source: 'thunderstore',
            sourceId: 'Example-Mod',
            name: 'Example Mod',
            author: 'Example',
            description: 'A Valheim mod',
            latestVersion: '1.2.3',
          },
        ],
        totalCount: 1,
      },
    }),
  )
  await gotoAppPage(page, `/game-servers/${requireTestState().gameServerId}/mods`)
  await expect(page.getByRole('tab', { name: 'Browse', exact: true })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Search mods' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'View details for Example Mod' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Install', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Browse', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(page.getByRole('button', { name: 'Thunderstore', exact: false })).toBeInViewport({
    ratio: 1,
  })
  await page.screenshot({ path: testInfo.outputPath('valheim-mods-desktop.png'), fullPage: true })
  await page.getByRole('tab', { name: 'Installed', exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Search mods' })).toHaveCount(0)
})
