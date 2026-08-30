import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameServerSchema, Status } from '@/proto/shared_pb'

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
  GetSevenDaysToDieWebAPIStatusResponseSchema,
  ListGameServerOperationsResponseSchema,
  SevenDaysToDieGameTimeSchema,
  SevenDaysToDieWebAPIStatusSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import type { GameOperationDescriptor, GameOperationField } from '@/proto/xylona_pb'
import GameServerOperations from './GameServerOperations.vue'

const mocks = vi.hoisted(() => ({
  executeGameServerOperation: vi.fn(),
  getSevenDaysToDieWebAPIStatus: vi.fn(),
  getGameServer: vi.fn(),
  listGameServerOperations: vi.fn(),
  startGameServer: vi.fn(),
}))
const route = vi.hoisted(() => ({
  current: { params: { id: 'server-1' }, query: {} as Record<string, string> },
}))
const localStorageValues = new Map<string, string>()
const localStorageMock = {
  getItem: vi.fn((key: string) => localStorageValues.get(key) ?? null),
  setItem: vi.fn((key: string, value: string) => localStorageValues.set(key, value)),
  removeItem: vi.fn((key: string) => localStorageValues.delete(key)),
  clear: vi.fn(() => localStorageValues.clear()),
}

vi.stubGlobal('localStorage', localStorageMock)

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    executeGameServerOperation: mocks.executeGameServerOperation,
    getGameServer: mocks.getGameServer,
    getSevenDaysToDieWebAPIStatus: mocks.getSevenDaysToDieWebAPIStatus,
    listGameServerOperations: mocks.listGameServerOperations,
    startGameServer: mocks.startGameServer,
  }),
  ConnectErrorToString: () => 'Connection failed',
}))

vi.mock('vue-router', () => ({
  useRoute: () => route.current,
}))

function addAdministrator(overrides: Partial<GameOperationDescriptor> = {}) {
  return create(GameOperationDescriptorSchema, {
    id: 'player_access.add_administrator',
    name: 'Add administrator',
    summary: 'Grant a player an explicit native permission level.',
    category: 'Player access',
    risk: GameOperationRisk.CAUTION,
    available: true,
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
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'permission_level',
        label: 'Permission level',
        type: GameOperationFieldType.INTEGER,
        required: true,
        defaultValue: '0',
        allowExactValue: true,
        minValue: 0,
        maxValue: 1000,
      }),
    ],
    review: create(GameOperationReviewSchema, {
      title: 'Review administrator access',
      effect: 'The selected player will be added as an administrator.',
      caution: 'Lower permission levels grant more access.',
    }),
    ...overrides,
  })
}

function operation(
  id: string,
  name: string,
  risk = GameOperationRisk.ROUTINE,
  fields: GameOperationField[] = [],
) {
  return create(GameOperationDescriptorSchema, {
    id,
    name,
    summary: name,
    risk,
    available: true,
    fields,
    review: create(GameOperationReviewSchema, { title: name, effect: name }),
  })
}

function playerIdentityField(id = 'player', label = 'Player') {
  return create(GameOperationFieldSchema, {
    id,
    label,
    type: GameOperationFieldType.PLAYER_IDENTITY,
    required: true,
    validationPattern: '^[A-Za-z]+_[A-Za-z0-9_]+$',
    options: [
      create(GameOperationFieldOptionSchema, {
        label: 'Player One',
        value: 'Steam_PLAYER_1',
      }),
      create(GameOperationFieldOptionSchema, {
        label: 'Player Two',
        value: 'Steam_PLAYER_2',
      }),
    ],
  })
}

function textField(id: string, label: string) {
  return create(GameOperationFieldSchema, {
    id,
    label,
    type: GameOperationFieldType.TEXT,
    required: true,
  })
}

function integerField(id: string, label: string, defaultValue: string, maxValue: number) {
  return create(GameOperationFieldSchema, {
    id,
    label,
    type: GameOperationFieldType.INTEGER,
    required: true,
    defaultValue,
    minValue: 1,
    maxValue,
  })
}

function messagePlayerOperation() {
  return create(GameOperationDescriptorSchema, {
    id: 'communication.message_player',
    name: 'Message player',
    risk: GameOperationRisk.ROUTINE,
    available: true,
    fields: [
      create(GameOperationFieldSchema, {
        id: 'player',
        label: 'Player',
        type: GameOperationFieldType.PLAYER_IDENTITY,
        required: true,
        options: [
          create(GameOperationFieldOptionSchema, {
            label: 'Player One',
            value: 'Steam_PLAYER_1',
          }),
        ],
      }),
      create(GameOperationFieldSchema, {
        id: 'message',
        label: 'Message',
        type: GameOperationFieldType.TEXT,
        required: true,
      }),
    ],
  })
}

function commandPermissionOperation(
  id: 'permissions.set_command_permission' | 'permissions.reset_command_permission',
  name: string,
) {
  const fields = [
    create(GameOperationFieldSchema, {
      id: 'command',
      label: 'Command',
      type: GameOperationFieldType.TEXT,
      required: true,
    }),
  ]
  if (id === 'permissions.set_command_permission') {
    fields.push(
      create(GameOperationFieldSchema, {
        id: 'permission_level',
        label: 'Permission level',
        type: GameOperationFieldType.INTEGER,
        required: true,
        defaultValue: '0',
        minValue: 0,
        maxValue: 1000,
      }),
    )
  }
  return create(GameOperationDescriptorSchema, {
    id,
    name,
    risk: GameOperationRisk.CAUTION,
    available: true,
    fields,
    review: create(GameOperationReviewSchema, { title: name, effect: name }),
  })
}

function moderationOperation(
  id: 'player_moderation.kick' | 'player_moderation.ban' | 'player_moderation.unban',
  name: string,
  risk: GameOperationRisk,
) {
  const fields = [
    create(GameOperationFieldSchema, {
      id: 'player',
      label: 'Player',
      type: GameOperationFieldType.PLAYER_IDENTITY,
      required: true,
      options: [
        create(GameOperationFieldOptionSchema, {
          label: 'Player One',
          value: 'Steam_PLAYER_1',
        }),
      ],
    }),
  ]
  if (id !== 'player_moderation.unban') {
    fields.push(
      create(GameOperationFieldSchema, {
        id: 'reason',
        label: 'Reason',
        type: GameOperationFieldType.TEXT,
      }),
    )
  }
  return create(GameOperationDescriptorSchema, {
    id,
    name,
    risk,
    available: true,
    fields,
    review: create(GameOperationReviewSchema, { title: name, effect: name }),
  })
}

const commonOperations = [
  addAdministrator(),
  operation(
    'player_access.remove_administrator',
    'Remove administrator',
    GameOperationRisk.CAUTION,
  ),
  operation('player_access.allowlist_add', 'Add to allowlist'),
  operation('player_access.allowlist_remove', 'Remove from allowlist', GameOperationRisk.CAUTION),
  moderationOperation('player_moderation.kick', 'Kick player', GameOperationRisk.CAUTION),
  moderationOperation('player_moderation.ban', 'Ban player', GameOperationRisk.CAUTION),
  moderationOperation('player_moderation.unban', 'Unban player', GameOperationRisk.ROUTINE),
  operation('player_assistance.teleport_to_player', 'Teleport player', GameOperationRisk.CAUTION, [
    playerIdentityField(),
    playerIdentityField('destination', 'Destination player'),
  ]),
  operation('player_assistance.give_item', 'Give item', GameOperationRisk.CAUTION, [
    playerIdentityField(),
    textField('item', 'Item'),
    integerField('amount', 'Amount', '1', 1000),
  ]),
  operation('player_assistance.give_experience', 'Give experience', GameOperationRisk.CAUTION, [
    playerIdentityField(),
    integerField('experience', 'Experience', '1000', 1000000),
  ]),
  operation('player_assistance.apply_buff', 'Apply buff', GameOperationRisk.CAUTION, [
    playerIdentityField(),
    textField('buff', 'Buff'),
  ]),
  operation('player_assistance.remove_buff', 'Remove buff', GameOperationRisk.CAUTION, [
    playerIdentityField(),
    textField('buff', 'Buff'),
  ]),
  commandPermissionOperation('permissions.set_command_permission', 'Set command permission'),
  commandPermissionOperation('permissions.reset_command_permission', 'Reset command permission'),
  operation('communication.message_player', 'Message player'),
  operation('communication.broadcast_message', 'Broadcast message'),
  operation('server_control.set_game_time', 'Set game time', GameOperationRisk.CAUTION, [
    textField('time', 'World time'),
  ]),
  operation('server_control.set_temperature_unit', 'Set temperature unit'),
  operation('server_control.save_world', 'Save world'),
  operation('world_events.spawn_airdrop', 'Spawn airdrop', GameOperationRisk.CAUTION),
  operation(
    'world_events.spawn_wandering_horde',
    'Spawn wandering horde',
    GameOperationRisk.CAUTION,
  ),
  operation('world_events.set_weather', 'Set weather', GameOperationRisk.CAUTION, [
    textField('weather', 'Weather'),
  ]),
]

async function mountOperations(operations = commonOperations) {
  mocks.listGameServerOperations.mockResolvedValue(
    create(ListGameServerOperationsResponseSchema, {
      gameServerName: 'Test Server',
      operations,
    }),
  )
  const wrapper = mount(GameServerOperations, {
    global: {
      stubs: {
        'q-icon': { props: ['name'], template: '<i :data-icon="name" />' },
        'q-spinner': { template: '<span>Loading</span>' },
        'q-dialog': {
          props: ['modelValue'],
          template: '<div v-if="modelValue"><slot /></div>',
        },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<section><slot /></section>' },
        'q-card-actions': { template: '<footer><slot /></footer>' },
        'router-link': {
          props: ['to'],
          template: '<a :href="to"><slot /></a>',
        },
        OperationCatalogInput: {
          props: ['label', 'modelValue', 'options', 'placeholder', 'testId'],
          emits: ['update:modelValue'],
          template:
            '<label><span>{{ label }}</span><input :data-testid="testId" :placeholder="placeholder" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" /></label>',
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

async function selectPanel(wrapper: Awaited<ReturnType<typeof mountOperations>>, id: string) {
  await wrapper.get<HTMLButtonElement>(`[data-testid="operation-option-${id}"]`).trigger('click')
}

describe('GameServerOperations', () => {
  beforeEach(() => {
    mocks.executeGameServerOperation.mockReset()
    mocks.getSevenDaysToDieWebAPIStatus.mockReset()
    mocks.getGameServer.mockReset()
    mocks.listGameServerOperations.mockReset()
    mocks.startGameServer.mockReset()
    localStorage.clear()
    route.current = { params: { id: 'server-1' }, query: {} }
    mocks.getSevenDaysToDieWebAPIStatus.mockResolvedValue(
      create(GetSevenDaysToDieWebAPIStatusResponseSchema, {
        status: create(SevenDaysToDieWebAPIStatusSchema, {
          worldTimeState:
            SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
          worldTime: create(SevenDaysToDieGameTimeSchema, { day: 42, hour: 7, minute: 5 }),
        }),
      }),
    )
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        name: 'Test Server',
        status: Status.ONLINE,
        effectivePermissions: ['game_server.start'],
      }),
    })
    mocks.startGameServer.mockResolvedValue({})
  })

  it('presents common administration tasks through a searchable one-task workbench', async () => {
    const wrapper = await mountOperations([
      ...commonOperations,
      operation('server_control.shutdown', 'Shut down server'),
      operation('server_information.game_time', 'Inspect game time'),
      operation('server_information.version', 'Inspect game version'),
    ])

    expect(wrapper.get('h1').text()).toBe('Operations workbench')
    expect(wrapper.text()).toContain('Day 42, 07:05')
    expect(wrapper.get('[data-testid="operation-search"]').exists()).toBe(true)
    expect(wrapper.findAll('.active-task')).toHaveLength(1)
    expect(wrapper.text()).toContain('Kick player')
    expect(wrapper.text()).toContain('Ban player')
    expect(wrapper.text()).toContain('Player assistance')
    expect(wrapper.findAll('.operation-button').map((button) => button.text())).toEqual([
      'Kick player',
      'Ban player',
      'Unban player',
      'Player assistance5 actions',
    ])

    await wrapper.get('[data-testid="operation-category-access"]').trigger('click')
    expect(wrapper.find('[data-testid="add-administrator"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-administrator"]').exists()).toBe(true)
    expect(wrapper.findAll('.operation-button').map((button) => button.text())).toEqual([
      'Player access4 actions',
      'Command permissions2 actions',
    ])

    await wrapper.get('[data-testid="operation-category-world"]').trigger('click')
    expect(wrapper.text()).toContain('Spawn events')
    expect(wrapper.findAll('.operation-button').map((button) => button.text())).toEqual([
      'World controls4 actions',
      'Spawn events2 actions',
    ])
    expect(wrapper.find('[data-testid="operation-picker"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Shut down server')
    expect(wrapper.text()).not.toContain('Inspect game time')
    expect(wrapper.text()).not.toContain('Inspect game version')
    expect(wrapper.text()).not.toContain('Recent actions')
  })

  it('filters across categories and keeps only the selected task form active', async () => {
    const wrapper = await mountOperations()
    const search = wrapper.get<HTMLInputElement>('[data-testid="operation-search"]')

    await search.setValue('broadcast')
    expect(wrapper.findAll('.operation-button')).toHaveLength(1)
    expect(wrapper.get('.operation-button').text()).toContain('Messaging')
    expect(wrapper.findAll('.active-task')).toHaveLength(1)

    await selectPanel(wrapper, 'messaging')
    expect(wrapper.get('#active-operation-title').text()).toBe('Messaging')
    expect(wrapper.find('[data-testid="player-identity"]').exists()).toBe(true)
    expect(wrapper.findAll('.active-task > .control-group')).toHaveLength(1)
  })

  it('does not present stale world time as live after a refresh failure', async () => {
    const wrapper = await mountOperations()
    expect(wrapper.text()).toContain('Day 42, 07:05')

    mocks.getSevenDaysToDieWebAPIStatus.mockRejectedValueOnce(new Error('offline'))
    await wrapper.get<HTMLButtonElement>('[aria-label="Refresh world time"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load')
    expect(wrapper.text()).not.toContain('Day 42, 07:05')
  })

  it('keeps one world status refresh in flight while polling continues', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await mountOperations()
      mocks.getSevenDaysToDieWebAPIStatus.mockImplementationOnce(() => new Promise(() => {}))
      const refresh = wrapper.get<HTMLButtonElement>('[aria-label="Refresh world time"]')

      await refresh.trigger('click')
      expect(refresh.attributes('disabled')).toBeDefined()

      await vi.advanceTimersByTimeAsync(30_000)
      expect(mocks.getSevenDaysToDieWebAPIStatus).toHaveBeenCalledTimes(2)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows a repeated availability reason once beside the active task', async () => {
    const reason = 'Start the game server to make this operation available.'
    const wrapper = await mountOperations([
      addAdministrator({ available: false, availabilityReasonText: reason }),
      addAdministrator({
        id: 'player_access.remove_administrator',
        name: 'Remove administrator',
        available: false,
        availabilityReasonText: reason,
      }),
    ])

    expect(wrapper.text().match(new RegExp(reason.replace('.', '\\.'), 'g'))).toHaveLength(1)
    expect(wrapper.get('.active-task__unavailable').text()).toContain(reason)
    expect(wrapper.find('.availability-summary').exists()).toBe(false)
  })

  it('keeps the selected player identity and connection state visible', async () => {
    const wrapper = await mountOperations()
    const input = wrapper.get<HTMLInputElement>('[data-testid="player-identity"]')

    await input.setValue('Steam_PLAYER_1')
    expect(wrapper.get('[data-testid="selected-player"]').text()).toContain('Player One')
    expect(wrapper.get('[data-testid="selected-player"]').text()).toContain('Steam_PLAYER_1')
    expect(wrapper.get('[data-testid="selected-player"]').text()).toContain('Online')

    await input.setValue('Steam_MANUAL')
    expect(wrapper.get('[data-testid="selected-player"]').text()).toContain('Manual identity')
  })

  it('previews a selected server item and distinguishes manual exact names', async () => {
    const itemField = textField('item', 'Item')
    itemField.options = [
      create(GameOperationFieldOptionSchema, {
        label: 'Wood',
        value: 'resourceWood',
        description: 'resourceWood',
        iconUrl: '/api/game-server-operation-item-icons/server-1/resourceWood.png',
        category: 'Resources / Basics',
      }),
    ]
    const wrapper = await mountOperations([
      operation('player_assistance.give_item', 'Give item', GameOperationRisk.CAUTION, [
        playerIdentityField(),
        itemField,
        integerField('amount', 'Amount', '1', 1000),
      ]),
    ])
    const input = wrapper.get<HTMLInputElement>('[data-testid="item-name"]')

    await input.setValue('resourceWood')
    expect(wrapper.get('[data-testid="selected-item"]').text()).toContain('Wood')
    expect(wrapper.get('[data-testid="selected-item"]').text()).toContain('resourceWood')
    expect(wrapper.get('[data-testid="selected-item"]').text()).toContain('Resources / Basics')
    expect(wrapper.get('[data-testid="selected-item"]').text()).toContain('Server catalog')

    await input.setValue('moddedCustomItem')
    expect(wrapper.get('[data-testid="selected-item"]').text()).toContain('Manual exact name')
  })

  it('marks saved teleport destinations offline and requires a live target', async () => {
    const destination = playerIdentityField('destination', 'Destination player')
    destination.options = destination.options.filter((option) => option.value === 'Steam_PLAYER_1')
    const wrapper = await mountOperations([
      operation(
        'player_assistance.teleport_to_player',
        'Teleport player',
        GameOperationRisk.CAUTION,
        [playerIdentityField(), destination],
      ),
    ])

    const labels = wrapper
      .findAll('#teleport-player-identities option')
      .map((option) => option.attributes('label'))
    expect(labels).toEqual(['Player One', 'Player Two (offline)'])

    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper
      .get<HTMLInputElement>('[data-testid="teleport-destination"]')
      .setValue('Steam_PLAYER_2')
    expect(wrapper.text()).toContain('This saved player is offline.')
    expect(
      wrapper.get<HTMLButtonElement>('[data-testid="teleport-player"]').attributes(),
    ).toHaveProperty('disabled')

    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_2')
    await wrapper
      .get<HTMLInputElement>('[data-testid="teleport-destination"]')
      .setValue('Steam_PLAYER_1')
    expect(wrapper.text()).not.toContain('This saved player is offline.')
    expect(
      wrapper.get<HTMLButtonElement>('[data-testid="teleport-player"]').attributes(),
    ).not.toHaveProperty('disabled')
  })

  it('opens a linked player action with the requested player ready for review', async () => {
    route.current = {
      params: { id: 'server-1' },
      query: { operation: 'player_access.add_administrator', player: 'Steam_PLAYER_1' },
    }
    const wrapper = await mountOperations([addAdministrator()])

    expect(wrapper.get<HTMLInputElement>('[data-testid="player-identity"]').element.value).toBe(
      'Steam_PLAYER_1',
    )
    expect(wrapper.get('#active-operation-title').text()).toBe('Player access')
    expect(wrapper.text()).toContain('Review administrator access')
    expect(wrapper.text()).toContain('Player One (Steam_PLAYER_1)')
  })

  it('reviews a protected action and submits typed values once confirmed', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.CONFIRMED,
          message: 'Administrator access was confirmed.',
        }),
      }),
    )
    const wrapper = await mountOperations([addAdministrator()])
    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper.get<HTMLButtonElement>('[data-testid="add-administrator"]').trigger('click')

    expect(wrapper.text()).toContain('Review administrator access')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation).toHaveBeenCalledTimes(1)
    expect(mocks.executeGameServerOperation.mock.calls[0]?.[0]).toMatchObject({
      gameServerId: 'server-1',
      operationId: 'player_access.add_administrator',
      values: [
        { fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } },
        { fieldId: 'permission_level', value: { case: 'integerValue', value: 200n } },
      ],
    })
    expect(wrapper.get('[data-testid="operation-result"]').text()).toContain(
      'Administrator access was confirmed.',
    )
  })

  it('runs routine world controls directly with issued feedback', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'The server accepted the command, but completion could not be verified.',
        }),
      }),
    )
    const wrapper = await mountOperations([operation('server_control.save_world', 'Save world')])

    await wrapper.get<HTMLButtonElement>('[data-testid="save-world"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        operationId: 'server_control.save_world',
        values: [],
      }),
    )
    expect(wrapper.find('[data-testid="confirm-operation"]').exists()).toBe(false)
    const result = wrapper.get('[data-testid="operation-result"]').text()
    expect(result).toContain('Save world — Command issued')
    expect(result).toContain('The command was sent to the game server.')
    expect(result).not.toContain('verified')
  })

  it('groups spawn world events without changing their operation identities', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'World event accepted.',
        }),
      }),
    )
    const wrapper = await mountOperations([
      operation('world_events.spawn_airdrop', 'Spawn airdrop', GameOperationRisk.CAUTION),
      operation(
        'world_events.spawn_wandering_horde',
        'Spawn wandering horde',
        GameOperationRisk.CAUTION,
      ),
    ])

    expect(wrapper.get('[data-testid="spawn-airdrop"]').text()).toContain('Spawn airdrop')
    await wrapper.get<HTMLButtonElement>('[data-testid="spawn-wandering-horde"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        operationId: 'world_events.spawn_wandering_horde',
        values: [],
      }),
    )
  })

  it('groups related controls without changing their operation identities', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'Operation accepted.',
        }),
      }),
    )
    const wrapper = await mountOperations()

    await wrapper.get('[data-testid="operation-category-access"]').trigger('click')
    await selectPanel(wrapper, 'player-access')
    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    expect(wrapper.find('[data-testid="remove-administrator"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="allowlist-remove"]').exists()).toBe(true)
    await wrapper.get<HTMLButtonElement>('[data-testid="allowlist-add"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="operation-category-players"]').trigger('click')
    await selectPanel(wrapper, 'player-assistance')
    expect(wrapper.find('[data-testid="teleport-player"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-buff"]').exists()).toBe(true)
    await wrapper.get<HTMLButtonElement>('[data-testid="give-experience"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="operation-category-access"]').trigger('click')
    await selectPanel(wrapper, 'command-permissions')
    await wrapper.get<HTMLInputElement>('[data-testid="command-name"]').setValue('teleportplayer')
    expect(wrapper.find('[data-testid="set-command-permission"]').exists()).toBe(true)
    await wrapper
      .get<HTMLButtonElement>('[data-testid="reset-command-permission"]')
      .trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="operation-category-messages"]').trigger('click')
    await selectPanel(wrapper, 'messaging')
    expect(wrapper.find('[data-testid="player-identity"]').exists()).toBe(true)
    await wrapper.get<HTMLTextAreaElement>('[data-testid="operation-message"]').setValue('Hello')
    expect(wrapper.find('[data-testid="broadcast-message"]').exists()).toBe(true)
    await wrapper.get<HTMLButtonElement>('[data-testid="message-player"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="operation-category-world"]').trigger('click')
    await selectPanel(wrapper, 'world-controls')
    expect(wrapper.find('[data-testid="set-exact-world-time"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="set-temperature-unit"]').exists()).toBe(true)
    await wrapper.get<HTMLButtonElement>('[data-testid="save-world"]').trigger('click')
    await flushPromises()

    expect(
      mocks.executeGameServerOperation.mock.calls.map(([request]) => request.operationId),
    ).toEqual([
      'player_access.allowlist_add',
      'player_assistance.give_experience',
      'permissions.reset_command_permission',
      'communication.message_player',
      'server_control.save_world',
    ])
  })

  it('submits bounded player-assistance values from the selected player', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'Item grant accepted.',
        }),
      }),
    )
    const giveItem = operation(
      'player_assistance.give_item',
      'Give item',
      GameOperationRisk.CAUTION,
      [
        playerIdentityField(),
        textField('item', 'Item'),
        integerField('amount', 'Amount', '1', 1000),
      ],
    )
    const wrapper = await mountOperations([giveItem])
    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper.get<HTMLInputElement>('[data-testid="item-name"]').setValue('resourceWood')
    await wrapper.get<HTMLInputElement>('[data-testid="item-amount"]').setValue('50')
    await wrapper.get<HTMLButtonElement>('[data-testid="give-item"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation.mock.calls[0]?.[0]).toMatchObject({
      operationId: 'player_assistance.give_item',
      values: [
        { fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } },
        { fieldId: 'item', value: { case: 'stringValue', value: 'resourceWood' } },
        { fieldId: 'amount', value: { case: 'integerValue', value: 50n } },
      ],
    })
  })

  it('submits bounded weather and exact-time controls', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'World change accepted.',
        }),
      }),
    )
    const setWeather = operation(
      'world_events.set_weather',
      'Set weather',
      GameOperationRisk.CAUTION,
      [textField('weather', 'Weather')],
    )
    const setTime = operation(
      'server_control.set_game_time',
      'Set game time',
      GameOperationRisk.CAUTION,
      [textField('time', 'World time')],
    )
    const wrapper = await mountOperations([setWeather, setTime])

    await wrapper.get<HTMLSelectElement>('[data-testid="weather-preset"]').setValue('rain')
    await wrapper.get<HTMLButtonElement>('[data-testid="set-weather"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    await selectPanel(wrapper, 'world-controls')
    await wrapper.get<HTMLInputElement>('[data-testid="exact-world-day"]').setValue('42')
    await wrapper.get<HTMLInputElement>('[data-testid="exact-world-hour"]').setValue('7')
    await wrapper.get<HTMLInputElement>('[data-testid="exact-world-minute"]').setValue('5')
    await wrapper.get<HTMLButtonElement>('[data-testid="set-exact-world-time"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation.mock.calls.map((call) => call[0])).toMatchObject([
      {
        operationId: 'world_events.set_weather',
        values: [{ fieldId: 'weather', value: { case: 'stringValue', value: 'rain' } }],
      },
      {
        operationId: 'server_control.set_game_time',
        values: [{ fieldId: 'time', value: { case: 'stringValue', value: '42 7 5' } }],
      },
    ])
  })

  it('keeps player selection available to private-message-only users', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'Message accepted.',
        }),
      }),
    )
    const wrapper = await mountOperations([messagePlayerOperation()])

    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper.get<HTMLTextAreaElement>('textarea').setValue('Hello')
    await wrapper.get<HTMLButtonElement>('[data-testid="message-player"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation.mock.calls[0]?.[0]).toMatchObject({
      operationId: 'communication.message_player',
      values: [
        { fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } },
        { fieldId: 'message', value: { case: 'stringValue', value: 'Hello' } },
      ],
    })
  })

  it('submits typed values for command permission changes', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.CONFIRMED,
          message: 'Permission updated.',
        }),
      }),
    )
    const wrapper = await mountOperations([
      commandPermissionOperation('permissions.set_command_permission', 'Set command permission'),
      commandPermissionOperation(
        'permissions.reset_command_permission',
        'Reset command permission',
      ),
    ])
    await wrapper.get<HTMLInputElement>('[data-testid="command-name"]').setValue('teleport')
    await wrapper.get('summary').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-testid="command-permission-level"]').setValue('42')

    await wrapper.get<HTMLButtonElement>('[data-testid="set-command-permission"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()
    await selectPanel(wrapper, 'command-permissions')
    await wrapper
      .get<HTMLButtonElement>('[data-testid="reset-command-permission"]')
      .trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation.mock.calls.map((call) => call[0])).toMatchObject([
      {
        operationId: 'permissions.set_command_permission',
        values: [
          { fieldId: 'command', value: { case: 'stringValue', value: 'teleport' } },
          { fieldId: 'permission_level', value: { case: 'integerValue', value: 42n } },
        ],
      },
      {
        operationId: 'permissions.reset_command_permission',
        values: [{ fieldId: 'command', value: { case: 'stringValue', value: 'teleport' } }],
      },
    ])
  })

  it('submits typed values for ban and unban actions', async () => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED,
          message: 'Moderation request accepted.',
        }),
      }),
    )
    const wrapper = await mountOperations([
      moderationOperation('player_moderation.ban', 'Ban player', GameOperationRisk.CAUTION),
      moderationOperation('player_moderation.unban', 'Unban player', GameOperationRisk.ROUTINE),
    ])
    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper.get<HTMLInputElement>('input[placeholder="Optional"]').setValue('Griefing')

    await wrapper.get<HTMLButtonElement>('[data-testid="ban-player"]').trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()
    await selectPanel(wrapper, 'player_moderation.unban')
    await wrapper.get<HTMLButtonElement>('[data-testid="unban-player"]').trigger('click')
    await flushPromises()

    expect(mocks.executeGameServerOperation.mock.calls.map((call) => call[0])).toMatchObject([
      {
        operationId: 'player_moderation.ban',
        values: [
          { fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } },
          { fieldId: 'reason', value: { case: 'stringValue', value: 'Griefing' } },
        ],
      },
      {
        operationId: 'player_moderation.unban',
        values: [{ fieldId: 'player', value: { case: 'stringValue', value: 'Steam_PLAYER_1' } }],
      },
    ])
  })

  it.each([1001, -1])(
    'validates advanced administrator level %s against descriptor bounds',
    async (level) => {
      const wrapper = await mountOperations([addAdministrator()])
      await wrapper
        .get<HTMLInputElement>('[data-testid="player-identity"]')
        .setValue('Steam_PLAYER_1')
      await wrapper.get('summary').trigger('click')
      await wrapper
        .get<HTMLInputElement>('[data-testid="administrator-permission-level"]')
        .setValue(String(level))
      await wrapper.get<HTMLButtonElement>('[data-testid="add-administrator"]').trigger('click')

      expect(wrapper.find('[data-testid="confirm-operation"]').exists()).toBe(false)
      expect(wrapper.get('[role="alert"]').text()).toContain('Permission level must be')
    },
  )

  it.each([
    {
      name: 'administrator access',
      operation: addAdministrator(),
      inputTestID: 'player-identity',
      inputValue: 'Steam_PLAYER_1',
      actionTestID: 'add-administrator',
    },
    {
      name: 'command access',
      operation: commandPermissionOperation(
        'permissions.set_command_permission',
        'Set command permission',
      ),
      inputTestID: 'command-name',
      inputValue: 'teleport',
      actionTestID: 'set-command-permission',
    },
  ])('submits the named full-administrator value for $name', async (testCase) => {
    mocks.executeGameServerOperation.mockResolvedValue(
      create(ExecuteGameServerOperationResponseSchema, {
        result: create(GameOperationResultSchema, {
          classification: GameOperationResultClassification.CONFIRMED,
          message: 'Permission updated.',
        }),
      }),
    )
    const wrapper = await mountOperations([testCase.operation])
    await wrapper
      .get<HTMLInputElement>(`[data-testid="${testCase.inputTestID}"]`)
      .setValue(testCase.inputValue)
    const fullAdministrator = wrapper
      .findAll<HTMLInputElement>('input[type="radio"]')
      .find((radio) => radio.element.value === 'administrator')
    expect(fullAdministrator).toBeDefined()
    await fullAdministrator?.setValue()
    await wrapper
      .get<HTMLButtonElement>(`[data-testid="${testCase.actionTestID}"]`)
      .trigger('click')
    await wrapper.get<HTMLButtonElement>('[data-testid="confirm-operation"]').trigger('click')
    await flushPromises()

    expect(
      mocks.executeGameServerOperation.mock.calls[0]?.[0].values.find(
        (value: { fieldId: string }) => value.fieldId === 'permission_level',
      ),
    ).toMatchObject({
      fieldId: 'permission_level',
      value: { case: 'integerValue', value: 0n },
    })
  })

  it('shows authoritative stopped recovery and keeps start progress local', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        name: 'Test Server',
        status: Status.OFFLINE,
        effectivePermissions: ['game_server.start'],
      }),
    })
    let resolveStart: (() => void) | undefined
    mocks.startGameServer.mockImplementation(
      () => new Promise<void>((resolve) => (resolveStart = resolve)),
    )
    const wrapper = await mountOperations()

    expect(wrapper.find('[data-testid="operations-workbench"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Start Test Server to run operations')
    const start = wrapper.get<HTMLButtonElement>('[data-testid="start-server"]')
    await start.trigger('click')
    expect(start.text()).toBe('Starting…')
    expect(start.attributes('disabled')).toBeDefined()

    resolveStart?.()
    await flushPromises()
    expect(start.text()).toBe('Start requested')
    expect(wrapper.text()).toContain('Open Overview to follow the lifecycle state.')
  })

  it.each([
    { permissions: ['game_server.start'], expected: 'The game server could not be started' },
    { permissions: [], expected: 'Starting this game server requires start permission.' },
  ])('provides a stopped-state recovery fallback: $expected', async (testCase) => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        name: 'Test Server',
        status: Status.OFFLINE,
        effectivePermissions: testCase.permissions,
      }),
    })
    mocks.startGameServer.mockRejectedValue(new Error('offline'))
    const wrapper = await mountOperations()

    const start = wrapper.find<HTMLButtonElement>('[data-testid="start-server"]')
    if (testCase.permissions.length > 0) {
      await start.trigger('click')
      await flushPromises()
    }
    expect(wrapper.text()).toContain(testCase.expected)
    expect(wrapper.text()).toContain('Open Overview')
  })

  it('persists only non-sensitive workbench preferences per server', async () => {
    const giveItem = operation(
      'player_assistance.give_item',
      'Give item',
      GameOperationRisk.CAUTION,
      [
        playerIdentityField(),
        textField('item', 'Item'),
        integerField('amount', 'Amount', '1', 1000),
      ],
    )
    const wrapper = await mountOperations([giveItem])
    await wrapper
      .get<HTMLInputElement>('[data-testid="player-identity"]')
      .setValue('Steam_PLAYER_1')
    await wrapper.get<HTMLInputElement>('[data-testid="item-name"]').setValue('resourceWood')
    await wrapper.get<HTMLInputElement>('[data-testid="item-amount"]').setValue('25')
    await flushPromises()

    const key = 'xylona:game-server:server-1:operations-workbench'
    const stored = localStorage.getItem(key) ?? ''
    expect(stored).toContain('"itemAmount":25')
    expect(stored).not.toContain('Steam_PLAYER_1')
    expect(stored).not.toContain('resourceWood')

    wrapper.unmount()
    const restored = await mountOperations([giveItem])
    expect(restored.get<HTMLInputElement>('[data-testid="item-amount"]').element.value).toBe('25')
    expect(restored.get<HTMLInputElement>('[data-testid="item-name"]').element.value).toBe('')
    expect(restored.get<HTMLInputElement>('[data-testid="player-identity"]').element.value).toBe('')
  })

  it('keeps unavailable actions disabled and explains the server state', async () => {
    const unavailable = operation('player_moderation.ban', 'Ban player', GameOperationRisk.CAUTION)
    unavailable.available = false
    unavailable.availabilityReasonText = 'Start the server to manage players.'
    const wrapper = await mountOperations([unavailable])

    expect(
      wrapper.get<HTMLButtonElement>('[data-testid="ban-player"]').attributes('disabled'),
    ).toBeDefined()
    expect(wrapper.text()).toContain('Start the server to manage players.')
  })
})
