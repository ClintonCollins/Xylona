import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Status } from '@/proto/shared_pb'
import {
  GameServerManagementPlayerSchema,
  type GameServerManagementPlayer,
  GameServerPlayerAction,
  GameServerPlayerManagementRosterState,
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
    props: ['disable', 'label'],
    template: '<label>{{ label }}<select :disabled="disable"/></label>',
  },
  'q-list': { template: '<div><slot/></div>' },
  'q-item': { template: '<div class="player-row"><slot/></div>' },
  'q-item-section': { template: '<div><slot/></div>' },
  'q-item-label': { template: '<div><slot/></div>' },
  'q-separator': { template: '<hr/>' },
  'q-skeleton': { template: '<div>Loading player roster</div>' },
  'q-tooltip': { template: '<span><slot/></span>' },
}

function response(
  rosterState: GameServerPlayerManagementRosterState,
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
      rosterState,
    },
    managementPlayers: options.players ?? [],
    status: options.status ?? Status.ONLINE,
  })
}

async function mountPanel() {
  const wrapper = mount(PlayerManagementPanel, {
    props: { gameServerId: 'server-1' },
    global: { stubs },
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

  it.each([
    [GameServerPlayerManagementRosterState.UNSUPPORTED, 'Native player roster is not supported'],
    [
      GameServerPlayerManagementRosterState.PERMISSION_DENIED,
      'Native player roster access was denied',
    ],
    [GameServerPlayerManagementRosterState.UNAVAILABLE, 'Native player roster is unavailable'],
  ])('renders roster state %s while leaving manual actions enabled', async (state, expected) => {
    mocks.getManagement.mockResolvedValue(response(state))
    const wrapper = await mountPanel()

    expect(wrapper.text()).toContain(expected)
    expect(wrapper.text()).toContain('Manage by platform, cross-platform, or entity id')
    expect(wrapper.get('select').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('input').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('distinguishes offline and a confirmed empty roster', async () => {
    mocks.getManagement.mockResolvedValueOnce(
      response(GameServerPlayerManagementRosterState.UNAVAILABLE, { status: Status.OFFLINE }),
    )
    const offline = await mountPanel()
    expect(offline.text()).toContain('Start the game server to query its roster')
    offline.unmount()

    mocks.getManagement.mockResolvedValueOnce(
      response(GameServerPlayerManagementRosterState.AVAILABLE),
    )
    const empty = await mountPanel()
    expect(empty.text()).toContain('No players reported')
    expect(empty.text()).toContain('The native player roster is currently empty')
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
      response(GameServerPlayerManagementRosterState.AVAILABLE, { players: [player] }),
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

  it('preserves non-native roster wording and identifier presentation', async () => {
    const player = create(GameServerManagementPlayerSchema, {
      name: 'Alex',
      actionIdentifier: 'Alex',
    })
    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementRosterState.UNSPECIFIED, { players: [player] }),
    )
    const minecraft = await mountPanel()

    expect(minecraft.text()).toContain('Alex')
    expect(minecraft.text()).not.toContain('Native player roster')
    expect(minecraft.text()).not.toContain('Action ID: Alex')
    minecraft.unmount()

    mocks.getManagement.mockResolvedValue(
      response(GameServerPlayerManagementRosterState.UNSPECIFIED, {
        actionsSupported: false,
        unavailableReason: 'Palworld REST API credentials are not configured for this server.',
      }),
    )
    const palworld = await mountPanel()

    expect(palworld.text()).toContain('Palworld REST API credentials are not configured')
    expect(palworld.text()).toContain('The server query returned an empty roster')
    expect(palworld.text()).not.toContain('Native player roster')
    palworld.unmount()
  })
})
