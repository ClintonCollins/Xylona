import { create, toJsonString } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Status } from '@/proto/shared_pb'
import {
  GameServerStatusPagePlayersState,
  type PublicGameServerStatus,
  PublicGameServerStatusSchema,
  PublicGameServerStatusPageSchema,
} from '@/proto/xylona_pb'
import PublicGameServerStatusPage from './PublicGameServerStatusPage.vue'

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getStatusPage: vi.fn(),
  notify: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
}))

vi.mock('@/api/connect-client', () => ({
  getXylonaClient: () => ({ getPublicGameServerStatusPage: mocks.getStatusPage }),
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

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
}))

class FakeEventSource {
  static readonly CLOSED = 2
  static instances: FakeEventSource[] = []
  onerror: (() => void) | null = null
  closed = false
  readyState = 0
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
    this.readyState = FakeEventSource.CLOSED
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
    mocks.notifySuccess.mockReset()
    mocks.notifyError.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('does not open the live stream when the first snapshot lands after unmount', async () => {
    let resolveSnapshot!: (value: unknown) => void
    mocks.getStatusPage.mockReturnValueOnce(new Promise((resolve) => (resolveSnapshot = resolve)))
    const wrapper = shallowMount(PublicGameServerStatusPage)

    wrapper.unmount()
    resolveSnapshot({ page: create(PublicGameServerStatusPageSchema, { title: 'Late' }) })
    await flushPromises()

    expect(FakeEventSource.instances).toHaveLength(0)
  })

  it('keeps the latest snapshot while reconnecting and clears it after NotFound', async () => {
    const initialDocumentTitle = document.title
    const initial = create(PublicGameServerStatusPageSchema, {
      title: 'Owner fleet',
      servers: [
        {
          id: 'server-1',
          name: 'Alpha',
          gameName: 'Minecraft',
          status: Status.ONLINE,
          connectionAddress: 'play.example.test:25565',
          publicNote: 'Bring a friend.\nEvents begin at 8 PM.',
          publicPassword: 'join-us',
          publicMapPath: '/maps/Alpha_Map',
          version: '1.21.1',
          maxPlayerCount: 20,
          observedAt: timestampFromDate(new Date('2026-08-21T12:00:00Z')),
          playerNames: ['Alex'],
          playersState: GameServerStatusPagePlayersState.AVAILABLE,
        },
      ],
    })
    mocks.getStatusPage.mockResolvedValueOnce({ page: initial })
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()

    expect(wrapper.text()).toContain('Owner fleet')
    expect(document.title).toBe('Owner fleet · Xylona')
    expect(wrapper.text()).toContain('Live · updated 2m ago')
    expect(wrapper.text()).not.toContain('0 known · 1 unavailable')
    expect(wrapper.text()).toContain('Player count unavailable')
    expect(wrapper.text()).toContain('1.21.1')
    expect(wrapper.get('.public-server-row').classes()).toContain('is-online')
    expect(wrapper.text()).toContain('Bring a friend.')
    expect(wrapper.text()).toContain('join-us')
    const mapLink = wrapper.get('[aria-label="Open Alpha public map"]')
    expect(mapLink.attributes('href')).toBe('/maps/Alpha_Map')
    expect(mapLink.attributes('target')).toBe('_blank')
    expect(mapLink.attributes('rel')).toBe('noopener noreferrer')
    expect(FakeEventSource.instances[0]?.url).toBe('/api/public/status-pages/Fleet_A/events')

    const initialServer = initial.servers[0]
    if (!initialServer) throw new Error('Expected the initial server fixture.')
    const live = create(PublicGameServerStatusPageSchema, {
      ...initial,
      servers: [
        {
          ...initialServer,
          currentPlayerCount: 2,
          publicNote: 'Map reset tomorrow.',
          publicPassword: '',
          publicMapPath: '',
        },
      ],
    })
    FakeEventSource.instances[0]?.emit(
      'snapshot',
      toJsonString(PublicGameServerStatusPageSchema, live),
    )
    await flushPromises()
    expect(wrapper.text()).toContain('2 / 20')
    // Rows are not live regions; only the online summary speaks.
    expect(wrapper.get('.public-status').text()).toBe('Online')
    expect(wrapper.findAll('[role="status"]').map((region) => region.text())).toEqual([
      '',
      '1 of 1 servers online',
    ])
    expect(wrapper.text()).toContain('Map reset tomorrow.')
    expect(wrapper.text()).not.toContain('join-us')
    expect(wrapper.find('[aria-label="Open Alpha public map"]').exists()).toBe(false)

    mocks.getStatusPage.mockRejectedValueOnce(new ConnectError('not found', Code.NotFound))
    FakeEventSource.instances[0]?.onerror?.()
    await flushPromises()
    expect(wrapper.text()).toContain('data observed 2m ago')
    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()

    expect(wrapper.text()).toContain('This status page is not available')
    expect(wrapper.get('.public-status-state--fault').attributes('role')).toBe('alert')
    wrapper.unmount()
    expect(FakeEventSource.instances[0]?.closed).toBe(true)
    expect(document.title).toBe(initialDocumentTitle)
  })

  it('reopens a closed live stream once polling reaches the controller again', async () => {
    mocks.getStatusPage.mockResolvedValue({
      page: create(PublicGameServerStatusPageSchema, { title: 'Owner fleet' }),
    })
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()

    // A non-200 response (a proxy 502 during a controller restart) closes the stream for good.
    const first = FakeEventSource.instances[0]
    if (!first) throw new Error('Expected the live stream to open.')
    first.readyState = FakeEventSource.CLOSED
    first.onerror?.()
    await flushPromises()
    expect(wrapper.text()).toContain('Live updates were interrupted')

    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()
    expect(FakeEventSource.instances).toHaveLength(2)
    // Polling alone doesn't clear the notice; the reopened stream's first snapshot does.
    expect(wrapper.text()).toContain('Live updates were interrupted')

    FakeEventSource.instances[1]?.emit(
      'snapshot',
      toJsonString(
        PublicGameServerStatusPageSchema,
        create(PublicGameServerStatusPageSchema, { title: 'Owner fleet' }),
      ),
    )
    await flushPromises()
    expect(wrapper.text()).not.toContain('Live updates were interrupted')
  })

  it('does not reopen the live stream when a poll lands after unmount', async () => {
    mocks.getStatusPage.mockResolvedValue({
      page: create(PublicGameServerStatusPageSchema, { title: 'Owner fleet' }),
    })
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()
    const first = FakeEventSource.instances[0]
    if (!first) throw new Error('Expected the live stream to open.')
    first.readyState = FakeEventSource.CLOSED
    first.onerror?.()
    await flushPromises()

    let resolvePoll!: (value: unknown) => void
    mocks.getStatusPage.mockReturnValueOnce(new Promise((resolve) => (resolvePoll = resolve)))
    await vi.advanceTimersByTimeAsync(15_000)
    wrapper.unmount()
    resolvePoll({ page: create(PublicGameServerStatusPageSchema, { title: 'Late' }) })
    await flushPromises()
    expect(FakeEventSource.instances).toHaveLength(1)
  })

  it("offers the Who's online toggle only for online servers", async () => {
    mocks.getStatusPage.mockResolvedValue({
      page: create(PublicGameServerStatusPageSchema, {
        title: 'Owner fleet',
        servers: [
          {
            id: 'server-1',
            name: 'Alpha',
            status: Status.ONLINE,
            playersState: GameServerStatusPagePlayersState.AVAILABLE,
          },
          {
            id: 'server-2',
            name: 'Beta',
            status: Status.OFFLINE,
            playersState: GameServerStatusPagePlayersState.AVAILABLE,
          },
        ],
      }),
    })
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()

    expect(wrapper.find('[aria-label="Show Alpha online players"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="Show Beta online players"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('copies connection addresses with the fallback and reports failures', async () => {
    mocks.getStatusPage.mockResolvedValue({
      page: create(PublicGameServerStatusPageSchema, {
        title: 'Owner fleet',
        servers: [
          {
            id: 'server-1',
            name: 'Alpha',
            connectionAddress: 'play.example.test:25565',
          },
        ],
      }),
    })
    vi.stubGlobal('navigator', {})
    const wrapper = shallowMount(PublicGameServerStatusPage)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      copiedServerID: string
      copyAnnouncement: string
      copyAddress: (server: PublicGameServerStatus) => Promise<void>
    }
    const server = create(PublicGameServerStatusSchema, {
      id: 'server-1',
      name: 'Alpha',
      connectionAddress: 'play.example.test:25565',
    })

    await vm.copyAddress(server)

    expect(mocks.copyToClipboard).toHaveBeenCalledWith('play.example.test:25565')
    expect(vm.copiedServerID).toBe('server-1')
    expect(vm.copyAnnouncement).toBe('Alpha connection address copied.')
    await wrapper.vm.$nextTick()
    const copiedButton = wrapper.get('[aria-label="Alpha connection address copied"]')
    expect(copiedButton.attributes('icon')).toBe('check')
    expect(copiedButton.classes()).toContain('is-copied')

    await vi.advanceTimersByTimeAsync(1500)
    expect(vm.copyAnnouncement).toBe('')

    mocks.copyToClipboard.mockRejectedValueOnce(new Error('copy failed'))
    await vm.copyAddress(create(PublicGameServerStatusSchema, { id: 'server-2' }))

    expect(mocks.notifyError).toHaveBeenCalledWith('Could not copy the connection address.')
    wrapper.unmount()
  })
})
