import { nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'
import Navigation from './Navigation.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/game-servers' }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/stores/xylona', () => ({
  useUserAuthStore: () => ({
    checkUserAuthenticated: vi.fn(),
    initialResponse: undefined,
    logout: vi.fn(),
    user: { id: 'user-1', userName: 'operator', superUser: false },
  }),
}))

describe('Navigation', () => {
  afterEach(() => {
    setWebsocketConnectionStatus('connecting')
  })

  it('announces stale live state until the controller connection is authoritative', async () => {
    setWebsocketConnectionStatus('reconnecting')
    const wrapper = shallowMount(Navigation, {
      global: { renderStubDefaultSlot: true },
    })

    const notice = wrapper.find('[role="status"]')
    expect(notice.exists()).toBe(true)
    expect(notice.attributes('aria-live')).toBe('assertive')
    expect(notice.text()).toContain('Displayed server status may be stale')

    setWebsocketConnectionStatus('connected')
    await nextTick()

    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })
})
