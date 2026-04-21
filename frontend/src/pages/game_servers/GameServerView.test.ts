import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  GameSchema,
  GameServerSchema,
  ReadGameServerOutputResponseSchema,
  Status,
} from '@/proto/shared_pb'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'
import GameServerView from './GameServerView.vue'

const mocks = vi.hoisted(() => {
  type EventHandler = (...args: unknown[]) => void

  const listeners = new Map<string, Set<EventHandler>>()

  const eventBus = {
    on(event: string, handler: EventHandler) {
      if (!listeners.has(event)) {
        listeners.set(event, new Set())
      }
      listeners.get(event)?.add(handler)
      return eventBus
    },
    off(event: string, handler: EventHandler) {
      listeners.get(event)?.delete(handler)
      return eventBus
    },
    emit(event: string, ...args: unknown[]) {
      const handlers = listeners.get(event)
      if (!handlers) {
        return eventBus
      }
      for (const handler of handlers) {
        handler(...args)
      }
      return eventBus
    },
    reset() {
      listeners.clear()
    },
  }

  return {
    eventBus,
    routeState: {
      current: {
        params: {
          id: 'server-remote-1',
        },
      },
    },
    notify: vi.fn(),
    getGameServer: vi.fn(),
    readGameServerOutput: vi.fn(),
    waitForOpen: vi.fn(),
    isOpen: vi.fn(),
    queryGameServer: vi.fn(),
    startQueryStatusVersionLifecycle: vi.fn(),
    startMetricsPreviewLifecycle: vi.fn(),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
      screen: { lt: { md: false } },
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => mocks.routeState.current,
  }
})

vi.mock('@/utils/shared', () => ({
  bytesToSize: (value: bigint | number | string) => String(value),
  ConnectErrorToString: (err: unknown) => String(err),
  GetOrCreateXylonaWebsocketClient: () => ({
    waitForOpen: mocks.waitForOpen,
    isOpen: mocks.isOpen,
  }),
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    readGameServerOutput: mocks.readGameServerOutput,
  }),
  XylonaEventBus: mocks.eventBus,
}))

vi.mock('./useGameServerMetricsPreview', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useGameServerMetricsPreview: () => ({
      cpuBarClass: ref(''),
      formatRate: vi.fn(() => ''),
      formattedUptime: ref(''),
      memoryBarClass: ref(''),
      metricsConnections: ref(0),
      metricsCpu: ref(0),
      metricsCpuCores: ref(0),
      metricsDisk: ref(0),
      metricsIoReadRate: ref(''),
      metricsIoWriteRate: ref(''),
      metricsMaxMemory: ref(0),
      metricsMemory: ref(0),
      metricsMemoryPercent: ref(0),
      metricsMemoryRatio: ref(0),
      metricsThreads: ref(0),
      startMetricsPreviewLifecycle: mocks.startMetricsPreviewLifecycle,
    }),
  }
})

vi.mock('./useGameServerQueryStatusVersion', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useGameServerQueryStatusVersion: () => ({
      currentPlayerCount: ref(0),
      maxPlayerCount: ref(0),
      queryGameServer: mocks.queryGameServer,
      startQueryStatusVersionLifecycle: mocks.startQueryStatusVersionLifecycle,
    }),
  }
})

vi.mock('@/utils/game-server-notifications', () => ({
  recordLifecycleIntent: vi.fn(),
}))

function buildGameServer() {
  return create(GameServerSchema, {
    id: 'server-remote-1',
    name: 'Remote Minecraft',
    gameId: 'minecraft',
    gameName: 'Minecraft',
    status: Status.OFFLINE,
    port: 25565n,
    queryPort: 25565n,
    selectedVariantId: 'paper',
    selectedTarget: '',
    selectedTargetPinned: false,
    effectivePermissions: [],
    game: create(GameSchema, {
      id: 'minecraft',
      name: 'Minecraft',
      variants: [],
    }),
  })
}

function buildOnlineGameServer() {
  return create(GameServerSchema, {
    ...buildGameServer(),
    status: Status.ONLINE,
  })
}

function mountView() {
  return mount(GameServerView, {
    shallow: true,
    global: {
      renderStubDefaultSlot: true,
      stubs: {
        'q-input': {
          template: '<input />',
        },
        ClipBoardCopy: { template: '<div><slot /></div>' },
        StatusBadge: { template: '<div><slot /></div>' },
        OperationProgressDialog: {
          props: ['modelValue', 'outputLines', 'steps', 'complete'],
          template: '<div class="operation-progress-dialog-stub"><slot /></div>',
        },
        ServerSoftwareSelector: {
          name: 'ServerSoftwareSelector',
          emits: ['software-changed', 'software-operation-state'],
          template: '<div class="server-software-selector-stub" />',
        },
      },
    },
  })
}

describe('GameServerView', () => {
  beforeEach(() => {
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: {
        getItem: vi.fn(() => 'true'),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn(),
        key: vi.fn(),
        length: 0,
      },
    })
    Object.defineProperty(globalThis, 'requestAnimationFrame', {
      configurable: true,
      value: (callback: FrameRequestCallback) => {
        callback(0)
        return 1
      },
    })
    Object.defineProperty(globalThis, 'cancelAnimationFrame', {
      configurable: true,
      value: vi.fn(),
    })
    mocks.routeState.current = {
      params: {
        id: 'server-remote-1',
      },
    }
    mocks.waitForOpen.mockResolvedValue(undefined)
    mocks.isOpen.mockReturnValue(true)
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildGameServer(),
      }),
    )
    mocks.readGameServerOutput.mockReset()
    mocks.queryGameServer.mockReset()
    mocks.startQueryStatusVersionLifecycle.mockReset()
    mocks.startMetricsPreviewLifecycle.mockReset()
  })

  afterEach(() => {
    mocks.eventBus.reset()
    mocks.notify.mockReset()
    mocks.getGameServer.mockReset()
    mocks.readGameServerOutput.mockReset()
    mocks.waitForOpen.mockReset()
    mocks.isOpen.mockReset()
  })

  it('backfills console output after install completion when the live console is still empty', async () => {
    mocks.getGameServer
      .mockResolvedValueOnce(
        create(GetGameServerResponseSchema, {
          gameServer: buildGameServer(),
        }),
      )
      .mockResolvedValueOnce(
        create(GetGameServerResponseSchema, {
          gameServer: buildOnlineGameServer(),
        }),
      )
    mocks.readGameServerOutput
      .mockResolvedValueOnce(create(ReadGameServerOutputResponseSchema, { output: '' }))
      .mockResolvedValueOnce(
        create(ReadGameServerOutputResponseSchema, {
          output: 'Downloading latest server\n',
        }),
      )

    const wrapper = mountView()
    await flushPromises()

    const selector = wrapper.findComponent({ name: 'ServerSoftwareSelector' })
    selector.vm.$emit('software-operation-state', {
      status: 'installing',
      softwareId: 'paper',
      softwareName: 'Paper',
    })
    await flushPromises()

    mocks.eventBus.emit('gameServerConsoleOutput', 'server-remote-1', 'Downloading latest server\n')
    await flushPromises()

    selector.vm.$emit('software-operation-state', {
      status: 'complete',
      softwareId: 'paper',
      softwareName: 'Paper',
    })
    selector.vm.$emit('software-changed')
    await flushPromises()

    expect(mocks.getGameServer).toHaveBeenCalledTimes(2)
    expect(mocks.readGameServerOutput).toHaveBeenCalledTimes(2)
  })

  it('does not refetch the console buffer after install completion when output is already visible', async () => {
    mocks.getGameServer
      .mockResolvedValueOnce(
        create(GetGameServerResponseSchema, {
          gameServer: buildOnlineGameServer(),
        }),
      )
      .mockResolvedValueOnce(
        create(GetGameServerResponseSchema, {
          gameServer: buildOnlineGameServer(),
        }),
      )
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, {
        output: 'Server ready\n',
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const selector = wrapper.findComponent({ name: 'ServerSoftwareSelector' })
    selector.vm.$emit('software-changed')
    await flushPromises()

    expect(mocks.getGameServer).toHaveBeenCalledTimes(2)
    expect(mocks.readGameServerOutput).toHaveBeenCalledTimes(1)
  })
})
