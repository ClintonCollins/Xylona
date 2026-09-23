import { nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  CONNECTING_NOTICE_DELAY_MS,
  restartConnectingNoticeDelay,
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
} from '@/utils/websocket-connection'
import Navigation from './Navigation.vue'

const mocks = vi.hoisted(() => ({
  logout: vi.fn(),
  push: vi.fn(),
  reconnectControllerWebsocket: vi.fn(),
  route: { path: '/game-servers' },
  superUser: false,
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/utils/shared', () => ({
  reconnectControllerWebsocket: mocks.reconnectControllerWebsocket,
}))

vi.mock('@/stores/xylona', () => ({
  useUserAuthStore: () => ({
    checkUserAuthenticated: vi.fn(),
    initialResponse: undefined,
    logout: mocks.logout,
    user: { id: 'user-1', userName: 'operator', superUser: mocks.superUser },
  }),
}))

function mountNavigation() {
  return shallowMount(Navigation, {
    global: { renderStubDefaultSlot: true },
  })
}

describe('Navigation', () => {
  afterEach(() => {
    setWebsocketConnectionStatus('connecting')
    setWebsocketBrowserOnline(true)
    mocks.route.path = '/game-servers'
    mocks.superUser = false
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('politely announces stale live state and offers an immediate retry while online', async () => {
    setWebsocketBrowserOnline(true)
    setWebsocketConnectionStatus('reconnecting')
    const wrapper = mountNavigation()

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
    const wrapper = mountNavigation()

    const notice = wrapper.find('[role="status"]')
    expect(notice.text()).toContain("You're offline")
    expect(notice.text()).toContain('Reconnection resumes when online')
    expect(wrapper.find('[aria-label="Reconnecting"]').exists()).toBe(false)
    expect(wrapper.find('[label="Retry now"]').exists()).toBe(false)
  })

  it('keeps a fresh connection quiet until it is overdue', async () => {
    vi.useFakeTimers()
    setWebsocketConnectionStatus('connecting')
    restartConnectingNoticeDelay()
    const wrapper = mountNavigation()

    expect(wrapper.find('[role="status"]').exists()).toBe(false)

    vi.advanceTimersByTime(CONNECTING_NOTICE_DELAY_MS)
    await nextTick()

    expect(wrapper.find('[role="status"]').text()).toContain('Connecting to live updates')
  })

  it.each([
    { path: '/games/new', link: '/games' },
    { path: '/games/minecraft/config-schema/0', link: '/games' },
    { path: '/admin/users/create', link: '/admin/users' },
    { path: '/game-servers/abc/console', link: '/game-servers' },
  ])('highlights $link on $path', ({ path, link }) => {
    mocks.route.path = path
    mocks.superUser = true
    const wrapper = mountNavigation()

    const activeLinks = wrapper
      .findAll('q-item-stub')
      .filter((item) => item.classes('q-router-link--active'))
      .map((item) => item.attributes('to'))
    expect(activeLinks).toEqual([link])
  })

  it('opens the change password dialog once the account menu has closed', async () => {
    const wrapper = mountNavigation()
    const dialog = wrapper.findComponent({ name: 'ChangePasswordDialog' })
    const item = wrapper.findAll('q-item-stub').find((i) => i.text() === 'Change password')

    await item?.trigger('click')
    expect(dialog.props('showDialog')).toBe(false)

    wrapper.findComponent({ name: 'QMenu' }).vm.$emit('hide')
    await nextTick()
    expect(dialog.props('showDialog')).toBe(true)
  })

  it('offers the account actions with a labelled sign out', async () => {
    const wrapper = mountNavigation()

    expect(wrapper.text()).toContain('Signed in as')
    expect(wrapper.text()).toContain('operator')
    expect(wrapper.text()).toContain('Change password')

    const signOut = wrapper.findAll('q-item-stub').find((item) => item.text() === 'Sign out')
    expect(signOut).toBeDefined()
    await signOut?.trigger('click')
    await nextTick()

    expect(mocks.logout).toHaveBeenCalledOnce()
    expect(mocks.push).toHaveBeenCalledWith('/login')
  })
})
