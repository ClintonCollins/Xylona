import { create } from '@bufbuild/protobuf'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, GameServerSchema, IPSchema, NodeSchema } from '@/proto/shared_pb'

import { getGameServer, listGames, listIPs, listNodes, listUsers } from './game-server-provisioning'

describe('game-server-provisioning api', () => {
  const client = {
    getGameServer: vi.fn(),
    listGames: vi.fn(),
    listIPs: vi.fn(),
    listNodes: vi.fn(),
    listUsers: vi.fn(),
  }

  beforeEach(() => {
    client.getGameServer.mockReset()
    client.listGames.mockReset()
    client.listIPs.mockReset()
    client.listNodes.mockReset()
    client.listUsers.mockReset()
  })

  it('returns the provisioned game server when present', async () => {
    const gameServer = create(GameServerSchema, { id: 'server-1', name: 'One' })
    client.getGameServer.mockResolvedValue({ gameServer })

    await expect(getGameServer('server-1', client as never)).resolves.toBe(gameServer)
    expect(client.getGameServer).toHaveBeenCalledTimes(1)
  })

  it('shapes game list options for selects', async () => {
    client.listGames.mockResolvedValue({
      games: [
        create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        create(GameSchema, { id: 'factorio', name: 'Factorio' }),
      ],
    })

    await expect(listGames(client as never)).resolves.toEqual({
      games: [
        create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        create(GameSchema, { id: 'factorio', name: 'Factorio' }),
      ],
      options: [
        { label: 'Minecraft', value: 'minecraft' },
        { label: 'Factorio', value: 'factorio' },
      ],
    })
  })

  it('returns nodes, users, and IPs without component-side transport code', async () => {
    const node = create(NodeSchema, { id: 'node-1', name: 'Node 1', local: true })
    const ip = create(IPSchema, { address: '127.0.0.1' })
    client.listNodes.mockResolvedValue({ nodes: [node] })
    client.listUsers.mockResolvedValue({ users: [{ id: 'user-1', userName: 'owner' }] })
    client.listIPs.mockResolvedValue({ ips: [ip] })

    await expect(listNodes(client as never)).resolves.toEqual([node])
    await expect(listUsers(client as never)).resolves.toEqual([{ label: 'owner', value: 'user-1' }])
    await expect(listIPs('node-1', client as never)).resolves.toEqual([ip])
    expect(client.listIPs).toHaveBeenCalledWith(expect.objectContaining({ nodeId: 'node-1' }))
  })
})
