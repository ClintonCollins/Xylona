import { nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
} from '@/utils/websocket-connection'
import Navigation from './Navigation.vue'

const mocks = vi.hoisted(() => ({
  reconnectControllerWebsocket: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/game-servers' }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/utils/shared', () => ({
  reconnectControllerWebsocket: mocks.reconnectControllerWebsocket,
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
    setWebsocketBrowserOnline(true)
    vi.clearAllMocks()
  })

  it('politely announces stale live state and offers an immediate retry while online', async () => {
    setWebsocketBrowserOnline(true)
    setWebsocketConnectionStatus('reconnecting')
    const wrapper = shallowMount(Navigation, {
      global: { renderStubDefaultSlot: true },
    })

    const notice = wrapper.find('[role="status"]')
    expect(notice.exists()).toBe(true)
    expect(notice.attributes('aria-live')).toBe('polite')
    expect(notice.attributes('aria-atomic')).toBe('true')
    expect(notice.text()).toContain('Displayed server state may be stale')
    expect(wrapper.find('[aria-label="Reconnecting"]').exists()).toBe(true)

    const retry = wrapper.find('[label="Retry now"]')
    expect(retry.exists()).toBe(true)
    await retry.trigger('click')
    expect(mocks.reconnectControllerWebsocket).toHaveBeenCalledOnce()

    setWebsocketConnectionStatus('connected')
    await nextTick()

    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('shows an offline stale-state notice without reconnect animation or retry action', () => {
    setWebsocketBrowserOnline(false)
    setWebsocketConnectionStatus('disconnected')
    const wrapper = shallowMount(Navigation, {
      global: { renderStubDefaultSlot: true },
    })

    const notice = wrapper.find('[role="status"]')
    expect(notice.text()).toContain("You're offline")
    expect(notice.text()).toContain('Reconnection resumes when online')
    expect(wrapper.find('[aria-label="Reconnecting"]').exists()).toBe(false)
    expect(wrapper.find('[label="Retry now"]').exists()).toBe(false)
  })
})
