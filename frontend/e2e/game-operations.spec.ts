import { create, fromJson, toJson } from '@bufbuild/protobuf'
import { expect, test } from '@playwright/test'

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

test('executes known and manual Player workflows through visible results', async ({ page }) => {
  const state = requireTestState()
  const requests: ExecuteGameServerOperationRequest[] = []

  await page.route('**/xylona.Xylona/GetGameServer', async (route) => {
    const original = await route.fetch()
    const response = fromJson(GetGameServerResponseSchema, await original.json())
    if (!response.gameServer) {
      throw new Error('Expected the seeded game server')
    }
    response.gameServer.gameId = '7_days_to_die'
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
      operations: [addAdministratorOperation()],
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ListGameServerOperationsResponseSchema, response),
    })
  })

  await page.route('**/xylona.Xylona/ExecuteGameServerOperation', async (route) => {
    requests.push(fromJson(ExecuteGameServerOperationRequestSchema, route.request().postDataJSON()))
    const confirmed = requests.length === 1
    const response = create(ExecuteGameServerOperationResponseSchema, {
      result: create(GameOperationResultSchema, {
        classification: confirmed
          ? GameOperationResultClassification.CONFIRMED
          : GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
        message: confirmed
          ? 'Administrator access was confirmed.'
          : 'The server accepted Add administrator, but read-back was unavailable.',
        transportDetails: create(GameOperationTransportDetailsSchema, {
          method: '7 Days to Die native dashboard',
          verification: 'User permission read-back',
        }),
      }),
    })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: toJson(ExecuteGameServerOperationResponseSchema, response),
    })
  })

  await gotoAppPage(page, `/game-servers/${state.gameServerId}/operations`)
  await expect(page.getByRole('heading', { name: 'Operation Ledger' })).toBeVisible()
  await page.getByTestId('operation-toggle').click()
  await page.locator('.player-option').filter({ hasText: 'Player Two' }).click()
  await expect(page.getByTestId('operation-review')).toContainText('Player Two — EOS_PLAYER_2')
  await page.getByTestId('execute-operation').click()
  await expect(page.getByTestId('operation-result')).toContainText('Confirmed')
  await expect(page.getByTestId('operation-result')).toContainText(
    'Administrator access was confirmed.',
  )

  await page.getByTestId('player-mode-manual').click()
  await page.getByTestId('manual-player-identity').fill('EOS_PLAYER_42')
  await page.getByTestId('permission-exact-value').fill('42')
  await page.getByTestId('execute-operation').click()
  await expect(page.getByTestId('operation-result')).toContainText('Accepted, not verified')

  expect(requests).toHaveLength(2)
  expect(requests[0]).toMatchObject({
    gameServerId: state.gameServerId,
    operationId: 'player_access.add_administrator',
    values: [
      { fieldId: 'player', value: { case: 'stringValue', value: 'EOS_PLAYER_2' } },
      { fieldId: 'permission_level', value: { case: 'integerValue', value: 0n } },
    ],
  })
  expect(requests[1]?.values).toMatchObject([
    { fieldId: 'player', value: { case: 'stringValue', value: 'EOS_PLAYER_42' } },
    { fieldId: 'permission_level', value: { case: 'integerValue', value: 42n } },
  ])

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
