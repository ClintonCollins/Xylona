import { create, fromJson, toJson } from '@bufbuild/protobuf'
import { expect, test } from '@playwright/test'

import {
  StartGameServerRequestSchema,
  StartGameServerResponseSchema,
  Status,
  type StartGameServerRequest,
} from '@/proto/shared_pb'

import {
  ExecuteGameServerOperationRequestSchema,
  ExecuteGameServerOperationResponseSchema,
  GameOperationDescriptorSchema,
  GameOperationFieldOptionSchema,
  GameOperationFieldSchema,
  GameOperationFieldType,
  GameOperationResultClassification,
  GameOperationResultSchema,
  GameOperationReviewSchema,
  GameOperationRisk,
  GameOperationTransportDetailsSchema,
  GetGameServerResponseSchema,
  ListGameServerOperationsResponseSchema,
} from '@/proto/xylona_pb'
import type { ExecuteGameServerOperationRequest } from '@/proto/xylona_pb'

import { requireTestState } from './fixtures'
import { gotoAppPage } from './pages'

test('executes direct administration controls by keyboard on mobile through visible results', async ({
  page,
}) => {
  const state = requireTestState()
  const requests: ExecuteGameServerOperationRequest[] = []
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/xylona.Xylona/GetGameServer', async (route) => {
    const original = await route.fetch()
    const response = fromJson(GetGameServerResponseSchema, await original.json())
    if (!response.gameServer) {
      throw new Error('Expected the seeded game server')
    }
    response.gameServer.gameId = '7_days_to_die'
    response.gameServer.status = Status.ONLINE
    if (!response.gameServer.effectivePermissions.includes('game_server.players.manage')) {
      response.gameServer.effectivePermissions.push('game_server.players.manage')
    }
    await route.fulfill({
      response: original,
      json: toJson(GetGameServerResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ListGameServerOperations', async (route) => {
    const response = create(ListGameServerOperationsResponseSchema, {
      gameServerName: 'Deterministic 7DTD Server',
      operations: [
        addAdministratorOperation(),
        kickOperation(),
        saveWorldOperation(),
        setGameTimeOperation(),
        setTemperatureUnitOperation(),
        giveItemOperation(),
        setWeatherOperation(),
        spawnAirdropOperation(),
        spawnWanderingHordeOperation(),
      ],
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ListGameServerOperationsResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ExecuteGameServerOperation', async (route) => {
    const request = fromJson(
      ExecuteGameServerOperationRequestSchema,
      route.request().postDataJSON(),
    )
    requests.push(request)
    const setGameTime = request.operationId === 'server_control.set_game_time'
    const confirmed =
      setGameTime ||
      (request.operationId === 'player_access.add_administrator' && requests.length === 1)
    const operationName =
      request.operationId === 'player_moderation.kick'
        ? 'Kick Player'
        : request.operationId === 'server_control.set_game_time'
          ? 'Set game time'
          : request.operationId === 'server_control.save_world'
            ? 'Save world'
            : request.operationId === 'server_control.set_temperature_unit'
              ? 'Set temperature unit'
              : 'Add administrator'
    const response = create(ExecuteGameServerOperationResponseSchema, {
      result: create(GameOperationResultSchema, {
        classification: confirmed
          ? GameOperationResultClassification.CONFIRMED
          : GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
        message: setGameTime
          ? 'The world time change was confirmed by native read-back.'
          : confirmed
            ? 'Administrator access was confirmed.'
            : `The server accepted ${operationName}, but read-back was unavailable.`,
        transportDetails: create(
          GameOperationTransportDetailsSchema,
          request.operationId === 'player_moderation.kick'
            ? {
                method: 'Typed 7 Days to Die console action',
                verification: 'Console submission only',
              }
            : {
                method: '7 Days to Die native dashboard',
                verification: setGameTime ? 'World time read-back' : 'User permission read-back',
              },
        ),
      }),
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ExecuteGameServerOperationResponseSchema, response),
    })
  })

  await gotoAppPage(page, `/game-servers/${state.gameServerId}/operations`)
  await expect(page.getByRole('heading', { name: 'Operations workbench' })).toBeVisible()
  await expect(page.locator('main, [role="main"]')).toHaveCount(1)
  expect(
    await page.locator('.game-server-content').evaluate((content) => {
      return content.scrollWidth <= content.clientWidth
    }),
  ).toBe(true)
  const operationButtonHeights = await page
    .locator('.operations-page button:visible')
    .evaluateAll((buttons) => buttons.map((button) => button.getBoundingClientRect().height))
  expect(Math.min(...operationButtonHeights)).toBeGreaterThanOrEqual(44)
  await expect(page.getByTestId('operation-picker')).toHaveCount(0)
  await expect(page.locator('.active-task')).toHaveCount(1)
  await page.getByTestId('operation-option-player-assistance').click()
  await expect(page.getByRole('heading', { name: 'Player assistance' })).toBeVisible()
  const itemPicker = page.getByRole('combobox', { name: 'Item', exact: true })
  await itemPicker.fill('wood')
  const itemOption = page.getByRole('option', { name: /Wood.*resourceWood/ })
  await expect(itemOption).toBeVisible()
  await expect(itemOption.locator('img')).toBeVisible()
  await expect(itemOption).toContainText('Resources / Basics')
  await itemOption.click()
  await expect(itemPicker).toHaveValue('Wood')
  await expect(page.getByTestId('selected-item')).toContainText('Wood')
  await expect(page.getByTestId('selected-item')).toContainText('Server catalog')
  await itemPicker.fill('moddedCustomItem')
  await page.keyboard.press('Enter')
  await expect(itemPicker).toHaveValue('moddedCustomItem')
  await page.getByTestId('operation-category-access').click()
  await page.getByTestId('operation-option-player-access').click()
  const playerPicker = page.getByTestId('player-identity')
  await playerPicker.click()
  await expect(page.getByRole('option', { name: /Player One/ })).toBeVisible()
  await playerPicker.fill('EOS_PLAYER_2')
  await expect(page.getByTestId('selected-player')).toContainText('Player Two')
  await expect(page.getByTestId('selected-player')).toContainText('Offline')
  const addAdministrator = page.getByTestId('add-administrator')
  await addAdministrator.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toContainText('Player Two (EOS_PLAYER_2)')
  await page.getByTestId('confirm-operation').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('operation-result')).toContainText('Confirmed')
  await expect(page.getByTestId('operation-result')).toContainText(
    'Administrator access was confirmed.',
  )
  await page.getByRole('button', { name: 'Dismiss operation result' }).click()

  await page.getByTestId('player-identity').fill('EOS_PLAYER_42')
  await page.getByText('Advanced custom numeric', { exact: true }).click()
  await page.getByTestId('administrator-permission-level').fill('42')
  await addAdministrator.focus()
  await page.keyboard.press('Enter')
  await page.getByTestId('confirm-operation').focus()
  await page.keyboard.press('Enter')

  await page.getByTestId('operation-category-world').click()
  await page.getByTestId('operation-option-world-controls').click()
  await expect(page.getByTestId('set-weather')).toBeVisible()
  await expect(page.getByTestId('save-world')).toBeVisible()
  await expect(page.getByTestId('set-temperature-unit')).toBeVisible()
  const setDay = page.getByRole('button', { name: 'Set day', exact: true })
  await setDay.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toContainText('Review world time change')
  await page.getByTestId('confirm-operation').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('operation-result')).toContainText('Confirmed')
  await page.getByRole('button', { name: 'Dismiss operation result' }).click()

  const saveWorld = page.getByTestId('save-world')
  await saveWorld.focus()
  await page.keyboard.press('Enter')

  await page.getByRole('combobox', { name: 'Unit' }).selectOption('C')
  await page.getByRole('button', { name: 'Apply', exact: true }).focus()
  await page.keyboard.press('Enter')

  await page.getByTestId('operation-option-spawn-events').click()
  const spawnAirdrop = page.getByTestId('spawn-airdrop')
  await expect(spawnAirdrop).toBeVisible()
  await expect(page.getByTestId('spawn-wandering-horde')).toBeVisible()
  await spawnAirdrop.hover()
  await expect(spawnAirdrop).toHaveCSS('background-color', 'rgb(251, 191, 36)')
  await expect(spawnAirdrop).toHaveCSS('color', 'rgb(13, 14, 15)')
  await spawnAirdrop.click()
  const spawnConfirmation = page.getByTestId('confirm-operation')
  await expect(spawnConfirmation).toHaveCSS('min-height', '44px')
  await expect(spawnConfirmation).toHaveClass(/action-button--warning/)
  await spawnConfirmation.hover()
  await expect(spawnConfirmation).toHaveCSS('background-color', 'rgb(251, 191, 36)')
  await expect(page.getByRole('button', { name: 'Cancel' })).toHaveCSS('min-height', '44px')
  await expect(page.locator('.confirmation-actions')).toHaveCSS('gap', '8px')
  await page.getByRole('button', { name: 'Cancel' }).click()

  await expect.poll(() => requests.length).toBe(5)
  await expect(page.getByTestId('operation-result')).toContainText(
    'Set temperature unit — Command issued',
  )
  await expect(page.getByTestId('operation-result')).toContainText(
    'The command was sent to the game server.',
  )
  await expect(page.getByTestId('operation-result')).not.toContainText('read-back')
  expect(requests.map((request) => request.operationId)).toEqual([
    'player_access.add_administrator',
    'player_access.add_administrator',
    'server_control.set_game_time',
    'server_control.save_world',
    'server_control.set_temperature_unit',
  ])

  await gotoAppPage(
    page,
    `/game-servers/${state.gameServerId}/operations?operation=player_moderation.kick&player=EOS_PLAYER_2`,
  )
  await expect(page.getByRole('dialog')).toContainText('Review Kick Player')
  await expect(page.getByRole('dialog')).toContainText('Player Two (EOS_PLAYER_2)')
  await page.getByTestId('confirm-operation').click()

  await expect.poll(() => requests.length).toBe(6)
  await expect(page.getByTestId('operation-result')).toContainText('Kick Player — Command issued')
  await expect(page.getByTestId('operation-result')).toContainText(
    'The command was sent to the game server.',
  )
  expect(requests[0]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'player_access.add_administrator',
    values: [
      { fieldId: 'player', value: { case: 'stringValue', value: 'EOS_PLAYER_2' } },
      { fieldId: 'permission_level', value: { case: 'integerValue', value: 200n } },
    ],
  })
  expect(requests[1]?.values).toMatchObject([
    { fieldId: 'player', value: { case: 'stringValue', value: 'EOS_PLAYER_42' } },
    { fieldId: 'permission_level', value: { case: 'integerValue', value: 42n } },
  ])
  expect(requests[2]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'server_control.set_game_time',
    values: [{ fieldId: 'time', value: { case: 'stringValue', value: 'day' } }],
  })
  expect(requests[3]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'server_control.save_world',
    values: [],
  })
  expect(requests[4]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'server_control.set_temperature_unit',
    values: [{ fieldId: 'unit', value: { case: 'stringValue', value: 'C' } }],
  })
  expect(requests[5]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'player_moderation.kick',
    values: [
      { fieldId: 'player', value: { case: 'stringValue', value: 'EOS_PLAYER_2' } },
      { fieldId: 'reason', value: { case: 'stringValue', value: '' } },
    ],
  })

  await page.unrouteAll({ behavior: 'wait' })
})

test('executes communication controls without exposing information commands', async ({ page }) => {
  const state = requireTestState()
  const requests: ExecuteGameServerOperationRequest[] = []

  await page.route('**/xylona.Xylona/GetGameServer', async (route) => {
    const original = await route.fetch()
    const response = fromJson(GetGameServerResponseSchema, await original.json())
    if (!response.gameServer) {
      throw new Error('Expected the seeded game server')
    }
    response.gameServer.gameId = '7_days_to_die'
    response.gameServer.status = Status.ONLINE
    await route.fulfill({
      response: original,
      json: toJson(GetGameServerResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ListGameServerOperations', async (route) => {
    const response = create(ListGameServerOperationsResponseSchema, {
      gameServerName: 'Deterministic 7DTD Server',
      operations: [broadcastOperation(), versionOperation()],
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ListGameServerOperationsResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ExecuteGameServerOperation', async (route) => {
    const request = fromJson(
      ExecuteGameServerOperationRequestSchema,
      route.request().postDataJSON(),
    )
    requests.push(request)
    const response = create(ExecuteGameServerOperationResponseSchema, {
      result: create(GameOperationResultSchema, {
        classification:
          request.operationId === 'communication.broadcast_message'
            ? GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED
            : GameOperationResultClassification.CONFIRMED,
        message:
          request.operationId === 'communication.broadcast_message'
            ? 'The announcement was accepted, but delivery could not be verified.'
            : 'Version: V2.6 b22',
      }),
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ExecuteGameServerOperationResponseSchema, response),
    })
  })

  await gotoAppPage(page, `/game-servers/${state.gameServerId}/operations`)
  await expect(page.getByRole('heading', { name: 'Messaging' })).toBeVisible()
  await expect(page.getByText('Inspect game version')).toHaveCount(0)
  await page.getByRole('textbox', { name: 'Message' }).fill('Server restart soon')
  await page.getByRole('button', { name: 'Send server announcement', exact: true }).click()

  await expect.poll(() => requests.length).toBe(1)
  await expect(page.getByTestId('operation-result')).toContainText(
    'Send server announcement — Command issued',
  )
  await expect(page.getByTestId('operation-result')).toContainText(
    'The command was sent to the game server.',
  )
  await expect(page.getByTestId('operation-result')).not.toContainText('verified')
  expect(requests[0]).toMatchObject({
    operationId: 'communication.broadcast_message',
    values: [{ fieldId: 'message', value: { case: 'stringValue', value: 'Server restart soon' } }],
  })

  await page.unrouteAll({ behavior: 'wait' })
})

test('starts an authoritatively stopped server from Operations when permitted', async ({
  page,
}) => {
  const state = requireTestState()
  let startRequest: StartGameServerRequest | undefined

  await page.route('**/xylona.Xylona/GetGameServer', async (route) => {
    const original = await route.fetch()
    const response = fromJson(GetGameServerResponseSchema, await original.json())
    if (!response.gameServer) {
      throw new Error('Expected the seeded game server')
    }
    response.gameServer.gameId = '7_days_to_die'
    response.gameServer.status = Status.OFFLINE
    if (!response.gameServer.effectivePermissions.includes('game_server.start')) {
      response.gameServer.effectivePermissions.push('game_server.start')
    }
    await route.fulfill({
      response: original,
      json: toJson(GetGameServerResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ListGameServerOperations', async (route) => {
    const response = create(ListGameServerOperationsResponseSchema, {
      gameServerName: 'Deterministic 7DTD Server',
      operations: [saveWorldOperation()],
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ListGameServerOperationsResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/StartGameServer', async (route) => {
    startRequest = fromJson(StartGameServerRequestSchema, route.request().postDataJSON())
    const response = create(StartGameServerResponseSchema, {})
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(StartGameServerResponseSchema, response),
    })
  })

  await gotoAppPage(page, `/game-servers/${state.gameServerId}/operations`)
  await expect(page.getByRole('heading', { name: 'Operations workbench' })).toBeVisible()
  await expect(page.getByTestId('operations-workbench')).toHaveCount(0)
  await expect(
    page.getByRole('heading', { name: 'Start Deterministic 7DTD Server to run operations' }),
  ).toBeVisible()
  await page.getByTestId('start-server').click()
  await expect(page.getByTestId('start-server')).toHaveText('Start requested')
  await expect(page.getByText('Open Overview to follow the lifecycle state.')).toBeVisible()
  expect(startRequest).toMatchObject({ serverId: state.gameServerId })

  await page.unrouteAll({ behavior: 'wait' })
})

function addAdministratorOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'player_access.add_administrator',
    name: 'Add administrator',
    summary: 'Grant a Player an explicit native permission level.',
    category: 'Player access',
    permissionId: 'game_server.players.manage',
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'player',
        label: 'Player',
        description: 'Choose a known Player or enter a stable platform identity.',
        type: GameOperationFieldType.PLAYER_IDENTITY,
        required: true,
        allowManual: true,
        validationPattern: '^[A-Za-z]+_[A-Za-z0-9_]+$',
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Player One',
            value: 'Steam_PLAYER_1',
            description: 'Platform: Steam_PLAYER_1 · Cross-platform: EOS_PLAYER_1',
          }),
          create(GameOperationFieldOptionSchema, {
            label: 'Player Two',
            value: 'EOS_PLAYER_2',
            description: 'Cross-platform: EOS_PLAYER_2',
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'permission_level',
        label: 'Permission level',
        description: 'Lower native values grant more access.',
        type: GameOperationFieldType.INTEGER,
        required: true,
        defaultValue: '0',
        allowExactValue: true,
        minValue: 0,
        maxValue: 1000,
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Maximum permission',
            value: '0',
          }),
        ],
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review administrator access',
      effect: 'The selected Player will be added as an administrator.',
      caution: 'Lower permission levels grant more access.',
    }),
  })
}

function kickOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'player_moderation.kick',
    name: 'Kick Player',
    summary: 'Disconnect a Player from the game server.',
    category: 'Player moderation',
    permissionId: 'game_server.players.manage',
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'player',
        label: 'Player',
        type: GameOperationFieldType.PLAYER_IDENTITY,
        required: true,
        allowManual: true,
        validationPattern: '^[^"\\\\\\x00-\\x1F\\x7F]{1,256}$',
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Player Two',
            value: 'EOS_PLAYER_2',
            description: 'Cross-platform: EOS_PLAYER_2',
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'reason',
        label: 'Reason',
        type: GameOperationFieldType.TEXT,
        validationPattern: '^[^\\x00-\\x1F\\x7F]{0,256}$',
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review Kick Player',
      effect: 'The selected Player will be disconnected.',
      caution: 'The final Player state cannot be read back.',
    }),
  })
}

function setGameTimeOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'server_control.set_game_time',
    name: 'Set game time',
    summary: 'Move the current world clock to a preset or an exact day and time.',
    category: 'Server control',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'time',
        label: 'World time',
        description: 'Choose a preset or enter an exact day, hour, and minute.',
        type: GameOperationFieldType.TEXT,
        required: true,
        defaultValue: 'day',
        allowManual: true,
        validationPattern:
          '^(?:day|night|[1-9][0-9]{0,9} (?:[0-9]|1[0-9]|2[0-3]) (?:[0-9]|[1-5][0-9]))$',
        options: [
          create(GameOperationFieldOptionSchema, { label: 'Day', value: 'day' }),
          create(GameOperationFieldOptionSchema, { label: 'Night', value: 'night' }),
        ],
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review world time change',
      effect: "The selected game server's world clock will move to the chosen time.",
      caution: 'Changing world time can affect active Players and game events.',
    }),
  })
}

function saveWorldOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'server_control.save_world',
    name: 'Save world',
    summary: 'Save the current world state immediately.',
    category: 'Server control',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.ROUTINE,
    available: true,
    review: create(GameOperationReviewSchema, {
      title: 'Review world save',
      effect: 'The selected game server will save its current world state.',
    }),
  })
}

function setTemperatureUnitOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'server_control.set_temperature_unit',
    name: 'Set temperature unit',
    summary: 'Choose how the game reports temperatures.',
    category: 'Server control',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.ROUTINE,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'unit',
        label: 'Temperature unit',
        description: 'Choose Fahrenheit or Celsius.',
        type: GameOperationFieldType.ENUM,
        required: true,
        defaultValue: 'F',
        options: [
          create(GameOperationFieldOptionSchema, { label: 'Fahrenheit', value: 'F' }),
          create(GameOperationFieldOptionSchema, { label: 'Celsius', value: 'C' }),
        ],
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review temperature unit change',
      effect: 'The selected game server will report temperatures using the chosen unit.',
    }),
  })
}

function giveItemOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'player_assistance.give_item',
    name: 'Give item',
    summary: 'Drop an item stack in front of a Player.',
    category: 'Player assistance',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'player',
        label: 'Player',
        type: GameOperationFieldType.PLAYER_IDENTITY,
        required: true,
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Player Two',
            value: 'EOS_PLAYER_2',
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'item',
        label: 'Item',
        type: GameOperationFieldType.TEXT,
        required: true,
        allowManual: true,
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Wood',
            value: 'resourceWood',
            description: 'resourceWood',
            category: 'Resources / Basics',
            iconUrl:
              'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="36" height="36"%3E%3Crect width="36" height="36" fill="%238b5a2b"/%3E%3C/svg%3E',
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'amount',
        label: 'Amount',
        type: GameOperationFieldType.INTEGER,
        required: true,
        defaultValue: '1',
        minValue: 1,
        maxValue: 1000,
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review item grant',
      effect: 'The item stack will be dropped in front of the selected Player.',
      caution: 'The item name must exactly match a server item definition.',
    }),
  })
}

function setWeatherOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'world_events.set_weather',
    name: 'Set weather',
    summary: 'Restore natural weather or start rain or snowfall.',
    category: 'World events',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'weather',
        label: 'Weather',
        type: GameOperationFieldType.ENUM,
        required: true,
        defaultValue: 'natural',
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review weather change',
      effect: 'The active world weather will change to the selected state.',
      caution: 'Weather changes affect every online Player.',
    }),
  })
}

function spawnAirdropOperation() {
  return worldEventOperation('world_events.spawn_airdrop', 'Spawn airdrop')
}

function spawnWanderingHordeOperation() {
  return worldEventOperation('world_events.spawn_wandering_horde', 'Spawn wandering horde')
}

function worldEventOperation(id: string, name: string) {
  return create(GameOperationDescriptorSchema, {
    id,
    name,
    summary: name,
    category: 'World events',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.CAUTION,
    available: true,
    review: create(GameOperationReviewSchema, {
      title: `Review ${name.toLowerCase()}`,
      effect: `The game server will ${name.toLowerCase()}.`,
      caution: 'This world event cannot be automatically reversed.',
    }),
  })
}

function broadcastOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'communication.broadcast_message',
    name: 'Send server announcement',
    summary: 'Broadcast an announcement to every connected Player.',
    category: 'Communication',
    permissionId: 'game_server.console',
    risk: GameOperationRisk.ROUTINE,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'message',
        label: 'Announcement',
        description: 'Enter the message every connected Player should receive.',
        type: GameOperationFieldType.TEXT,
        required: true,
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review server announcement',
      effect: 'The announcement will be sent to every connected Player.',
    }),
  })
}

function versionOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'server_information.version',
    name: 'Inspect game version',
    summary: 'Read the running game version and loaded mods.',
    category: 'Server information',
    permissionId: 'game_server.view',
    risk: GameOperationRisk.ROUTINE,
    available: true,
    review: create(GameOperationReviewSchema, {
      title: 'Review version query',
      effect: 'The running game version and loaded mods will be read.',
    }),
  })
}
