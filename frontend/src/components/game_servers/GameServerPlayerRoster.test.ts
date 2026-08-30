import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Status } from '@/proto/shared_pb'
import {
  GameServerPlayerAction,
  GetGameServerPlayerManagementResponseSchema,
} from '@/proto/xylona_pb'
import GameServerPlayerRoster from './GameServerPlayerRoster.vue'

const mocks = vi.hoisted(() => ({
  getManagement: vi.fn(),
  performAction: vi.fn(),
  push: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerPlayerManagement: mocks.getManagement,
    performGameServerPlayerAction: mocks.performAction,
  }),
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: vi.fn(),
  notifySuccess: vi.fn(),
}))

async function mountRoster(nativeIdentifiersRequired: boolean, playerNames = ['Player One']) {
  const wrapper = mount(GameServerPlayerRoster, {
    props: {
      gameServerId: 'server-1',
      isOnline: true,
      playerNames,
      currentPlayerCount: playerNames.length,
      maxPlayerCount: 8,
      playerListSupported: true,
      unlistedPlayerCount: 0,
      canManagePlayers: true,
      nativeIdentifiersRequired,
    },
    global: {
      stubs: {
        'q-icon': true,
        'q-btn': {
          emits: ['click'],
          props: ['label'],
          template: '<button @click="$emit(\'click\')">{{ label }}</button>',
        },
        'q-dialog': true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('GameServerPlayerRoster', () => {
  beforeEach(() => {
    mocks.getManagement.mockReset().mockResolvedValue(
      create(GetGameServerPlayerManagementResponseSchema, {
        status: Status.ONLINE,
        capabilities: {
          actionsSupported: true,
          supportedActions: [GameServerPlayerAction.KICK],
        },
        players: [{ name: 'Player One', id: 'Player_1' }],
        managementPlayers: [
          { name: 'Player One', actionIdentifier: 'Steam_PLAYER_1', online: true },
        ],
      }),
    )
    mocks.performAction.mockReset()
    mocks.push.mockReset()
  })

  it('deep-links a 7DTD quick action with its authoritative Player identity', async () => {
    const wrapper = await mountRoster(true)

    expect(mocks.getManagement).toHaveBeenCalledOnce()
    await wrapper.get('[aria-label="Kick Player One"]').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith({
      path: '/game-servers/server-1/operations',
      query: { operation: 'player_moderation.kick', player: 'Steam_PLAYER_1' },
    })
    expect(mocks.performAction).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not guess between duplicate 7DTD display names', async () => {
    mocks.getManagement.mockResolvedValue(
      create(GetGameServerPlayerManagementResponseSchema, {
        status: Status.ONLINE,
        capabilities: {
          actionsSupported: true,
          supportedActions: [GameServerPlayerAction.KICK],
        },
        managementPlayers: [
          { name: 'Duplicated name', actionIdentifier: 'Steam_PLAYER_1', online: true },
          { name: 'Duplicated name', actionIdentifier: 'Steam_PLAYER_2', online: true },
        ],
      }),
    )
    const wrapper = await mountRoster(true, ['Duplicated name', 'Duplicated name'])

    expect(wrapper.find('[aria-label="Kick Duplicated name"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps non-7DTD console quick actions on the compatible player field', async () => {
    const wrapper = await mountRoster(false)
    await wrapper.vm.$nextTick()

    expect(mocks.getManagement).toHaveBeenCalledOnce()
    expect(wrapper.html()).toContain('roster__actions')
    expect(wrapper.find('[aria-label="Kick Player One"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
