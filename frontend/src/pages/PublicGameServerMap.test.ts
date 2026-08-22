import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameServerMapKind } from '@/proto/xylona_pb'
import PublicGameServerMap from './PublicGameServerMap.vue'
import PublicPalworldMap from './PublicPalworldMap.vue'

const mocks = vi.hoisted(() => ({ resolveMap: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ resolvePublicGameServerMap: mocks.resolveMap }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { identifier: 'Public_Map' } }),
}))

describe('PublicGameServerMap', () => {
  beforeEach(() => mocks.resolveMap.mockReset())

  it('resolves the identifier and passes it to the matching public map', async () => {
    mocks.resolveMap.mockResolvedValue({ kind: GameServerMapKind.PALWORLD })
    const wrapper = shallowMount(PublicGameServerMap)
    await flushPromises()

    expect(mocks.resolveMap).toHaveBeenCalledWith(
      expect.objectContaining({ publicIdentifier: 'Public_Map' }),
    )
    expect(wrapper.findComponent(PublicPalworldMap).props('identifier')).toBe('Public_Map')
  })

  it('uses the same unavailable state when resolution fails', async () => {
    mocks.resolveMap.mockResolvedValue({ kind: GameServerMapKind.UNSPECIFIED })
    const wrapper = shallowMount(PublicGameServerMap)
    await flushPromises()

    expect(wrapper.text()).toContain('This map link is not available')
    expect(wrapper.findComponent(PublicPalworldMap).exists()).toBe(false)
  })
})
