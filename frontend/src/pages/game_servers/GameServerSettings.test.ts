import { flushPromises, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GameServerSettingsForm from '@/components/game_servers/GameServerSettingsForm.vue'
import GameServerSettings from './GameServerSettings.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/stores/xylona', () => ({
  useUserAuthStore: () => ({
    user: { superUser: true },
    checkUserAuthenticated: vi.fn(),
  }),
}))

describe('GameServerSettings', () => {
  it('renders the settings form for the route server', async () => {
    const wrapper = shallowMount(GameServerSettings, {
      global: { renderStubDefaultSlot: true },
    })
    await flushPromises()

    expect(wrapper.findComponent(GameServerSettingsForm).props()).toMatchObject({
      canEditProvisioning: true,
      gameServerId: 'server-1',
    })
  })
})
