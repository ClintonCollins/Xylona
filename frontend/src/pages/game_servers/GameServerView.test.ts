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
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'

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
    dialogChoice: { value: 'ok' as 'ok' | 'dismiss' },
    dialog: vi.fn(() => {
      const chain = {
        onOk(callback: (value?: unknown) => void) {
          if (mocks.dialogChoice.value === 'ok') {
            callback()
          }
          return chain
        },
        onDismiss(callback: () => void) {
          callback()
          return chain
        },
      }
      return chain
    }),
    getGameServer: vi.fn(),
    getGameServerReadiness: vi.fn(),
    readGameServerOutput: vi.fn(),
    sendGameServerInput: vi.fn(),
    stopGameServer: vi.fn(),
    updateGameServer: vi.fn(),
    waitForOpen: vi.fn(),
    isOpen: vi.fn(),
    queryState: {
      currentPlayerCount: 0,
      maxPlayerCount: 0,
      onlinePlayers: [] as string[],
      playerListSupported: false,
    },
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
      dialog: mocks.dialog,
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
    getGameServerReadiness: mocks.getGameServerReadiness,
    readGameServerOutput: mocks.readGameServerOutput,
    sendGameServerInput: mocks.sendGameServerInput,
    stopGameServer: mocks.stopGameServer,
    updateGameServer: mocks.updateGameServer,
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
      currentPlayerCount: ref(mocks.queryState.currentPlayerCount),
      maxPlayerCount: ref(mocks.queryState.maxPlayerCount),
      onlinePlayers: ref([...mocks.queryState.onlinePlayers]),
      playerListSupported: ref(mocks.queryState.playerListSupported),
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
    mocks.getGameServerReadiness.mockResolvedValue({ items: [] })
    mocks.readGameServerOutput.mockReset()
    mocks.queryState.currentPlayerCount = 0
    mocks.queryState.maxPlayerCount = 0
    mocks.queryState.onlinePlayers = []
    mocks.queryState.playerListSupported = false
    mocks.queryGameServer.mockReset()
    mocks.startQueryStatusVersionLifecycle.mockReset()
    mocks.startMetricsPreviewLifecycle.mockReset()
    mocks.sendGameServerInput.mockReset()
    mocks.stopGameServer.mockReset()
    mocks.updateGameServer.mockReset()
    mocks.dialog.mockClear()
    mocks.dialogChoice.value = 'ok'
    setWebsocketConnectionStatus('connected')
  })

  afterEach(() => {
    mocks.eventBus.reset()
    mocks.notify.mockReset()
    mocks.stopGameServer.mockReset()
    mocks.getGameServer.mockReset()
    mocks.getGameServerReadiness.mockReset()
    mocks.readGameServerOutput.mockReset()
    mocks.waitForOpen.mockReset()
    mocks.isOpen.mockReset()
    mocks.sendGameServerInput.mockReset()
    mocks.updateGameServer.mockReset()
    setWebsocketConnectionStatus('connecting')
  })

  it.each([
    {
      label: 'supported roster with all names',
      currentPlayerCount: 2,
      onlinePlayers: ['Alex', 'Steve'],
      playerListSupported: true,
      expectedNames: ['Alex', 'Steve'],
      expectedMessage: '',
    },
    {
      label: 'supported roster with a partial sample',
      currentPlayerCount: 3,
      onlinePlayers: ['Alex'],
      playerListSupported: true,
      expectedNames: ['Alex'],
      expectedMessage: '2 more players not reported',
    },
    {
      label: 'supported empty roster',
      currentPlayerCount: 0,
      onlinePlayers: [],
      playerListSupported: true,
      expectedNames: [],
      expectedMessage: 'No players online',
    },
    {
      label: 'unsupported roster',
      currentPlayerCount: 4,
      onlinePlayers: [],
      playerListSupported: false,
      expectedNames: [],
      expectedMessage: '',
    },
  ])(
    'renders player names only for an online server with a $label',
    async ({
      currentPlayerCount,
      onlinePlayers,
      playerListSupported,
      expectedNames,
      expectedMessage,
    }) => {
      mocks.queryState.currentPlayerCount = currentPlayerCount
      mocks.queryState.maxPlayerCount = 20
      mocks.queryState.onlinePlayers = onlinePlayers
      mocks.queryState.playerListSupported = playerListSupported
      mocks.getGameServer.mockResolvedValue(
        create(GetGameServerResponseSchema, {
          gameServer: buildOnlineGameServer(),
        }),
      )
      mocks.readGameServerOutput.mockResolvedValue(
        create(ReadGameServerOutputResponseSchema, { output: '' }),
      )

      const wrapper = mountView()
      await flushPromises()

      const playerListPanel = wrapper.find('.player-list-panel')
      expect(playerListPanel.exists()).toBe(playerListSupported)
      expect(wrapper.findAll('.player-list-name').map((player) => player.text())).toEqual(
        expectedNames,
      )
      if (expectedMessage === '') {
        expect(wrapper.find('.player-list-empty').exists()).toBe(false)
        expect(wrapper.find('.player-list-note').exists()).toBe(false)
      } else {
        expect(playerListPanel.text()).toContain(expectedMessage)
      }
    },
  )

  it.each([
    {
      label: 'confirms before stopping when players are online',
      currentPlayerCount: 3,
      dialogChoice: 'ok' as const,
      wantDialog: true,
      wantStopped: true,
    },
    {
      label: 'aborts the stop when the player confirm is cancelled',
      currentPlayerCount: 3,
      dialogChoice: 'dismiss' as const,
      wantDialog: true,
      wantStopped: false,
    },
    {
      label: 'stops immediately when no players are online',
      currentPlayerCount: 0,
      dialogChoice: 'ok' as const,
      wantDialog: false,
      wantStopped: true,
    },
  ])('$label', async ({ currentPlayerCount, dialogChoice, wantDialog, wantStopped }) => {
    mocks.queryState.currentPlayerCount = currentPlayerCount
    mocks.queryState.maxPlayerCount = 20
    mocks.dialogChoice.value = dialogChoice
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildOnlineGameServer(),
      }),
    )
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )
    mocks.stopGameServer.mockResolvedValue({})

    const wrapper = mountView()
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      stopGameServer: () => Promise<void>
    }
    await viewModel.stopGameServer()

    if (wantDialog) {
      expect(mocks.dialog).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Stop Remote Minecraft?',
          message: '3 players are online and will be disconnected.',
          ok: expect.objectContaining({ label: 'Stop server', color: 'negative' }),
        }),
      )
    } else {
      expect(mocks.dialog).not.toHaveBeenCalled()
    }
    expect(mocks.stopGameServer).toHaveBeenCalledTimes(wantStopped ? 1 : 0)
  })

  it('shows game server update output in the console without an update progress dialog', async () => {
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )
    mocks.updateGameServer.mockResolvedValue({})

    const wrapper = mountView()
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      consoleLines: Array<{ html: string }>
      updateGameServer: () => Promise<void>
    }
    await viewModel.updateGameServer()

    mocks.eventBus.emit(
      'gameServerConsoleOutput',
      'server-remote-1',
      '[Xylona]: Downloading game server update\n',
    )
    await flushPromises()

    expect(
      viewModel.consoleLines.some((line) => line.html.includes('Downloading game server update')),
    ).toBe(true)
    expect(wrapper.findAll('.operation-progress-dialog-stub')).toHaveLength(1)
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

  it('replaces the visible console from reset snapshots during software operations', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildOnlineGameServer(),
      }),
    )
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )

    const wrapper = mountView()
    await flushPromises()

    mocks.eventBus.emit('gameServerConsoleOutput', 'server-remote-1', 'Stale output\n', 1n)
    const selector = wrapper.findComponent({ name: 'ServerSoftwareSelector' })
    selector.vm.$emit('software-operation-state', {
      status: 'installing',
      softwareId: 'paper',
      softwareName: 'Paper',
    })
    await flushPromises()

    mocks.eventBus.emit(
      'gameServerConsoleOutput',
      'server-remote-1',
      'Retained server output\n',
      2n,
      true,
    )

    const viewModel = wrapper.vm as unknown as {
      consoleLines: Array<{ html: string }>
      softwareOperationOutputLines: string[]
    }
    expect(
      viewModel.consoleLines.some((line) => line.html.includes('Retained server output')),
    ).toBe(true)
    expect(viewModel.consoleLines.some((line) => line.html.includes('Stale output'))).toBe(false)
    expect(viewModel.softwareOperationOutputLines).toEqual([])
  })

  it('preserves typed command text when sending fails', async () => {
    mocks.getGameServer.mockImplementation(() =>
      Promise.resolve(
        create(GetGameServerResponseSchema, {
          gameServer: buildOnlineGameServer(),
        }),
      ),
    )
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )
    mocks.sendGameServerInput.mockRejectedValueOnce(new Error('node unavailable'))

    const wrapper = mountView()
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      sendGameServerInput: () => Promise<void>
      serverInput: string
    }
    viewModel.serverInput = 'say Server restart in 5 minutes'
    await viewModel.sendGameServerInput()

    expect(mocks.sendGameServerInput).toHaveBeenCalledTimes(1)
    expect(viewModel.serverInput).toBe('say Server restart in 5 minutes')
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ caption: expect.stringContaining('Failed to send command') }),
    )
  })

  it('keeps console recovery visible until an explicit connected control arrives', async () => {
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )

    const wrapper = mountView()
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      consoleLines: Array<{ html: string }>
      consoleStreamState: string
    }

    mocks.eventBus.emit(
      'gameServerConsoleOutput',
      'server-remote-1',
      '[Xylona]: Console connection lost; reconnecting...\n',
      0n,
      false,
      true,
    )

    expect(viewModel.consoleStreamState).toBe('reconnecting')
    expect(viewModel.consoleLines.some((line) => line.html.includes('connection lost'))).toBe(true)

    mocks.eventBus.emit('gameServerConsoleOutput', 'server-remote-1', '', 0n, false, false)

    expect(viewModel.consoleStreamState).toBe('ready')
  })

  it('marks status stale on disconnect and refreshes it before re-enabling controls', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildOnlineGameServer(),
      }),
    )
    mocks.readGameServerOutput.mockResolvedValue(
      create(ReadGameServerOutputResponseSchema, { output: '' }),
    )

    const wrapper = mountView()
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      gameServer: { status: Status }
      serverStateAuthoritative: boolean
    }
    expect(mocks.getGameServer).toHaveBeenCalledTimes(1)
    expect(viewModel.serverStateAuthoritative).toBe(true)

    setWebsocketConnectionStatus('reconnecting')
    mocks.eventBus.emit('websocketDisconnected')
    expect(viewModel.gameServer.status).toBe(Status.ONLINE)
    expect(viewModel.serverStateAuthoritative).toBe(false)

    setWebsocketConnectionStatus('connected')
    mocks.eventBus.emit('websocketConnected')
    expect(viewModel.serverStateAuthoritative).toBe(false)
    await flushPromises()

    expect(mocks.getGameServer).toHaveBeenCalledTimes(2)
    const reconnectResponse = await mocks.getGameServer.mock.results[1]?.value
    expect(reconnectResponse.gameServer?.status).toBe(Status.ONLINE)
    expect(viewModel.gameServer.status).toBe(Status.ONLINE)
    expect(viewModel.serverStateAuthoritative).toBe(true)
  })
})
