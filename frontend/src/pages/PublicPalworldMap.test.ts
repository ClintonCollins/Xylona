import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PublicPalworldMap from './PublicPalworldMap.vue'

const mocks = vi.hoisted(() => ({ getPublicMap: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getPublicPalworldMap: mocks.getPublicMap }),
}))

describe('PublicPalworldMap', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.getPublicMap.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('stops polling once the link is revoked', async () => {
    mocks.getPublicMap.mockRejectedValue(new ConnectError('gone', Code.NotFound))
    const wrapper = shallowMount(PublicPalworldMap, { props: { identifier: 'shared-map' } })
    await flushPromises()
    expect(wrapper.text()).toContain('This map link is not available')

    await vi.advanceTimersByTimeAsync(30_000)
    // Returning to the tab does not resume polling a revoked link either.
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(mocks.getPublicMap).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
