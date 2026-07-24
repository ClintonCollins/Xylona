import { flushPromises, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PageHeader from '@/components/shared/PageHeader.vue'
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
  it('renders the Settings page title', async () => {
    const wrapper = shallowMount(GameServerSettings, {
      global: { renderStubDefaultSlot: true },
    })
    await flushPromises()

    expect(wrapper.findComponent(PageHeader).props('title')).toBe('Settings')
  })
})
