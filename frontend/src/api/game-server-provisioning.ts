import { create } from '@bufbuild/protobuf'

import type { Game, GameServer, IP, Node } from '@/proto/shared_pb'
import {
  GetGameServerRequestSchema,
  ListGamesRequestSchema,
  ListIPsRequestSchema,
  ListNodesRequestSchema,
  ListUsersRequestSchema,
} from '@/proto/xylona_pb'

import { getXylonaClient } from './connect-client'

type ProvisioningClient = ReturnType<typeof getXylonaClient>

export interface ProvisioningOption {
  label: string
  value: string
}

export interface ProvisioningGamesResult {
  games: Game[]
  options: ProvisioningOption[]
}

export async function getGameServer(
  gameServerID: string,
  client: ProvisioningClient = getXylonaClient(),
): Promise<GameServer | undefined> {
  const response = await client.getGameServer(
    create(GetGameServerRequestSchema, {
      id: gameServerID,
    }),
  )
  return response.gameServer
}

export async function listGames(
  client: ProvisioningClient = getXylonaClient(),
): Promise<ProvisioningGamesResult> {
  const response = await client.listGames(create(ListGamesRequestSchema, {}))
  const games = response.games.slice()

  return {
    games,
    options: games.map((game) => ({
      label: game.name,
      value: game.id,
    })),
  }
}

export async function listNodes(client: ProvisioningClient = getXylonaClient()): Promise<Node[]> {
  const response = await client.listNodes(create(ListNodesRequestSchema, {}))
  return response.nodes.slice()
}

export async function listUsers(
  client: ProvisioningClient = getXylonaClient(),
): Promise<ProvisioningOption[]> {
  const response = await client.listUsers(create(ListUsersRequestSchema, {}))
  return response.users.map((user) => ({
    label: user.userName,
    value: user.id,
  }))
}

export async function listIPs(
  nodeId: string,
  client: ProvisioningClient = getXylonaClient(),
): Promise<IP[]> {
  const response = await client.listIPs(
    create(ListIPsRequestSchema, {
      nodeId,
    }),
  )
  return response.ips.slice()
}
