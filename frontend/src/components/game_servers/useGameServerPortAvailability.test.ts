import { flushPromises, mount } from '@vue/test-utils'
import { create } from '@bufbuild/protobuf'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'

import {
  evaluateGameServerPortAvailability,
  useGameServerPortAvailability,
} from './useGameServerPortAvailability'
import { GameSchema, GameServerSchema, IPSchema } from '@/proto/shared_pb'

const mocks = vi.hoisted(() => ({
  listGameServers: vi.fn(),
  notifyConnectError: vi.fn(),
  suggestGameServerPorts: vi.fn(),
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notifyConnectError,
}))

vi.mock('@/api/game-server-provisioning', () => ({
  suggestGameServerPorts: mocks.suggestGameServerPorts,
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listGameServers: mocks.listGameServers,
    }),
  }
})

describe('evaluateGameServerPortAvailability', () => {
  it('reports a conflicting port on the selected IP', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Existing Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25567,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('Port 25565 is already in use')
    expect(result.message).toContain('Existing Server')
  })

  it('reports a conflicting port on the selected IP even when the query port is blank', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Existing Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 0,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('Port 25565 is already in use')
  })

  it('reports a conflicting port on the selected IP even before the selected game resolves', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Existing Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25567,
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('Port 25565 is already in use')
  })

  it('ignores query port reuse on the selected IP', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Existing Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25565n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25566,
      queryPort: 25565,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })

  it('allows the same port and query port on the same IP', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25565,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })

  it('allows the same port on a different IP when neither game binds to all IPs', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Other IP Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.229' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25567,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })

  it('blocks a node port when the selected game binds to all IPs', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Occupied Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.229' }),
          port: 27015n,
          queryPort: 27016n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 27015,
      queryPort: 27016,
      selectedGame: create(GameSchema, {
        id: 'source',
        name: 'Source Dedicated Server',
        bindsToAllIps: true,
      }),
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('binds to all IPs')
  })

  it('blocks a node port when an existing game binds to all IPs', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Bind All Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.229' }),
          port: 27015n,
          queryPort: 27016n,
          game: create(GameSchema, {
            id: 'source',
            name: 'Source Dedicated Server',
            bindsToAllIps: true,
          }),
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 27015,
      queryPort: 27016,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('binds to all IPs')
  })

  it('marks a free port pair as available', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25566,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })

  it('ignores servers on other nodes even when IP and port match', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Other Node Server',
          nodeId: 'node-remote',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
      nodeId: 'node-local',
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25567,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })
})

describe('useGameServerPortAvailability', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.listGameServers.mockReset()
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
    mocks.suggestGameServerPorts.mockReset()
    mocks.notifyConnectError.mockReset()
  })

  function mountComposable(
    gameServer: ReturnType<typeof ref>,
    selectedGame: ReturnType<typeof ref>,
  ) {
    let composableState!: ReturnType<typeof useGameServerPortAvailability>
    mount(
      defineComponent({
        setup() {
          composableState = useGameServerPortAvailability({
            enabled: ref(true),
            selectedGame: selectedGame as never,
            gameServer: gameServer as never,
          })
          return () => null
        },
      }),
    )
    return composableState
  }

  it('fills in the next free pair when the game defaults are taken, and keeps typed ports', async () => {
    mocks.suggestGameServerPorts.mockResolvedValue({ port: 2458n, queryPort: 2459n })
    const selectedGame = ref(
      create(GameSchema, { id: 'valheim', defaultPort: 2456n, defaultQueryPort: 2457n }),
    )
    const gameServer = ref(
      create(GameServerSchema, {
        nodeId: 'node-local',
        ip: create(IPSchema, { address: '10.0.0.5' }),
        port: 2456n,
        queryPort: 2457n,
      }),
    )

    const state = mountComposable(gameServer, selectedGame)
    await flushPromises()

    expect(mocks.suggestGameServerPorts).toHaveBeenCalledWith({
      nodeId: 'node-local',
      ipAddress: '10.0.0.5',
      gameId: 'valheim',
      port: 2456n,
      queryPort: 2457n,
    })
    expect(gameServer.value.port).toBe(2458n)
    expect(gameServer.value.queryPort).toBe(2459n)
    expect(state.portSuggestionNote.value).toContain(
      `Port 2456 or one of the ports this game also needs is taken`,
    )

    // A typed port clears the note and is never replaced when the IP changes.
    gameServer.value.port = 30000n
    gameServer.value.queryPort = 30001n
    await flushPromises()
    expect(state.portSuggestionNote.value).toBe('')
    gameServer.value.ip = create(IPSchema, { address: '10.0.0.6' })
    await flushPromises()
    expect(mocks.suggestGameServerPorts).toHaveBeenCalledTimes(1)
    expect(gameServer.value.port).toBe(30000n)
  })

  it('moves a typed conflicting port to the next free pair on request', async () => {
    mocks.suggestGameServerPorts.mockResolvedValue({ port: 25567n, queryPort: 25567n })
    const selectedGame = ref(
      create(GameSchema, { id: 'minecraft', defaultPort: 25565n, defaultQueryPort: 25565n }),
    )
    const gameServer = ref(
      create(GameServerSchema, {
        nodeId: 'node-local',
        ip: create(IPSchema, { address: '10.0.0.5' }),
        port: 25566n,
        queryPort: 25566n,
      }),
    )

    const state = mountComposable(gameServer, selectedGame)
    await flushPromises()
    expect(mocks.suggestGameServerPorts).not.toHaveBeenCalled()

    await state.fillNextFreePorts()

    expect(mocks.suggestGameServerPorts).toHaveBeenCalledWith(
      expect.objectContaining({ port: 25566n, queryPort: 25566n }),
    )
    expect(gameServer.value.port).toBe(25567n)
    expect(state.portSuggestionNote.value).toContain(
      `Port 25566 or one of the ports this game also needs is taken`,
    )
  })

  it('tells the operator when a requested free port lookup fails', async () => {
    const failure = new Error('no free port')
    mocks.suggestGameServerPorts.mockRejectedValue(failure)
    const selectedGame = ref(
      create(GameSchema, { id: 'minecraft', defaultPort: 25565n, defaultQueryPort: 25565n }),
    )
    const gameServer = ref(
      create(GameServerSchema, {
        nodeId: 'node-local',
        ip: create(IPSchema, { address: '10.0.0.5' }),
        port: 25566n,
        queryPort: 25566n,
      }),
    )

    const state = mountComposable(gameServer, selectedGame)
    await flushPromises()
    await state.fillNextFreePorts()

    expect(mocks.notifyConnectError).toHaveBeenCalledWith(failure, 'Could not find a free port')
    expect(gameServer.value.port).toBe(25566n)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('debounces the live server lookup and exposes the conflict state', async () => {
    mocks.listGameServers.mockResolvedValue({
      gameServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Existing Server',
          nodeId: 'node-local',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
    })

    const enabled = ref(true)
    const selectedGame = ref(create(GameSchema, { id: 'minecraft', name: 'Minecraft' }))
    const gameServer = ref(
      create(GameServerSchema, {
        nodeId: 'node-local',
        ip: create(IPSchema, { address: '216.177.177.228' }),
        port: 25565n,
        queryPort: 25567n,
      }),
    )

    let composableState!: ReturnType<typeof useGameServerPortAvailability>

    mount(
      defineComponent({
        setup() {
          composableState = useGameServerPortAvailability({
            enabled,
            selectedGame,
            gameServer,
          })
          return () => null
        },
      }),
    )

    await vi.advanceTimersByTimeAsync(299)
    expect(mocks.listGameServers).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()

    expect(mocks.listGameServers).toHaveBeenCalledTimes(1)
    expect(composableState.portAvailabilityBlocking.value).toBe(true)
    expect(composableState.portAvailabilityMessage.value).toContain('Existing Server')
  })
})
