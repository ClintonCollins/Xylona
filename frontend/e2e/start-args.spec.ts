import { create } from '@bufbuild/protobuf'
import { expect, test, type Browser } from '@playwright/test'
import * as fs from 'node:fs/promises'
import * as path from 'node:path'

import {
  CommandProcessor,
  GameSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
  type Game,
} from '@/proto/shared_pb'
import {
  apiAddGame,
  apiCreateGameServer,
  apiCreateUser,
  apiLogin,
  apiRemoveGame,
  apiRemoveGameServer,
  apiStopGameServer,
  apiUpdateGameStartArgBlocklist,
  apiUpdateGameStartArgsTemplate,
  loginAsUser,
  type ApiCookies,
  type TestUser,
} from './helpers'

const isWindows = process.platform === 'win32'
const currentPlatform = isWindows ? 'windows' : 'linux'
const dummyBinaryName = isWindows ? 'dummy-game-server.exe' : 'dummy-game-server'
const dummyBinaryPath = path
  .join(import.meta.dirname, '.e2e-data', 'bin', dummyBinaryName)
  .replaceAll('\\', '/')

let nextPort = 26700

type StartArgsFixture = {
  adminCookies: ApiCookies
  game: Game
  owner: TestUser
  serverDir: string
  serverId: string
}

function takePort() {
  const port = nextPort
  nextPort += 2
  return port
}

function uniqueName(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

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

function buildTempGame(templateJson: string) {
  return create(GameSchema, {
    name: uniqueName('E2E Start Args Game'),
    linuxSupport: true,
    windowsSupport: true,
    linuxStartArgsTemplate: templateJson,
    windowsStartArgsTemplate: templateJson,
    linuxBaseCommand: dummyBinaryPath,
    windowsBaseCommand: dummyBinaryPath,
    linuxStopCommand: 'stop',
    windowsStopCommand: 'stop',
    linuxInstallCommand: 'echo installed',
    windowsInstallCommand: `cmd /c "echo installed"`,
    linuxInstallCommandProcessor: CommandProcessor.BASH,
    windowsInstallCommandProcessor: CommandProcessor.CMD,
    linuxUpdateCommand: 'echo updated',
    windowsUpdateCommand: `cmd /c "echo updated"`,
    linuxUpdateCommandProcessor: CommandProcessor.BASH,
    windowsUpdateCommandProcessor: CommandProcessor.CMD,
    defaultPort: BigInt(25565),
    defaultQueryPort: BigInt(25566),
    allowStartArgEditing: true,
    startArgBlocklist: '[]',
    updateProvider: create(UpdateProviderConfigSchema, {
      kind: UpdateProviderKind.COMMAND,
    }),
  })
}

async function createFixture(): Promise<StartArgsFixture> {
  const adminCookies = await apiLogin('e2e-superuser', 'TestPassword123!')
  const owner = await apiCreateUser(adminCookies, {
    username: uniqueName('startargs-owner'),
    email: `${uniqueName('startargs-owner')}@test.local`,
    password: 'TestPassword123!',
    firstName: 'Start',
    lastName: 'Owner',
  })

  const game = await apiAddGame(adminCookies, buildTempGame(buildTemplateJson('5s')))
  const serverDir = path.join(
    import.meta.dirname,
    '.e2e-data',
    'bin',
    uniqueName('startargs-server'),
  )
  await fs.mkdir(serverDir, { recursive: true })

  const port = takePort()
  const serverId = await apiCreateGameServer(adminCookies, {
    name: uniqueName('E2E Start Args Server'),
    gameId: game.id,
    userId: owner.id,
    directory: serverDir.replaceAll('\\', '/'),
    port,
    queryPort: port + 1,
  })

  return {
    adminCookies,
    game,
    owner,
    serverDir,
    serverId,
  }
}

async function cleanupFixture(fixture: StartArgsFixture | undefined) {
  if (!fixture) {
    return
  }

  try {
    await apiStopGameServer(fixture.adminCookies, fixture.serverId)
  } catch (errStop) {
    void errStop
  }

  try {
    await apiRemoveGameServer(fixture.adminCookies, fixture.serverId)
  } catch (errRemoveServer) {
    void errRemoveServer
  }

  try {
    await apiRemoveGame(fixture.adminCookies, fixture.game.id)
  } catch (errRemoveGame) {
    void errRemoveGame
  }

  await fs.rm(fixture.serverDir, { recursive: true, force: true })
}

async function openOwnerSession(browser: Browser, owner: TestUser) {
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()
  await loginAsUser(page, owner.username, owner.password)

  return { context, page }
}

test.describe('Structured start args', () => {
  test('server owner can edit args, save them, and launch with the resolved argv', async ({
    browser,
  }) => {
    const fixture = await createFixture()

    try {
      const { context, page } = await openOwnerSession(browser, fixture.owner)

      try {
        await page.goto(`/game-servers/${fixture.serverId}/start-command`)
        await expect(page.getByRole('heading', { name: 'Start Command' })).toBeVisible()
        await expect(page.locator('[data-testid="arg-row-editable-heartbeat"]')).toContainText(
          '-heartbeat 5s',
        )
        await expect(page.locator('[data-testid="arg-row-locked-startup-delay"]')).toContainText(
          'Startup delay',
        )

        await page.locator('[data-testid="edit-editable-heartbeat"]').click()
        await page.locator('[data-testid="tokens-input"]').fill('-heartbeat\n1s')
        await page.locator('[data-testid="save-arg-button"]').click()

        await expect(page.locator('[data-testid="arg-row-editable-heartbeat"]')).toContainText(
          '-heartbeat 1s',
        )
        await expect(page.locator('[data-testid="arg-row-editable-heartbeat"]')).toContainText(
          'was -heartbeat 5s',
        )
        await expect(page.locator('[data-testid="resolved-command-preview"]')).toContainText(
          '-heartbeat',
        )
        await expect(page.locator('[data-testid="resolved-command-preview"]')).toContainText('1s')

        await page.getByRole('button', { name: /^Save$/ }).click()
        await expect(page.getByText('Start arguments saved successfully.')).toBeVisible({
          timeout: 10_000,
        })

        await page.goto(`/game-servers/${fixture.serverId}/console`)
        const startButton = page.getByRole('button', { name: /^Start$/ }).first()
        await expect(startButton).toBeEnabled({ timeout: 10_000 })
        await startButton.click()

        const consoleOutput = page.getByLabel('Game server console output')
        await expect(consoleOutput).toContainText('started pid=', { timeout: 10_000 })
        await expect(consoleOutput).toContainText('heartbeat', { timeout: 4_000 })
      } finally {
        await context.close()
      }
    } finally {
      await cleanupFixture(fixture)
    }
  })

  test('template updates flow through to servers that have no patches', async ({ browser }) => {
    const fixture = await createFixture()

    try {
      await apiUpdateGameStartArgsTemplate(fixture.adminCookies, {
        gameId: fixture.game.id,
        platform: currentPlatform,
        startArgsTemplate: buildTemplateJson('2s'),
        baseCommand: dummyBinaryPath,
        allowStartArgEditing: true,
      })

      const { context, page } = await openOwnerSession(browser, fixture.owner)

      try {
        await page.goto(`/game-servers/${fixture.serverId}/start-command`)
        await expect(page.locator('[data-testid="arg-row-editable-heartbeat"]')).toContainText(
          '-heartbeat 2s',
        )
        await expect(page.locator('[data-testid="resolved-command-preview"]')).toContainText('2s')
      } finally {
        await context.close()
      }
    } finally {
      await cleanupFixture(fixture)
    }
  })

  test('blocklist violations are shown inline when a server owner adds a blocked arg', async ({
    browser,
  }) => {
    const fixture = await createFixture()

    try {
      await apiUpdateGameStartArgBlocklist(
        fixture.adminCookies,
        fixture.game.id,
        JSON.stringify([
          {
            pattern: '^-javaagent:',
            reason: 'Java agents are blocked in E2E.',
          },
        ]),
      )

      const { context, page } = await openOwnerSession(browser, fixture.owner)

      try {
        await page.goto(`/game-servers/${fixture.serverId}/start-command`)
        await page.locator('[data-testid="add-arg-button"]').click()
        await page.locator('[data-testid="tokens-input"]').fill('-javaagent:evil.jar')
        await page.locator('[data-testid="save-arg-button"]').click()

        await expect(page.getByText('Java agents are blocked in E2E.')).toBeVisible()
        await expect(page.locator('[data-testid="save-arg-button"]')).toBeVisible()
      } finally {
        await context.close()
      }
    } finally {
      await cleanupFixture(fixture)
    }
  })

  test('the Start Command tab is hidden and direct route access redirects when editing is disabled', async ({
    browser,
  }) => {
    const fixture = await createFixture()

    try {
      await apiUpdateGameStartArgsTemplate(fixture.adminCookies, {
        gameId: fixture.game.id,
        platform: currentPlatform,
        startArgsTemplate: buildTemplateJson('5s'),
        baseCommand: dummyBinaryPath,
        allowStartArgEditing: false,
      })

      const { context, page } = await openOwnerSession(browser, fixture.owner)

      try {
        await page.goto(`/game-servers/${fixture.serverId}/console`)
        await expect(page.locator('.q-tabs')).not.toContainText('Start Command')

        await page.goto(`/game-servers/${fixture.serverId}/start-command`)
        await expect(page).toHaveURL(new RegExp(`/game-servers/${fixture.serverId}/console$`), {
          timeout: 10_000,
        })
      } finally {
        await context.close()
      }
    } finally {
      await cleanupFixture(fixture)
    }
  })
})
