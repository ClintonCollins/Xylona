import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { QBtn } from 'quasar'
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

  it('offers a retry instead of calling a link revoked when resolution fails', async () => {
    mocks.resolveMap
      .mockRejectedValueOnce(new ConnectError('controller down', Code.Unavailable))
      .mockResolvedValueOnce({ kind: GameServerMapKind.PALWORLD })
    const wrapper = shallowMount(PublicGameServerMap)
    await flushPromises()

    expect(wrapper.text()).toContain('Map temporarily unavailable')
    expect(wrapper.text()).not.toContain('This map link is not available')

    wrapper.getComponent(QBtn).vm.$emit('click')
    await flushPromises()

    expect(wrapper.findComponent(PublicPalworldMap).exists()).toBe(true)
  })

  it('says the link is not available only when the map is not found', async () => {
    mocks.resolveMap.mockRejectedValueOnce(new ConnectError('gone', Code.NotFound))
    const wrapper = shallowMount(PublicGameServerMap)
    await flushPromises()

    expect(wrapper.text()).toContain('This map link is not available')
  })

  it('uses the same unavailable state when resolution fails', async () => {
    mocks.resolveMap.mockResolvedValue({ kind: GameServerMapKind.UNSPECIFIED })
    const wrapper = shallowMount(PublicGameServerMap)
    await flushPromises()

    expect(wrapper.text()).toContain('This map link is not available')
    expect(wrapper.findComponent(PublicPalworldMap).exists()).toBe(false)
  })
})
