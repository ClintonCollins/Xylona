import { flushPromises, mount } from '@vue/test-utils'
import { create } from '@bufbuild/protobuf'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, IPSchema, NodeSchema } from '@/proto/shared_pb'

import { useGameServerFormState } from './useGameServerFormState'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  listGames: vi.fn(),
  listIPs: vi.fn(),
  listNodes: vi.fn(),
  listUsers: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('@/api/game-server-provisioning', () => ({
  getGameServer: mocks.getGameServer,
  listGames: mocks.listGames,
  listIPs: mocks.listIPs,
  listNodes: mocks.listNodes,
  listUsers: mocks.listUsers,
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

describe('useGameServerFormState', () => {
  beforeEach(() => {
    mocks.getGameServer.mockReset()
    mocks.listGames.mockReset()
    mocks.listIPs.mockReset()
    mocks.listNodes.mockReset()
    mocks.listUsers.mockReset()
    mocks.notify.mockReset()

    mocks.listGames.mockResolvedValue({
      games: [
        create(GameSchema, {
          id: 'minecraft',
          name: 'Minecraft',
          defaultPort: 25565n,
          defaultQueryPort: 25565n,
          defaultMaxPlayers: 20n,
        }),
      ],
      options: [{ label: 'Minecraft', value: 'minecraft' }],
    })
    mocks.listNodes.mockResolvedValue([
      create(NodeSchema, { id: 'node-local', name: 'Local Node', local: true }),
      create(NodeSchema, { id: 'node-remote', name: 'Remote Node', local: false }),
    ])
    mocks.listUsers.mockResolvedValue([{ label: 'owner', value: 'user-owner' }])
  })

  it('loads IPs for the selected node and refreshes them when the node changes', async () => {
    mocks.listIPs
      .mockResolvedValueOnce([
        create(IPSchema, { address: '127.0.0.1', nodeId: 'node-local' }),
      ])
      .mockResolvedValueOnce([
        create(IPSchema, { address: '10.0.0.10', nodeId: 'node-remote' }),
        create(IPSchema, { address: '198.51.100.10', nodeId: 'node-remote', external: true }),
      ])

    let state!: ReturnType<typeof useGameServerFormState>

    mount(
      defineComponent({
        async setup() {
          state = useGameServerFormState({
            loadProvisioningOptions: true,
          })
          await state.initialize()
          return () => null
        },
      }),
    )

    await flushPromises()

    expect(mocks.listIPs).toHaveBeenNthCalledWith(1, 'node-local')
    expect(state.gameServer.value.nodeId).toBe('node-local')
    expect(state.gameServer.value.ip?.address).toBe('127.0.0.1')

    state.gameServer.value.nodeId = 'node-remote'
    await flushPromises()

    expect(mocks.listIPs).toHaveBeenNthCalledWith(2, 'node-remote')
    expect(state.availableIPs.value.map((ip) => ip.address)).toEqual([
      '10.0.0.10',
      '198.51.100.10',
    ])
    expect(state.gameServer.value.ip?.address).toBe('198.51.100.10')
  })

  it('ignores stale IP responses after the selected node changes again', async () => {
    mocks.listNodes.mockResolvedValue([
      create(NodeSchema, { id: 'node-local', name: 'Local Node', local: true }),
      create(NodeSchema, { id: 'node-remote-a', name: 'Remote Node A', local: false }),
      create(NodeSchema, { id: 'node-remote-b', name: 'Remote Node B', local: false }),
    ])

    const pendingRequests = new Map<string, (ips: Array<ReturnType<typeof create<typeof IPSchema>>>) => void>()
    mocks.listIPs.mockImplementation((nodeId: string) => {
      if (nodeId === 'node-local') {
        return Promise.resolve([
          create(IPSchema, { address: '127.0.0.1', nodeId: 'node-local' }),
        ])
      }

      return new Promise((resolve) => {
        pendingRequests.set(nodeId, resolve)
      })
    })

    let state!: ReturnType<typeof useGameServerFormState>

    mount(
      defineComponent({
        async setup() {
          state = useGameServerFormState({
            loadProvisioningOptions: true,
          })
          await state.initialize()
          return () => null
        },
      }),
    )

    await flushPromises()

    state.gameServer.value.nodeId = 'node-remote-a'
    await flushPromises()

    state.gameServer.value.nodeId = 'node-remote-b'
    await flushPromises()

    const resolveRemoteB = pendingRequests.get('node-remote-b')
    if (!resolveRemoteB) {
      throw new Error('expected pending request for node-remote-b')
    }
    resolveRemoteB([
      create(IPSchema, { address: '10.0.0.20', nodeId: 'node-remote-b' }),
      create(IPSchema, { address: '198.51.100.20', nodeId: 'node-remote-b', external: true }),
    ])
    await flushPromises()

    expect(state.availableIPs.value.map((ip) => ip.address)).toEqual([
      '10.0.0.20',
      '198.51.100.20',
    ])
    expect(state.gameServer.value.ip?.address).toBe('198.51.100.20')

    const resolveRemoteA = pendingRequests.get('node-remote-a')
    if (!resolveRemoteA) {
      throw new Error('expected pending request for node-remote-a')
    }
    resolveRemoteA([
      create(IPSchema, { address: '10.0.0.10', nodeId: 'node-remote-a' }),
      create(IPSchema, { address: '198.51.100.10', nodeId: 'node-remote-a', external: true }),
    ])
    await flushPromises()

    expect(state.availableIPs.value.map((ip) => ip.address)).toEqual([
      '10.0.0.20',
      '198.51.100.20',
    ])
    expect(state.gameServer.value.nodeId).toBe('node-remote-b')
    expect(state.gameServer.value.ip?.address).toBe('198.51.100.20')
  })
})
