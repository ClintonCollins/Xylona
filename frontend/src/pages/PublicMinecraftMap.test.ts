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
})
