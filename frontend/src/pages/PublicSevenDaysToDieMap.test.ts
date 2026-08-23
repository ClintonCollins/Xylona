import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import { SevenDaysToDieMapViewSchema, SevenDaysToDieWebAPIValueState } from '@/proto/xylona_pb'
import PublicSevenDaysToDieMap from './PublicSevenDaysToDieMap.vue'

const mocks = vi.hoisted(() => ({ getPublicMap: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getPublicSevenDaysToDieMap: mocks.getPublicMap }),
}))

describe('PublicSevenDaysToDieMap', () => {
  beforeEach(() => mocks.getPublicMap.mockReset())

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
      global: { stubs: { SevenDaysToDieWorldOverview: false } },
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
    expect(wrapper.getComponent(SevenDaysToDieLiveMap).props('view')).toEqual(publicView)

    wrapper.unmount()
  })
})
