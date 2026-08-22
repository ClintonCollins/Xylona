import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GameServerPlayerRoster from './GameServerPlayerRoster.vue'

const mocks = vi.hoisted(() => ({
  getManagement: vi.fn(),
  performAction: vi.fn(),
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

describe('GameServerPlayerRoster', () => {
  beforeEach(() => {
    mocks.getManagement.mockReset().mockResolvedValue({
      status: 1,
      capabilities: { actionsSupported: true, supportedActions: [1] },
      players: [{ name: 'Duplicated name', actionIdentifier: 'Steam_1' }],
    })
  })

  it('does not associate a 7DTD query display name with a moderation target', async () => {
    const wrapper = mount(GameServerPlayerRoster, {
      props: {
        gameServerId: 'server-1',
        isOnline: true,
        playerNames: ['Duplicated name'],
        currentPlayerCount: 1,
        maxPlayerCount: 8,
        playerListSupported: true,
        unlistedPlayerCount: 0,
        canManagePlayers: true,
        nativeIdentifiersRequired: true,
      },
      global: {
        stubs: {
          'q-icon': true,
          'q-btn': { props: ['label'], template: '<button>{{ label }}</button>' },
          'q-dialog': true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Duplicated name')
    expect(wrapper.text()).not.toContain('Kick')
    expect(mocks.getManagement).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
