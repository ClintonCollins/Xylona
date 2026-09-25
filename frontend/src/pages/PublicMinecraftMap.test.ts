import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PublicMinecraftMap from './PublicMinecraftMap.vue'

const mocks = vi.hoisted(() => ({ getPublicMinecraftMap: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getPublicMinecraftMap: mocks.getPublicMinecraftMap }),
}))

describe('PublicMinecraftMap', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-31T12:00:00Z'))
    mocks.getPublicMinecraftMap.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not replace iframe src on each poll', async () => {
    mocks.getPublicMinecraftMap
      .mockResolvedValueOnce({
        map: {
          available: true,
          gameServerName: 'Local One',
          viewerUrl: '/api/minecraft-map/shared/server-local-1/token-a/',
        },
      })
      .mockResolvedValue({
        map: {
          available: true,
          gameServerName: 'Local One',
          viewerUrl: '/api/minecraft-map/shared/server-local-1/token-b/',
        },
      })

    const wrapper = shallowMount(PublicMinecraftMap, { props: { identifier: 'Public_Map' } })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toBe(
      '/api/minecraft-map/shared/server-local-1/token-a/',
    )

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(mocks.getPublicMinecraftMap).toHaveBeenCalledTimes(2)
    expect(wrapper.get('iframe').attributes('src')).toBe(
      '/api/minecraft-map/shared/server-local-1/token-a/',
    )
    wrapper.unmount()
  })

  it('pauses polling while the tab is hidden', async () => {
    mocks.getPublicMinecraftMap.mockResolvedValue({ map: { gameServerName: 'Local One' } })
    const wrapper = shallowMount(PublicMinecraftMap, { props: { identifier: 'Public_Map' } })
    await flushPromises()

    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(mocks.getPublicMinecraftMap).toHaveBeenCalledTimes(1)

    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(mocks.getPublicMinecraftMap).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('stops polling once the link is revoked', async () => {
    mocks.getPublicMinecraftMap.mockRejectedValue(new ConnectError('gone', Code.NotFound))
    const wrapper = shallowMount(PublicMinecraftMap, { props: { identifier: 'Public_Map' } })
    await flushPromises()
    expect(wrapper.text()).toContain('This map link is not available')

    await vi.advanceTimersByTimeAsync(30_000)
    // Returning to the tab does not resume polling a revoked link either.
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(mocks.getPublicMinecraftMap).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
