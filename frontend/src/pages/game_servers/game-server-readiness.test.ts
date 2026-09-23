import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'

import { GameServerReadinessItemSchema } from '@/proto/xylona_pb'
import { XylonaEventBus } from '@/utils/shared'

import {
  createGameServerReadiness,
  findStartBlocker,
  isConfigReadinessItem,
  readinessLabel,
} from './game-server-readiness'

const mocks = vi.hoisted(() => ({ getGameServerReadiness: vi.fn() }))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({ getGameServerReadiness: mocks.getGameServerReadiness }),
  }
})

function item(overrides: Record<string, unknown>) {
  return create(GameServerReadinessItemSchema, { required: true, ...overrides })
}

describe('game-server-readiness', () => {
  afterEach(() => {
    mocks.getGameServerReadiness.mockReset()
  })

  it('finds the first required blocking item and labels it', () => {
    const blocker = item({ kind: 'dragonwilds_config', blocking: true, message: 'Set Owner ID' })
    const complete = item({ kind: 'steam_gslt', complete: true })
    const items = [complete, item({ kind: 'optional', required: false, blocking: true }), blocker]

    expect(findStartBlocker(items)).toBe(blocker)
    expect(findStartBlocker([complete])).toBeUndefined()
    expect(readinessLabel(blocker.kind)).toBe('Dragonwilds configuration')
    expect(readinessLabel('something_new')).toBe('Setup')
    expect(isConfigReadinessItem(blocker)).toBe(true)
    expect(isConfigReadinessItem(item({ kind: 'minecraft_eula' }))).toBe(false)
  })

  it('reloads on mount, window focus and a rejected start for its own server', async () => {
    mocks.getGameServerReadiness.mockResolvedValue({
      items: [item({ kind: 'minecraft_eula', blocking: true })],
    })
    let readiness: ReturnType<typeof createGameServerReadiness> | undefined
    const wrapper = mount(
      defineComponent({
        setup() {
          readiness = createGameServerReadiness(ref('server-1'))
          return () => null
        },
      }),
    )
    await flushPromises()
    expect(mocks.getGameServerReadiness).toHaveBeenCalledTimes(1)
    expect(readiness?.items.value).toHaveLength(1)

    window.dispatchEvent(new Event('focus'))
    XylonaEventBus.emit('gameServerStartRejected', 'another-server')
    XylonaEventBus.emit('gameServerStartRejected', 'server-1')
    await flushPromises()
    expect(mocks.getGameServerReadiness).toHaveBeenCalledTimes(3)

    wrapper.unmount()
    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(mocks.getGameServerReadiness).toHaveBeenCalledTimes(3)
  })
})
