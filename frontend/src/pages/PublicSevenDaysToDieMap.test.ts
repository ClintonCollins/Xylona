import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'

import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import { SevenDaysToDieMapViewSchema, SevenDaysToDieWebAPIValueState } from '@/proto/xylona_pb'
import PublicSevenDaysToDieMap from './PublicSevenDaysToDieMap.vue'

// The page loads the live map through defineAsyncComponent, so shallowMount's default stub has
// no props. Stub it with the real prop list so props() keeps working.
const LiveMapStub = defineComponent({ props: SevenDaysToDieLiveMap.props, render: () => null })

const mocks = vi.hoisted(() => ({ getPublicMap: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getPublicSevenDaysToDieMap: mocks.getPublicMap }),
}))

describe('PublicSevenDaysToDieMap', () => {
  // A block body: a returned function would run as teardown and call the mock again.
  beforeEach(() => {
    mocks.getPublicMap.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it.each([
    { code: Code.Unavailable, want: 'Map temporarily unavailable' },
    { code: Code.NotFound, want: 'This map link is not available' },
  ])('shows "$want" when the first load fails with code $code', async ({ code, want }) => {
    mocks.getPublicMap.mockRejectedValue(new ConnectError('failed', code))
    const wrapper = shallowMount(PublicSevenDaysToDieMap, {
      props: { identifier: 'shared-map' },
      global: { stubs: { SevenDaysToDieLiveMap: LiveMapStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain(want)
    expect(wrapper.findComponent(LiveMapStub).exists()).toBe(false)
    wrapper.unmount()
  })

  it('stops polling once the link is revoked', async () => {
    vi.useFakeTimers()
    mocks.getPublicMap.mockRejectedValue(new ConnectError('gone', Code.NotFound))
    const wrapper = shallowMount(PublicSevenDaysToDieMap, {
      props: { identifier: 'shared-map' },
      global: { stubs: { SevenDaysToDieLiveMap: LiveMapStub } },
    })
    await flushPromises()

    await vi.advanceTimersByTimeAsync(30_000)
    // Returning to the tab does not resume polling a revoked link either.
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(mocks.getPublicMap).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('shows the shared world overview and passes tactical data to the live map', async () => {
    const available = SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
    const unsupported =
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED
    const publicView = create(SevenDaysToDieMapViewSchema, {
      enabled: true,
      gameServerName: 'Shared server',
      mapSize: { x: 10_240, z: 10_240 },
      players: [{ id: 'public-player', name: 'Clinton', position: { x: 10, z: 20 } }],
      claimsState: unsupported,
      hostileState: available,
      animalState: available,
      bloodMoonState: available,
      bloodMoon: {
        gameTime: { day: 3, hour: 20, minute: 39 },
        active: false,
        nextBloodMoon: { day: 7, hour: 22 },
        nextBloodMoonEnd: { day: 8, hour: 4 },
      },
    })
    mocks.getPublicMap.mockResolvedValue({ map: publicView })

    const wrapper = shallowMount(PublicSevenDaysToDieMap, {
      props: { identifier: 'shared-map' },
      global: { stubs: { SevenDaysToDieWorldOverview: false, SevenDaysToDieLiveMap: LiveMapStub } },
    })
    await flushPromises()

    const overview = wrapper.get('[data-testid="world-overview"]')
    expect(overview.text()).toContain('World overview')
    expect(overview.text()).toContain('Day 3, 20:39')
    expect(overview.text()).toContain('Inactive')
    expect(overview.text()).toContain('Day 7, 22:00')
    expect(overview.text()).toContain('Day 8, 04:00')
    expect(overview.text()).toContain('0 online · 1 known')
    expect(overview.text()).toContain('10,240 × 10,240')
    expect(overview.text()).toContain('Not supported by this WebAPI')
    expect(wrapper.getComponent(LiveMapStub).props('view')).toEqual(publicView)

    wrapper.unmount()
  })
})
