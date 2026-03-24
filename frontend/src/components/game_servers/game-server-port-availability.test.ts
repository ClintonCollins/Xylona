import { flushPromises, mount } from '@vue/test-utils'
import { create } from '@bufbuild/protobuf'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'

import {
  evaluateGameServerPortAvailability,
  useGameServerPortAvailability,
} from './game-server-port-availability'
import { GameSchema, GameServerSchema, IPSchema } from '@/proto/shared_pb'

const mocks = vi.hoisted(() => ({
  listGameServers: vi.fn(),
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
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
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
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
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
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25566n,
        }),
      ],
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
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 25565n,
          queryPort: 25565n,
        }),
      ],
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
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25565,
      selectedGame: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
    })

    expect(result.state).toBe('available')
    expect(result.message).toContain('available')
  })

  it('blocks an IP when the selected game needs a dedicated address', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Occupied Server',
          ip: create(IPSchema, { address: '216.177.177.228' }),
          port: 27015n,
          queryPort: 27016n,
        }),
      ],
      ipAddress: '216.177.177.228',
      port: 27017,
      queryPort: 27018,
      selectedGame: create(GameSchema, {
        id: 'source',
        name: 'Source Dedicated Server',
        requireDedicatedIp: true,
      }),
    })

    expect(result.state).toBe('conflict')
    expect(result.message).toContain('requires a dedicated IP')
  })

  it('marks a free port pair as available', () => {
    const result = evaluateGameServerPortAvailability({
      existingServers: [],
      ipAddress: '216.177.177.228',
      port: 25565,
      queryPort: 25566,
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
