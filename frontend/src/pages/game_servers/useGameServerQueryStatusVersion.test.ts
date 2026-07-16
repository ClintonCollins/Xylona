import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref, type ComponentPublicInstance } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AllServersQueryInfoSchema,
  GameServerSchema,
  ServerQuery_Type,
  Status,
  type VersionInfo,
  VersionInfoSchema,
  VersionStatus,
} from '@/proto/shared_pb'
import { QueryGameServerResponseSchema } from '@/proto/xylona_pb'
import { XylonaEventBus } from '@/utils/shared'

const mocks = vi.hoisted(() => {
  const queryGameServer = vi.fn()

  return {
    queryGameServer,
    getXylonaClient: vi.fn(() => ({
      queryGameServer,
    })),
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')

  return {
    ...actual,
    GetXylonaClient: mocks.getXylonaClient,
  }
})

function makeGameServer(status: Status = Status.ONLINE) {
  return create(GameServerSchema, {
    id: 'server-1',
    name: 'Primary Server',
    status,
    version: '1.20.1',
  })
}

function makeMinecraftQueryResponse(
  currentPlayers: number,
  maxPlayers: number,
  playerList: string[] = [],
) {
  return create(QueryGameServerResponseSchema, {
    queryInfo: {
      serverId: 'server-1',
      serverName: 'Primary Server',
      type: ServerQuery_Type.Minecraft,
      minecraft: {
        numberOfPlayers: currentPlayers,
        maxPlayers,
        playerList,
      },
    },
  })
}

function makeSourceQueryResponse(currentPlayers: number, maxPlayers: number) {
  return create(QueryGameServerResponseSchema, {
    queryInfo: {
      serverId: 'server-1',
      serverName: 'Primary Server',
      type: ServerQuery_Type.Source,
      source: {
        players: currentPlayers,
        maxPlayers,
      },
    },
  })
}

function makePalworldQueryResponse(
  currentPlayers: number,
  maxPlayers: number,
  playerList: string[] = [],
) {
  return create(QueryGameServerResponseSchema, {
    queryInfo: {
      type: ServerQuery_Type.Palworld,
      palworld: {
        players: currentPlayers,
        maxPlayers,
        playerList,
        responded: true,
      },
    },
  })
}

function makeQueryInfoEvent(
  serverId: string,
  type: ServerQuery_Type,
  currentPlayers: number,
  maxPlayers: number,
  playerList: string[] = [],
) {
  if (type === ServerQuery_Type.Minecraft) {
    return create(AllServersQueryInfoSchema, {
      servers: {
        [serverId]: {
          serverId,
          serverName: serverId,
          type,
          minecraft: {
            numberOfPlayers: currentPlayers,
            maxPlayers,
            playerList,
          },
        },
      },
    })
  }

  if (type === ServerQuery_Type.Palworld) {
    return create(AllServersQueryInfoSchema, {
      servers: {
        [serverId]: {
          serverId,
          serverName: serverId,
          type,
          palworld: {
            players: currentPlayers,
            maxPlayers,
            playerList,
            responded: true,
          },
        },
      },
    })
  }

  return create(AllServersQueryInfoSchema, {
    servers: {
      [serverId]: {
        serverId,
        serverName: serverId,
        type,
        source: {
          players: currentPlayers,
          maxPlayers,
        },
      },
    },
  })
}

async function loadComposable() {
  const composablePath = './useGameServerQueryStatusVersion'
  return import(/* @vite-ignore */ composablePath)
}

type HarnessVm = ComponentPublicInstance & {
  currentPlayerCount: number
  gameServer: ReturnType<typeof makeGameServer>
  maxPlayerCount: number
  onlinePlayers: string[]
  playerListSupported: boolean
  queryGameServer: () => Promise<void>
  startQueryStatusVersionLifecycle: () => void
}

function getHarnessVm(wrapper: { vm: unknown }): HarnessVm {
  return wrapper.vm as HarnessVm
}

const mountedWrappers = new Set<{ unmount: () => void }>()
const cleanupCallbacks = new Set<() => void>()

function trackCleanup(cleanup: () => void) {
  cleanupCallbacks.add(cleanup)
}

function unmountHarness(wrapper: { unmount: () => void }) {
  if (!mountedWrappers.delete(wrapper)) {
    return
  }

  wrapper.unmount()
}

async function mountHarness(status: Status = Status.ONLINE) {
  const { useGameServerQueryStatusVersion } = await loadComposable()

  const Harness = defineComponent({
    setup() {
      const gameServer = ref(makeGameServer(status))
      const gameServerId = ref('server-1')

      return {
        gameServer,
        gameServerId,
        ...useGameServerQueryStatusVersion({
          gameServer,
          gameServerId,
        }),
      }
    },
    template: '<div />',
  })

  const wrapper = mount(Harness)
  mountedWrappers.add(wrapper)
  return wrapper
}

describe('useGameServerQueryStatusVersion', () => {
  beforeEach(() => {
    mocks.queryGameServer.mockReset()
    mocks.getXylonaClient.mockReset()
    mocks.getXylonaClient.mockReturnValue({
      queryGameServer: mocks.queryGameServer,
    })
  })

  afterEach(() => {
    for (const wrapper of [...mountedWrappers]) {
      wrapper.unmount()
    }
    mountedWrappers.clear()

    for (const cleanup of [...cleanupCallbacks].reverse()) {
      cleanup()
    }
    cleanupCallbacks.clear()

    vi.restoreAllMocks()
  })

  it.each([
    {
      label: 'Minecraft',
      response: makeMinecraftQueryResponse(7, 30, ['Alex', 'Steve']),
      expectedCurrent: 7,
      expectedMax: 30,
      expectedPlayers: ['Alex', 'Steve'],
      expectedPlayerListSupported: true,
    },
    {
      label: 'Source',
      response: makeSourceQueryResponse(11, 24),
      expectedCurrent: 11,
      expectedMax: 24,
      expectedPlayers: [],
      expectedPlayerListSupported: false,
    },
    {
      label: 'Palworld',
      response: makePalworldQueryResponse(3, 32, ['Cattiva', 'Lamball']),
      expectedCurrent: 3,
      expectedMax: 32,
      expectedPlayers: ['Cattiva', 'Lamball'],
      expectedPlayerListSupported: true,
    },
  ])(
    'queryGameServer applies $label player data from the initial RPC query',
    async ({
      response,
      expectedCurrent,
      expectedMax,
      expectedPlayers,
      expectedPlayerListSupported,
    }) => {
      mocks.queryGameServer.mockResolvedValue(response)
      const wrapper = await mountHarness()
      const vm = getHarnessVm(wrapper)

      await vm.queryGameServer()

      expect(mocks.queryGameServer).toHaveBeenCalledTimes(1)
      expect(mocks.queryGameServer).toHaveBeenCalledWith(
        expect.objectContaining({
          serverId: 'server-1',
        }),
      )
      expect(vm.currentPlayerCount).toBe(expectedCurrent)
      expect(vm.maxPlayerCount).toBe(expectedMax)
      expect(vm.onlinePlayers).toEqual(expectedPlayers)
      expect(vm.playerListSupported).toBe(expectedPlayerListSupported)
    },
  )

  it('registers lifecycle listeners once and removes only its own listeners on unmount', async () => {
    const onSpy = vi.spyOn(XylonaEventBus, 'on')
    const offSpy = vi.spyOn(XylonaEventBus, 'off')
    const sharedStatusListener = vi.fn()
    const wrapper = await mountHarness()
    const vm = getHarnessVm(wrapper)

    XylonaEventBus.on('gameServerStatus', sharedStatusListener)
    trackCleanup(() => XylonaEventBus.off('gameServerStatus', sharedStatusListener))

    vm.startQueryStatusVersionLifecycle()
    vm.startQueryStatusVersionLifecycle()

    const registeredEvents = onSpy.mock.calls.map(([eventName]) => eventName)
    expect(
      registeredEvents.filter((eventName) => eventName === 'gameServersQueryInfo'),
    ).toHaveLength(1)
    expect(registeredEvents.filter((eventName) => eventName === 'gameServerStatus')).toHaveLength(2)
    expect(registeredEvents.filter((eventName) => eventName === 'gameServerVersion')).toHaveLength(
      1,
    )

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Primary Server', Status.INSTALLING)
    await nextTick()

    expect(vm.gameServer.status).toBe(Status.INSTALLING)

    unmountHarness(wrapper)

    const removedEvents = offSpy.mock.calls.map(([eventName]) => eventName)
    expect(removedEvents.filter((eventName) => eventName === 'gameServersQueryInfo')).toHaveLength(
      1,
    )
    expect(removedEvents.filter((eventName) => eventName === 'gameServerStatus')).toHaveLength(1)
    expect(removedEvents.filter((eventName) => eventName === 'gameServerVersion')).toHaveLength(1)

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Primary Server', Status.OFFLINE)
    await nextTick()

    expect(sharedStatusListener).toHaveBeenCalledTimes(2)
    expect(vm.gameServer.status).toBe(Status.INSTALLING)
  })

  it('applies live status and version updates only for the active server', async () => {
    const versionInfo: VersionInfo = create(VersionInfoSchema, {
      status: VersionStatus.CHECKED,
      installedVersion: '1.21.1',
      latestVersion: '1.21.3',
      updateAvailable: true,
      lastCheckTime: 5n,
      trackerType: 'minecraft',
    })
    const wrapper = await mountHarness()
    const vm = getHarnessVm(wrapper)

    vm.startQueryStatusVersionLifecycle()

    XylonaEventBus.emit('gameServerStatus', 'server-99', 'Other Server', Status.OFFLINE)
    XylonaEventBus.emit('gameServerVersion', 'server-99', '9.9.9', versionInfo)
    await nextTick()

    expect(vm.gameServer.status).toBe(Status.ONLINE)
    expect(vm.gameServer.version).toBe('1.20.1')
    expect(vm.gameServer.versionInfo).toBeUndefined()

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Primary Server', Status.UPDATING)
    XylonaEventBus.emit('gameServerVersion', 'server-1', '1.21.1', versionInfo)
    await nextTick()

    expect(vm.gameServer.status).toBe(Status.UPDATING)
    expect(vm.gameServer.version).toBe('1.21.1')
    expect(vm.gameServer.versionInfo).toEqual(versionInfo)
  })

  it('applies live query info updates only for the active server', async () => {
    const wrapper = await mountHarness()
    const vm = getHarnessVm(wrapper)

    vm.startQueryStatusVersionLifecycle()

    XylonaEventBus.emit(
      'gameServersQueryInfo',
      makeQueryInfoEvent('server-99', ServerQuery_Type.Minecraft, 99, 100, ['Other Player']),
    )
    await nextTick()

    expect(vm.currentPlayerCount).toBe(0)
    expect(vm.maxPlayerCount).toBe(0)
    expect(vm.onlinePlayers).toEqual([])
    expect(vm.playerListSupported).toBe(false)

    XylonaEventBus.emit(
      'gameServersQueryInfo',
      makeQueryInfoEvent('server-1', ServerQuery_Type.Minecraft, 12, 64, ['Alex', 'Steve']),
    )
    await nextTick()

    expect(vm.currentPlayerCount).toBe(12)
    expect(vm.maxPlayerCount).toBe(64)
    expect(vm.onlinePlayers).toEqual(['Alex', 'Steve'])
    expect(vm.playerListSupported).toBe(true)

    XylonaEventBus.emit(
      'gameServersQueryInfo',
      makeQueryInfoEvent('server-1', ServerQuery_Type.Source, 8, 24),
    )
    await nextTick()

    expect(vm.currentPlayerCount).toBe(8)
    expect(vm.maxPlayerCount).toBe(24)
    expect(vm.onlinePlayers).toEqual([])
    expect(vm.playerListSupported).toBe(false)
  })

  it('preserves player counts when a live status update marks the active server offline', async () => {
    mocks.queryGameServer.mockResolvedValue(makeMinecraftQueryResponse(5, 20))
    const wrapper = await mountHarness()
    const vm = getHarnessVm(wrapper)

    await vm.queryGameServer()
    vm.startQueryStatusVersionLifecycle()

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Primary Server', Status.OFFLINE)
    await nextTick()

    expect(vm.gameServer.status).toBe(Status.OFFLINE)
    expect(vm.currentPlayerCount).toBe(5)
    expect(vm.maxPlayerCount).toBe(20)
    expect(vm.onlinePlayers).toEqual([])
    expect(vm.playerListSupported).toBe(true)
  })

  it('ignores late lifecycle starts after unmount so query, status, and version listeners do not leak', async () => {
    const sharedQueryInfoListener = vi.fn()
    const sharedStatusListener = vi.fn()
    const sharedVersionListener = vi.fn()
    const wrapper = await mountHarness()
    const vm = getHarnessVm(wrapper)
    const startQueryStatusVersionLifecycle = vm.startQueryStatusVersionLifecycle

    XylonaEventBus.on('gameServersQueryInfo', sharedQueryInfoListener)
    trackCleanup(() => XylonaEventBus.off('gameServersQueryInfo', sharedQueryInfoListener))

    XylonaEventBus.on('gameServerStatus', sharedStatusListener)
    trackCleanup(() => XylonaEventBus.off('gameServerStatus', sharedStatusListener))

    XylonaEventBus.on('gameServerVersion', sharedVersionListener)
    trackCleanup(() => XylonaEventBus.off('gameServerVersion', sharedVersionListener))

    unmountHarness(wrapper)

    startQueryStatusVersionLifecycle()

    XylonaEventBus.emit(
      'gameServersQueryInfo',
      makeQueryInfoEvent('server-1', ServerQuery_Type.Minecraft, 44, 80),
    )
    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Primary Server', Status.OFFLINE)
    XylonaEventBus.emit(
      'gameServerVersion',
      'server-1',
      '1.21.4',
      create(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '1.21.4',
        latestVersion: '1.21.4',
        updateAvailable: false,
        lastCheckTime: 9n,
        trackerType: 'minecraft',
      }),
    )
    await nextTick()

    expect(sharedQueryInfoListener).toHaveBeenCalledTimes(1)
    expect(sharedStatusListener).toHaveBeenCalledTimes(1)
    expect(sharedVersionListener).toHaveBeenCalledTimes(1)
    expect(vm.currentPlayerCount).toBe(0)
    expect(vm.maxPlayerCount).toBe(0)
    expect(vm.onlinePlayers).toEqual([])
    expect(vm.playerListSupported).toBe(false)
    expect(vm.gameServer.status).toBe(Status.ONLINE)
    expect(vm.gameServer.version).toBe('1.20.1')
    expect(vm.gameServer.versionInfo).toBeUndefined()
  })
})
