import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { QInput, QSelect, Quasar } from 'quasar'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AllServersQueryInfoSchema, ServerQuery_Type, Status } from '@/proto/shared_pb'
import {
  GameServerManagementPlayerSchema,
  type GameServerManagementPlayer,
  GameServerPlayerAction,
  GameServerPlayerManagementPlayersState,
  GetGameServerPlayerManagementResponseSchema,
} from '@/proto/xylona_pb'
import PlayerManagementPanel from './PlayerManagementPanel.vue'

const mocks = vi.hoisted(() => ({
  getManagement: vi.fn(),
  performAction: vi.fn(),
  on: vi.fn(),
  off: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerPlayerManagement: mocks.getManagement,
    performGameServerPlayerAction: mocks.performAction,
  }),
  XylonaEventBus: { on: mocks.on, off: mocks.off },
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: vi.fn(),
  notifySuccess: vi.fn(),
}))

const stubs = {
  'q-banner': {
    template: '<div class="q-banner-stub"><slot name="avatar"/><slot/><slot name="action"/></div>',
  },
  'q-btn': {
    props: ['label', 'disable', 'loading'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}<slot/></button>',
  },
  'q-card': { template: '<section><slot/></section>' },
  'q-card-section': { template: '<div><slot/></div>' },
  'q-card-actions': { template: '<div><slot/></div>' },
  'q-chip': { props: ['label'], template: '<span>{{ label }}</span>' },
  'q-dialog': { props: ['modelValue'], template: '<div v-if="modelValue"><slot/></div>' },
  'q-icon': { template: '<i />' },
  'q-avatar': { template: '<span><slot/></span>' },
  'q-input': {
    props: ['modelValue', 'disable', 'label'],
    emits: ['update:modelValue'],
    template:
      '<label>{{ label }}<input :disabled="disable" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"/></label>',
  },
  'q-select': {
    props: ['disable', 'label', 'modelValue', 'options'],
    emits: ['update:modelValue'],
    template: `<label>{{ label }}<select :disabled="disable" :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value ?? option.action" :value="option.value">{{ option.label }}</option>
    </select></label>`,
  },
  'q-list': { template: '<div><slot/></div>' },
  'q-item': { template: '<div class="player-row"><slot/></div>' },
  'q-item-section': { template: '<div><slot/></div>' },
  'q-item-label': { template: '<div><slot/></div>' },
  'q-separator': { template: '<hr/>' },
  'q-skeleton': { template: '<div>Loading players</div>' },
  'q-tooltip': { template: '<span><slot/></span>' },
}

function response(
  playersState: GameServerPlayerManagementPlayersState,
  options: {
    players?: GameServerManagementPlayer[]
    status?: Status
    actionsSupported?: boolean
    unavailableReason?: string
  } = {},
) {
  return create(GetGameServerPlayerManagementResponseSchema, {
    capabilities: {
      actionsSupported: options.actionsSupported ?? true,
      unavailableReason: options.unavailableReason,
      identifierLabel: 'Platform, cross-platform, or entity ID',
      supportedActions: [GameServerPlayerAction.KICK],
      playersState,
    },
    managementPlayers: options.players ?? [],
    status: options.status ?? Status.ONLINE,
  })
}

async function mountPanel(realFilters = false) {
  const wrapper = mount(PlayerManagementPanel, {
    props: { gameServerId: 'server-1' },
    global: {
      plugins: realFilters ? [Quasar] : [],
      stubs: {
        ...stubs,
        'q-input': realFilters ? QInput : stubs['q-input'],
        'q-select': realFilters ? QSelect : stubs['q-select'],
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('PlayerManagementPanel', () => {
  beforeEach(() => {
    mocks.getManagement.mockReset()
    mocks.performAction.mockReset().mockResolvedValue({})
    mocks.on.mockReset()
    mocks.off.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it.each([
    [
      GameServerPlayerManagementPlayersState.UNSUPPORTED,
      'This server does not report its online players',
    ],
    [
      GameServerPlayerManagementPlayersState.PERMISSION_DENIED,
      'The game server denied access to its player list',
    ],
    [GameServerPlayerManagementPlayersState.UNAVAILABLE, 'The player list is unavailable'],
  ])(
    'renders player list state %s while leaving manual actions enabled',
    async (state, expected) => {
      mocks.getManagement.mockResolvedValue(response(state))
      const wrapper = await mountPanel()

      expect(wrapper.text()).toContain(expected)
      expect(wrapper.text()).toContain('Manage by platform, cross-platform, or entity id')
      expect(wrapper.get('select').attributes('disabled')).toBeUndefined()
      expect(wrapper.get('input').attributes('disabled')).toBeUndefined()
      wrapper.unmount()
    },
  )

  it('distinguishes offline and a confirmed empty player list', async () => {
    mocks.getManagement.mockResolvedValueOnce(
      response(GameServerPlayerManagementPlayersState.AVAILABLE, { status: Status.OFFLINE }),
    )
    const offline = await mountPanel()
    expect(offline.text()).toContain('Players appear while the server is online')
    expect(offline.text()).not.toContain('No players online')
    offline.unmount()

    mocks.getManagement.mockResolvedValueOnce(
      response(GameServerPlayerManagementPlayersState.AVAILABLE),
    )
    const empty = await mountPanel()
    expect(empty.text()).toContain('No players reported')
    expect(empty.text()).toContain('The game server returned an empty player list')
    empty.unmount()
  })

  it('renders supplied zero values, escapes names, and targets the explicit action identifier', async () => {
    const player = create(GameServerManagementPlayerSchema, {
      name: '<img src=x onerror=alert(1)>',
      actionIdentifier: 'Steam_1',
      entityId: '7',
      platformId: 'Steam_1',
      crossPlatformId: 'EOS_1',
      online: false,
      ping: 0,
      level: 0,
      health: 0,
      stamina: 0,
      score: 0,
      deaths: 0,
      zombieKills: 0,
      playerKills: 0,
      banned: false,
    })
    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementPlayersState.AVAILABLE, { players: [player] }),
    )
    const wrapper = await mountPanel()

    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.html()).not.toContain('<img src="x"')
    expect(wrapper.text()).toContain('Action ID: Steam_1')
    for (const metric of [
      'Offline',
      'Ping 0 ms',
      'Level 0',
      'Health 0',
      'Stamina 0',
      'Score 0',
      'Deaths 0',
      'Zombie kills 0',
      'Player kills 0',
      'Banned No',
    ]) {
      expect(wrapper.text()).toContain(metric)
    }

    const kickButtons = wrapper.findAll('button').filter((button) => button.text().includes('Kick'))
    await kickButtons[0]?.trigger('click')
    await flushPromises()
    const confirmButtons = wrapper.findAll('button').filter((button) => button.text() === 'Kick')
    await confirmButtons.at(-1)?.trigger('click')
    await flushPromises()
    expect(mocks.performAction).toHaveBeenCalledWith(
      expect.objectContaining({ playerId: 'Steam_1' }),
    )
    wrapper.unmount()
  })

  it('preserves non-native player wording and identifier presentation', async () => {
    const player = create(GameServerManagementPlayerSchema, {
      name: 'Alex',
      actionIdentifier: 'Alex',
    })
    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementPlayersState.UNSPECIFIED, { players: [player] }),
    )
    const minecraft = await mountPanel()

    expect(minecraft.text()).toContain('Alex')
    expect(minecraft.text()).not.toContain('does not report its online players')
    expect(minecraft.text()).not.toContain('Action ID: Alex')
    minecraft.unmount()

    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementPlayersState.UNSPECIFIED, {
        actionsSupported: false,
        unavailableReason: 'Palworld REST API credentials are not configured for this server.',
      }),
    )
    const palworld = await mountPanel()

    expect(palworld.text()).toContain('Palworld REST API credentials are not configured')
    expect(palworld.text()).toContain('The server query returned no players')
    expect(palworld.text()).not.toContain('reports no connected players')
    palworld.unmount()
  })

  it('ignores retained offline native players when comparing live query events', async () => {
    vi.useFakeTimers()
    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementPlayersState.AVAILABLE, {
        players: [
          create(GameServerManagementPlayerSchema, { name: 'Alex', online: true }),
          create(GameServerManagementPlayerSchema, { name: 'Jamie', online: false }),
        ],
      }),
    )
    const wrapper = await mountPanel()
    const queryInfoHandler = mocks.on.mock.calls.find(
      ([eventName]) => eventName === 'gameServersQueryInfo',
    )?.[1]
    expect(queryInfoHandler).toBeTypeOf('function')

    const queryInfoEvent = (playerList: string[]) =>
      create(AllServersQueryInfoSchema, {
        servers: {
          'server-1': {
            serverId: 'server-1',
            type: ServerQuery_Type.Source,
            source: {
              playerList,
              playerListSupported: true,
              players: playerList.length,
            },
          },
        },
      })

    queryInfoHandler?.(queryInfoEvent(['Alex']))
    await vi.advanceTimersByTimeAsync(1500)
    expect(mocks.getManagement).toHaveBeenCalledTimes(1)

    queryInfoHandler?.(queryInfoEvent(['Alex', 'Morgan']))
    await vi.advanceTimersByTimeAsync(1500)
    expect(mocks.getManagement).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  describe('finding reported players', () => {
    const available = GameServerPlayerManagementPlayersState.AVAILABLE
    const players = [
      create(GameServerManagementPlayerSchema, {
        name: 'Alex',
        actionIdentifier: 'Steam_123',
        online: true,
      }),
      create(GameServerManagementPlayerSchema, {
        name: 'Alex',
        actionIdentifier: 'Steam_456',
        online: false,
      }),
      create(GameServerManagementPlayerSchema, {
        name: 'Morgan',
        actionIdentifier: 'EOS_789',
      }),
    ]
    const searchSelector = '[aria-label="Search reported players"] input'
    const presenceSelector = '[aria-label="Filter players by reported presence"] select'

    beforeEach(() => {
      mocks.getManagement.mockResolvedValue(response(available, { players }))
    })

    it('searches names and action identifiers without changing action targets', async () => {
      const wrapper = await mountPanel()
      expect(wrapper.text()).toContain('Reported players')
      expect(wrapper.text()).toContain('Showing 3 of 3 reported players')

      await wrapper.get(searchSelector).setValue('  aLeX  ')
      expect(wrapper.findAll('.player-row')).toHaveLength(2)
      expect(wrapper.text()).toContain('Showing 2 of 3 reported players')

      await wrapper.get(searchSelector).setValue('steam_456')
      expect(wrapper.findAll('.player-row')).toHaveLength(1)
      expect(wrapper.get('.player-row').text()).toContain('Action ID: Steam_456')
      await wrapper.get('.player-row button').trigger('click')
      const confirm = wrapper.findAll('button').findLast((button) => button.text() === 'Kick')
      await confirm?.trigger('click')
      await flushPromises()
      expect(mocks.performAction).toHaveBeenCalledWith(
        expect.objectContaining({ playerId: 'Steam_456' }),
      )
      expect(wrapper.get(searchSelector).element).toHaveProperty('value', 'steam_456')
      wrapper.unmount()
    })

    it('combines search with presence and keeps missing presence distinct from offline', async () => {
      const wrapper = await mountPanel()
      const presence = wrapper.get(presenceSelector)
      expect(presence.text()).toContain('Online (1)')
      expect(presence.text()).toContain('Offline (1)')
      expect(presence.text()).toContain('Unknown (1)')

      await presence.setValue('online')
      expect(wrapper.get('.player-row').text()).toContain('Steam_123')
      await presence.setValue('offline')
      expect(wrapper.findAll('.player-row')).toHaveLength(1)
      expect(wrapper.get('.player-row').text()).toContain('Steam_456')
      await presence.setValue('unknown')
      expect(wrapper.findAll('.player-row')).toHaveLength(1)
      expect(wrapper.get('.player-row').text()).toContain('EOS_789')

      await wrapper.get(searchSelector).setValue('Alex')
      expect(wrapper.findAll('.player-row')).toHaveLength(0)
      expect(wrapper.text()).toContain('No players match your filters')
      expect(wrapper.text()).not.toContain('No players reported')
      const clear = wrapper.findAll('button').find((button) => button.text() === 'Clear filters')
      await clear?.trigger('click')
      expect(wrapper.findAll('.player-row')).toHaveLength(3)
      expect(wrapper.get(searchSelector).element).toHaveProperty('value', '')
      expect(presence.element).toHaveProperty('value', 'all')
      expect(mocks.getManagement).toHaveBeenCalledTimes(1)
      expect(mocks.performAction).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('treats a cleared or whitespace search as no search', async () => {
      const wrapper = await mountPanel()
      await wrapper.get(searchSelector).setValue('Morgan')
      wrapper
        .getComponent('[aria-label="Search reported players"]')
        .vm.$emit('update:modelValue', null)
      await flushPromises()
      expect(wrapper.findAll('.player-row')).toHaveLength(3)
      await wrapper.get(searchSelector).setValue('   ')
      expect(wrapper.findAll('.player-row')).toHaveLength(3)
      expect(wrapper.text()).not.toContain('Clear filters')
      wrapper.unmount()
    })

    it('retains filters through refreshes and updates counts when a player leaves', async () => {
      const wrapper = await mountPanel()
      await wrapper.get(searchSelector).setValue('Alex')
      await wrapper.get(presenceSelector).setValue('online')
      mocks.getManagement.mockResolvedValueOnce(response(available, { players: players.slice(1) }))
      await wrapper.get('[aria-label="Refresh player management"]').trigger('click')
      await flushPromises()

      expect(wrapper.get(searchSelector).element).toHaveProperty('value', 'Alex')
      expect(wrapper.get(presenceSelector).element).toHaveProperty('value', 'online')
      expect(wrapper.text()).toContain('Showing 0 of 2 reported players')
      expect(wrapper.text()).toContain('No players match your filters')

      mocks.getManagement.mockResolvedValueOnce(response(available))
      await wrapper.get('[aria-label="Refresh player management"]').trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('No players reported')
      expect(wrapper.text()).not.toContain('No players match your filters')
      expect(wrapper.text()).toContain('Clear filters')
      wrapper.unmount()
    })

    it('searches legacy query players without inventing presence information', async () => {
      mocks.getManagement.mockResolvedValue(
        response(GameServerPlayerManagementPlayersState.UNSPECIFIED, {
          players: [
            create(GameServerManagementPlayerSchema, { name: 'Alex', actionIdentifier: 'Alex' }),
          ],
        }),
      )
      const wrapper = await mountPanel()
      expect(wrapper.text()).toContain('Online players')
      expect(wrapper.find(presenceSelector).exists()).toBe(false)
      await wrapper.get(searchSelector).setValue('alex')
      expect(wrapper.findAll('.player-row')).toHaveLength(1)
      expect(wrapper.text()).not.toContain('Action ID: Alex')
      wrapper.unmount()
    })

    it('resets filters when switching servers or reopening player management', async () => {
      const wrapper = await mountPanel()
      await wrapper.get(searchSelector).setValue('Alex')
      await wrapper.get(presenceSelector).setValue('offline')
      await wrapper.setProps({ gameServerId: 'server-2' })
      await flushPromises()
      expect(wrapper.get(searchSelector).element).toHaveProperty('value', '')
      expect(wrapper.get(presenceSelector).element).toHaveProperty('value', 'all')
      await wrapper.get(searchSelector).setValue('Morgan')
      wrapper.unmount()

      const reopened = await mountPanel()
      expect(reopened.get(searchSelector).element).toHaveProperty('value', '')
      expect(reopened.findAll('.player-row')).toHaveLength(3)
      reopened.unmount()
    })

    it('supports typing, keyboard presence selection, and clearing with real Quasar controls', async () => {
      const wrapper = await mountPanel(true)
      const search = wrapper.get('input[aria-label="Search reported players"]')
      await search.setValue('Alex')
      expect(wrapper.findAll('.player-row')).toHaveLength(2)

      const presence = wrapper.get(
        '[role="combobox"][aria-label="Filter players by reported presence"]',
      )
      await presence.trigger('keydown', { key: 'o', keyCode: 79 })
      await flushPromises()
      await presence.trigger('keydown', { key: 'Enter', keyCode: 13 })
      await flushPromises()
      expect(wrapper.findAll('.player-row')).toHaveLength(1)
      expect(wrapper.get('.player-row').text()).toContain('Steam_123')

      const clear = wrapper.findAll('button').find((button) => button.text() === 'Clear filters')
      await clear?.trigger('click')
      expect(search.element).toHaveProperty('value', '')
      expect(wrapper.findAll('.player-row')).toHaveLength(3)
      expect(mocks.getManagement).toHaveBeenCalledTimes(1)
      wrapper.unmount()
    })
  })
})
