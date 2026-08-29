import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import type { DOMWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
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
  ListGameServerOperationsResponseSchema,
} from '@/proto/xylona_pb'
import type { GameOperationDescriptor } from '@/proto/xylona_pb'
import GameServerOperations from './GameServerOperations.vue'

const mocks = vi.hoisted(() => ({
  executeGameServerOperation: vi.fn(),
  listGameServerOperations: vi.fn(),
}))
const screen = vi.hoisted(() => ({ mobile: false }))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ screen: { lt: { md: screen.mobile } } }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    executeGameServerOperation: mocks.executeGameServerOperation,
    listGameServerOperations: mocks.listGameServerOperations,
  }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

function addAdministrator(overrides: Partial<GameOperationDescriptor> = {}) {
  return create(GameOperationDescriptorSchema, {
    id: 'player_access.add_administrator',
    name: 'Add administrator',
    summary: 'Grant a Player an explicit native permission level.',
    category: 'Player access',
    permissionId: 'game_server.players.manage',
    risk: GameOperationRisk.CAUTION,
    available: true,
    availabilityRequirements: ['Server online'],
    fields: [
      create(GameOperationFieldSchema, {
        id: 'player',
        label: 'Player',
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
          create(GameOperationFieldOptionSchema, { label: 'Maximum permission', value: '0' }),
          create(GameOperationFieldOptionSchema, { label: 'Default Player level', value: '1000' }),
        ],
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review administrator access',
      effect: 'The selected Player will be added as an administrator.',
      caution: 'Lower permission levels grant more access.',
    }),
    ...overrides,
  })
}

function runtimeOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'runtime.restart',
    name: 'Restart server',
    summary: 'Restart the current server process.',
    category: 'Runtime',
    risk: GameOperationRisk.ROUTINE,
    available: true,
  })
}

async function mountOperations(
  operations = [addAdministrator(), runtimeOperation()],
  attachTo?: HTMLElement,
) {
  mocks.listGameServerOperations.mockResolvedValue(
    create(ListGameServerOperationsResponseSchema, {
      gameServerName: 'Test Server',
      operations,
    }),
  )
  const wrapper = mount(GameServerOperations, {
    ...(attachTo ? { attachTo } : {}),
    global: {
      stubs: {
        'q-icon': { props: ['name'], template: '<i :data-icon="name" />' },
        'q-spinner': { template: '<span>Loading</span>' },
      },
    },
  })
  await flushPromises()
  return wrapper
}

async function pressNativeButton(button: DOMWrapper<HTMLButtonElement>, key: 'Enter' | ' ') {
  button.element.focus()
  expect(document.activeElement).toBe(button.element)
  // happy-dom emits the key event but leaves native button activation to the test.
  await button.trigger(key === 'Enter' ? 'keydown' : 'keyup', { key })
  button.element.click()
  await flushPromises()
}

describe('GameServerOperations', () => {
  beforeEach(() => {
    mocks.executeGameServerOperation.mockReset()
    mocks.listGameServerOperations.mockReset()
    screen.mobile = false
  })

  it('filters the ledger by search and category', async () => {
    const wrapper = await mountOperations()
    expect(wrapper.get('h1').text()).toContain('Operation Ledger')
    expect(wrapper.findAll('[data-testid="operation-row"]')).toHaveLength(2)

    await wrapper.get('[data-testid="operation-search"]').setValue('administrator')
    expect(wrapper.findAll('[data-testid="operation-row"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('Restart server')

    await wrapper.get('[data-testid="operation-search"]').setValue('')
    await wrapper.get('[data-testid="category-Runtime"]').trigger('click')
    expect(wrapper.findAll('[data-testid="operation-row"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Restart server')
  })

  it('keeps one keyboard-accessible inline expansion open', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const wrapper = await mountOperations(undefined, host)
    const rows = wrapper.findAll<HTMLButtonElement>('[data-testid="operation-toggle"]')
    const firstRow = rows[0]
    const secondRow = rows[1]
    if (!firstRow || !secondRow) {
      throw new Error('Expected two operation rows')
    }
    expect(firstRow.element.tagName).toBe('BUTTON')
    expect(firstRow.attributes('type')).toBe('button')
    expect(firstRow.attributes('aria-expanded')).toBe('false')
    await pressNativeButton(firstRow, 'Enter')
    expect(firstRow.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('[data-testid="operation-expansion"]')).toHaveLength(1)

    await pressNativeButton(secondRow, ' ')
    expect(wrapper.findAll('[data-testid="operation-expansion"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Restart server')
    expect(wrapper.text()).not.toContain('Review administrator access')
    wrapper.unmount()
    host.remove()
  })

  it('supports known and manual Player identities, exact values, and plain review copy', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const wrapper = await mountOperations([addAdministrator({ rendererKey: 'constructor' })], host)
    await pressNativeButton(
      wrapper.get<HTMLButtonElement>('[data-testid="operation-toggle"]'),
      'Enter',
    )

    expect(wrapper.text()).toContain('Steam_PLAYER_1')
    expect(wrapper.text()).toContain('EOS_PLAYER_1')
    expect(wrapper.find('[data-testid="generic-operation-form"]').exists()).toBe(true)

    await wrapper.get('input[placeholder="Name or stable ID"]').setValue('EOS_PLAYER_1')
    const knownPlayers = wrapper.findAll<HTMLButtonElement>('.player-option')
    expect(knownPlayers).toHaveLength(1)
    const knownPlayer = knownPlayers[0]
    if (!knownPlayer) {
      throw new Error('Expected a matching known Player')
    }
    await pressNativeButton(knownPlayer, ' ')
    expect(wrapper.get('[data-testid="operation-review"]').text()).toContain(
      'Player One — Steam_PLAYER_1',
    )
    expect(wrapper.get('[data-testid="operation-review"]').text()).toContain(
      'Maximum permission — 0',
    )

    await pressNativeButton(
      wrapper.get<HTMLButtonElement>('[data-testid="player-mode-manual"]'),
      'Enter',
    )
    const manualIdentity = wrapper.get<HTMLInputElement>('[data-testid="manual-player-identity"]')
    manualIdentity.element.focus()
    expect(document.activeElement).toBe(manualIdentity.element)
    await manualIdentity.setValue('not valid')
    expect(wrapper.text()).toContain('Use a platform-prefixed identity')
    const invalidIdentity = wrapper.get('[data-testid="manual-player-identity"]')
    expect(invalidIdentity.attributes('aria-invalid')).toBe('true')
    expect(
      invalidIdentity.element.closest('[role="group"]')?.getAttribute('aria-describedby'),
    ).toContain('player-error')

    await manualIdentity.setValue('Steam_PLAYER_42')
    const exactValue = wrapper.get<HTMLInputElement>('[data-testid="permission-exact-value"]')
    exactValue.element.focus()
    expect(document.activeElement).toBe(exactValue.element)
    await exactValue.setValue('42')
    const review = wrapper.get('[data-testid="operation-review"]').text()
    expect(review).toContain('Steam_PLAYER_42')
    expect(review).toContain('42')
    expect(review).toContain('Execute Add administrator')
    expect(review).not.toMatch(/POST|\/api\/|command/i)
    wrapper.unmount()
    host.remove()
  })

  it('executes typed semantic values once and retains the confirmed result', async () => {
    let resolveExecution: ((value: unknown) => void) | undefined
    mocks.executeGameServerOperation.mockReturnValue(
      new Promise((resolve) => {
        resolveExecution = resolve
      }),
    )
    const wrapper = await mountOperations([addAdministrator()])
    await wrapper.get('[data-testid="operation-toggle"]').trigger('click')

    const review = wrapper.get('[data-testid="operation-review"]')
    expect(review.text()).toContain('Test Server')
    expect(review.text()).toContain('Caution')
    expect(review.text()).toContain('The selected Player will be added as an administrator.')

    const execute = wrapper.get<HTMLButtonElement>('[data-testid="execute-operation"]')
    await execute.trigger('click')
    await execute.trigger('click')
    expect(mocks.executeGameServerOperation).toHaveBeenCalledTimes(1)
    expect(execute.attributes('disabled')).toBeDefined()
    expect(execute.text()).toContain('Executing')

    const request = mocks.executeGameServerOperation.mock.calls[0]?.[0]
    expect(request).toMatchObject({
      gameServerId: 'server-1',
      operationId: 'player_access.add_administrator',
      values: [
        { fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } },
        { fieldId: 'permission_level', value: { case: 'integerValue', value: 0n } },
      ],
    })
    expect(
      JSON.stringify(request, (_key, value) =>
        typeof value === 'bigint' ? value.toString() : value,
      ),
    ).not.toMatch(/POST|\/api\/|command/i)

    resolveExecution?.(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.CONFIRMED,
          message: 'Administrator access was confirmed.',
          transportDetails: create(GameOperationTransportDetailsSchema, {
            method: '7 Days to Die native dashboard',
            verification: 'User permission read-back',
          }),
        }),
      }),
    )
    await flushPromises()

    const result = wrapper.get('[data-testid="operation-result"]')
    expect(result.attributes('role')).toBe('status')
    expect(result.text()).toContain('Confirmed')
    expect(result.text()).toContain('Administrator access was confirmed.')
    expect(result.text()).toContain('User permission read-back')
    expect(wrapper.get('[data-testid="operation-review"]').exists()).toBe(true)
  })

  it('exposes a native mobile category control', async () => {
    screen.mobile = true
    const wrapper = await mountOperations()
    const picker = wrapper.get<HTMLSelectElement>('[data-testid="mobile-category-picker"]')
    expect(picker.element.tagName).toBe('SELECT')
    expect(wrapper.find('.operations-categories').exists()).toBe(false)
    await picker.setValue('Runtime')
    expect(wrapper.findAll('[data-testid="operation-row"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Restart server')
  })
})
