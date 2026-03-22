import { test, expect } from '@playwright/test'
import { BACKEND_URL, apiLogin, loadTestState, type ApiCookies } from './helpers'

// ---------------------------------------------------------------------------
// API helpers for config schemas
// ---------------------------------------------------------------------------

async function apiGetConfigSchemas(cookies: ApiCookies, gameId: string): Promise<unknown[]> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/GetGameConfigSchemas`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ game_id: gameId }),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as { config_schemas_json?: string }
  if (!data.config_schemas_json) return []
  return JSON.parse(data.config_schemas_json) as unknown[]
}

async function apiSetConfigSchemas(
  cookies: ApiCookies,
  gameId: string,
  schemas: unknown[],
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/UpdateGameConfigSchemas`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_id: gameId,
      config_schemas_json: JSON.stringify(schemas),
    }),
  })
  if (!resp.ok) {
    throw new Error(`UpdateGameConfigSchemas failed: ${resp.status} ${await resp.text()}`)
  }
}

// ---------------------------------------------------------------------------
// Test schema with 3 groups and explicit x-order on each field
// ---------------------------------------------------------------------------

const TEST_SCHEMA = [
  {
    path: 'test-ordering.properties',
    format: 'properties',
    category: 'E2E Test',
    generate_before_start: false,
    schema: {
      type: 'object',
      'x-groups': ['network', 'gameplay', 'performance'],
      properties: {
        'server-ip': {
          type: 'string',
          title: 'Server IP',
          'x-group': 'network',
          'x-order': 0,
        },
        'server-port': {
          type: 'integer',
          title: 'Server Port',
          'x-group': 'network',
          'x-order': 1,
        },
        gamemode: {
          type: 'string',
          title: 'Game Mode',
          'x-group': 'gameplay',
          'x-order': 0,
        },
        difficulty: {
          type: 'string',
          title: 'Difficulty',
          'x-group': 'gameplay',
          'x-order': 1,
        },
        'max-players': {
          type: 'integer',
          title: 'Max Players',
          'x-group': 'performance',
          'x-order': 0,
        },
        'view-distance': {
          type: 'integer',
          title: 'View Distance',
          'x-group': 'performance',
          'x-order': 1,
        },
      },
    },
  },
]

// Schema WITHOUT x-groups — simulates a legacy or freshly-created schema.
const TEST_SCHEMA_NO_XGROUPS = [
  {
    path: 'test-no-xgroups.properties',
    format: 'properties',
    category: 'E2E Test',
    generate_before_start: false,
    schema: {
      type: 'object',
      properties: {
        'server-ip': {
          type: 'string',
          title: 'Server IP',
          'x-group': 'network',
        },
        'server-port': {
          type: 'integer',
          title: 'Server Port',
          'x-group': 'network',
        },
        gamemode: {
          type: 'string',
          title: 'Game Mode',
          'x-group': 'gameplay',
        },
        difficulty: {
          type: 'string',
          title: 'Difficulty',
          'x-group': 'gameplay',
        },
      },
    },
  },
]

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Config schema ordering — without x-groups (legacy schemas)', () => {
  let cookies: ApiCookies
  let gameId: string
  let originalSchemas: unknown[]

  test.beforeAll(async () => {
    const state = loadTestState()
    if (!state.gameId) return
    gameId = state.gameId

    cookies = await apiLogin('e2e-admin', 'e2e-admin')
    originalSchemas = await apiGetConfigSchemas(cookies, gameId)
    await apiSetConfigSchemas(cookies, gameId, TEST_SCHEMA_NO_XGROUPS)
  })

  test.afterAll(async () => {
    if (!gameId) return
    await apiSetConfigSchemas(cookies, gameId, originalSchemas)
  })

  test('can move a group down when x-groups is absent', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameId) {
      test.skip(true, 'No game available')
      return
    }

    await page.goto(`/games/${gameId}/config-schema/0`)
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    // Verify initial group order: network first (first-occurrence from properties)
    const groupTitles = page.locator('.schema-group-title')
    await expect(groupTitles).toHaveCount(2, { timeout: 10_000 })

    const titlesBefore = await groupTitles.allTextContents()
    expect(titlesBefore[0]?.toLowerCase()).toContain('network')
    expect(titlesBefore[1]?.toLowerCase()).toContain('gameplay')

    // Hover over the first group header and click move-down
    const firstGroupHeader = page.locator('.schema-group-header').first()
    await firstGroupHeader.hover()

    const moveDownBtn = firstGroupHeader.locator('.group-move-btn').last()
    await expect(moveDownBtn).toBeVisible({ timeout: 5_000 })
    await moveDownBtn.click()
    await page.waitForTimeout(500)

    // BUG: without x-groups, groupOrder is empty and moveGroupDown is a no-op.
    // After the fix, gameplay should now be first, network second.
    const titlesAfter = await page.locator('.schema-group-title').allTextContents()
    expect(titlesAfter[0]?.toLowerCase()).toContain('gameplay')
    expect(titlesAfter[1]?.toLowerCase()).toContain('network')
  })
})

test.describe('Config schema ordering', () => {
  let cookies: ApiCookies
  let gameId: string
  let originalSchemas: unknown[]

  test.beforeAll(async () => {
    const state = loadTestState()
    if (!state.gameId) {
      return
    }
    gameId = state.gameId

    cookies = await apiLogin('e2e-admin', 'e2e-admin')
    originalSchemas = await apiGetConfigSchemas(cookies, gameId)
    await apiSetConfigSchemas(cookies, gameId, TEST_SCHEMA)
  })

  test.afterAll(async () => {
    if (!gameId) return
    await apiSetConfigSchemas(cookies, gameId, originalSchemas)
  })

  test('displays fields in schema-defined order', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameId) {
      test.skip(true, 'No game available')
      return
    }

    await page.goto(`/games/${gameId}/config-schema/0`)
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    // Verify group headers appear in the correct order: Network, Gameplay, Performance
    const groupTitles = page.locator('.schema-group-title')
    await expect(groupTitles).toHaveCount(3, { timeout: 10_000 })

    const titles = await groupTitles.allTextContents()
    expect(titles[0]?.toLowerCase()).toContain('network')
    expect(titles[1]?.toLowerCase()).toContain('gameplay')
    expect(titles[2]?.toLowerCase()).toContain('performance')

    // Verify field order within each group by checking .field-card-key elements
    const fieldKeys = page.locator('.field-card-key')
    const allKeys = await fieldKeys.allTextContents()

    // Expected order: network fields, gameplay fields, performance fields
    const expectedOrder = [
      'server-ip',
      'server-port',
      'gamemode',
      'difficulty',
      'max-players',
      'view-distance',
    ]
    expect(allKeys.map((k) => k.trim())).toEqual(expectedOrder)
  })

  test('can move a field up within its group', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameId) {
      test.skip(true, 'No game available')
      return
    }

    await page.goto(`/games/${gameId}/config-schema/0`)
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    // Find the second field in the gameplay group ("difficulty") and move it up.
    // The field cards within each group contain move-up / move-down buttons.
    const gameplayGroup = page.locator('.schema-field-group').filter({
      has: page.locator('.schema-group-title', { hasText: /gameplay/i }),
    })
    await expect(gameplayGroup).toBeVisible()

    // The second field card in the gameplay group
    const secondFieldCard = gameplayGroup.locator('.field-card').nth(1)
    await expect(secondFieldCard).toBeVisible()

    // Hover to reveal action buttons, then click move-up
    await secondFieldCard.hover()
    const moveUpBtn = secondFieldCard
      .locator('button', {
        has: page.locator('i:has-text("arrow_upward"), .q-icon:has-text("arrow_upward")'),
      })
      .first()
    await moveUpBtn.click()
    await page.waitForTimeout(500)

    // Verify the field order has swapped: difficulty is now first, gamemode second
    const gameplayKeys = gameplayGroup.locator('.field-card-key')
    const keys = await gameplayKeys.allTextContents()
    expect(keys.map((k) => k.trim())).toEqual(['difficulty', 'gamemode'])

    // Save the schema
    await page.getByRole('button', { name: 'Save Schema' }).click()
    await expect(page.locator('.q-notification')).toBeVisible({ timeout: 10_000 })

    // Reload and verify persistence
    await page.reload()
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    const gameplayGroupAfter = page.locator('.schema-field-group').filter({
      has: page.locator('.schema-group-title', { hasText: /gameplay/i }),
    })
    const keysAfter = await gameplayGroupAfter.locator('.field-card-key').allTextContents()
    expect(keysAfter.map((k) => k.trim())).toEqual(['difficulty', 'gamemode'])

    // Restore original field order for subsequent tests
    const firstFieldCard = gameplayGroupAfter.locator('.field-card').nth(0)
    await firstFieldCard.hover()
    const moveDownBtn = firstFieldCard
      .locator('button', {
        has: page.locator('i:has-text("arrow_downward"), .q-icon:has-text("arrow_downward")'),
      })
      .first()
    await moveDownBtn.click()
    await page.waitForTimeout(300)
    await page.getByRole('button', { name: 'Save Schema' }).click()
    await expect(page.locator('.q-notification')).toBeVisible({ timeout: 10_000 })
  })

  test('can move a group down', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameId) {
      test.skip(true, 'No game available')
      return
    }

    await page.goto(`/games/${gameId}/config-schema/0`)
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    // Verify initial group order
    const groupTitles = page.locator('.schema-group-title')
    await expect(groupTitles).toHaveCount(3, { timeout: 10_000 })

    const titlesBefore = await groupTitles.allTextContents()
    expect(titlesBefore[0]?.toLowerCase()).toContain('network')

    // Hover over the first group header to reveal move buttons
    const firstGroupHeader = page.locator('.schema-group-header').first()
    await firstGroupHeader.hover()

    // Click the move-down button on the first group
    const moveDownBtn = firstGroupHeader.locator('.group-move-btn').last()
    await expect(moveDownBtn).toBeVisible({ timeout: 5_000 })
    await moveDownBtn.click()
    await page.waitForTimeout(500)

    // Verify group order has changed: Gameplay is now first, Network second
    const titlesAfter = await page.locator('.schema-group-title').allTextContents()
    expect(titlesAfter[0]?.toLowerCase()).toContain('gameplay')
    expect(titlesAfter[1]?.toLowerCase()).toContain('network')
    expect(titlesAfter[2]?.toLowerCase()).toContain('performance')

    // Save the schema
    await page.getByRole('button', { name: 'Save Schema' }).click()
    await expect(page.locator('.q-notification')).toBeVisible({ timeout: 10_000 })

    // Reload and verify persistence
    await page.reload()
    await expect(page.locator('.schema-editor')).toBeVisible({ timeout: 15_000 })

    const titlesReloaded = await page.locator('.schema-group-title').allTextContents()
    expect(titlesReloaded[0]?.toLowerCase()).toContain('gameplay')
    expect(titlesReloaded[1]?.toLowerCase()).toContain('network')
    expect(titlesReloaded[2]?.toLowerCase()).toContain('performance')

    // Restore original group order: move gameplay (now first) down so network is first again
    const restoreGroupHeader = page.locator('.schema-group-header').first()
    await restoreGroupHeader.hover()
    const restoreMoveDown = restoreGroupHeader.locator('.group-move-btn').last()
    await expect(restoreMoveDown).toBeVisible({ timeout: 5_000 })
    await restoreMoveDown.click()
    await page.waitForTimeout(300)
    await page.getByRole('button', { name: 'Save Schema' }).click()
    await expect(page.locator('.q-notification')).toBeVisible({ timeout: 10_000 })
  })
})
