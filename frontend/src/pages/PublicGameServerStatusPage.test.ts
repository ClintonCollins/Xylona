import { create, toJsonString } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Status } from '@/proto/shared_pb'
import {
  GameServerStatusPageRosterState,
  type PublicGameServerStatus,
  PublicGameServerStatusSchema,
  PublicGameServerStatusPageSchema,
} from '@/proto/xylona_pb'
import PublicGameServerStatusPage from './PublicGameServerStatusPage.vue'

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getStatusPage: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getPublicGameServerStatusPage: mocks.getStatusPage }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { identifier: 'Fleet_A' } }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    copyToClipboard: mocks.copyToClipboard,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

class FakeEventSource {
  static instances: FakeEventSource[] = []
  onerror: (() => void) | null = null
  closed = false
  private listeners = new Map<string, (event: MessageEvent<string>) => void>()

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener(name: string, listener: EventListenerOrEventListenerObject) {
    this.listeners.set(name, listener as (event: MessageEvent<string>) => void)
  }

  emit(name: string, data: string) {
    this.listeners.get(name)?.(new MessageEvent(name, { data }))
  }

  close() {
    this.closed = true
  }
}

describe('PublicGameServerStatusPage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-21T12:02:00Z'))
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    mocks.copyToClipboard.mockReset()
    mocks.copyToClipboard.mockResolvedValue(undefined)
    mocks.getStatusPage.mockReset()
    mocks.notify.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('keeps the latest snapshot while reconnecting and clears it after NotFound', async () => {
    const initial = create(PublicGameServerStatusPageSchema, {
      title: 'Owner fleet',
      servers: [
        {
          id: 'server-1',
          name: 'Alpha',
          gameName: 'Minecraft',
          status: Status.ONLINE,
          connectionAddress: 'play.example.test:25565',
          maxPlayerCount: 20,
          observedAt: timestampFromDate(new Date('2026-08-21T12:00:00Z')),
          playerNames: ['Alex'],
          rosterState: GameServerStatusPageRosterState.AVAILABLE,
        },
      ],
    })
    mocks.getStatusPage.mockResolvedValueOnce({ page: initial })
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()

    expect(wrapper.text()).toContain('Owner fleet')
    expect(wrapper.text()).not.toContain('0 / 20 players')
    expect(FakeEventSource.instances[0]?.url).toBe('/api/public/status-pages/Fleet_A/events')

    const initialServer = initial.servers[0]
    if (!initialServer) throw new Error('Expected the initial server fixture.')
    const live = create(PublicGameServerStatusPageSchema, {
      ...initial,
      servers: [{ ...initialServer, currentPlayerCount: 2 }],
    })
    FakeEventSource.instances[0]?.emit(
      'snapshot',
      toJsonString(PublicGameServerStatusPageSchema, live),
    )
    await flushPromises()
    expect(wrapper.text()).toContain('2 / 20')

    mocks.getStatusPage.mockRejectedValueOnce(new ConnectError('not found', Code.NotFound))
    FakeEventSource.instances[0]?.onerror?.()
    await flushPromises()
    expect(wrapper.text()).toContain('data observed 2m ago')
    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()

    expect(wrapper.text()).toContain('This status page is not available')
    wrapper.unmount()
    expect(FakeEventSource.instances[0]?.closed).toBe(true)
  })

  it('copies connection addresses with the fallback and reports failures', async () => {
    mocks.getStatusPage.mockResolvedValue({
      page: create(PublicGameServerStatusPageSchema, { title: 'Owner fleet' }),
    })
    vi.stubGlobal('navigator', {})
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      copiedServerID: string
      copyAddress: (server: PublicGameServerStatus) => Promise<void>
    }
    const server = create(PublicGameServerStatusSchema, {
      id: 'server-1',
      connectionAddress: 'play.example.test:25565',
    })

    await vm.copyAddress(server)

    expect(mocks.copyToClipboard).toHaveBeenCalledWith('play.example.test:25565')
    expect(vm.copiedServerID).toBe('server-1')

    mocks.copyToClipboard.mockRejectedValueOnce(new Error('copy failed'))
    await vm.copyAddress(create(PublicGameServerStatusSchema, { id: 'server-2' }))

    expect(mocks.notify).toHaveBeenCalledWith({
      type: 'negative',
      message: 'Could not copy the connection address.',
    })
  })
})
