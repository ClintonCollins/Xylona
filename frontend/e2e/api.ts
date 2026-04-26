import { create } from '@bufbuild/protobuf'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

import {
  BackupRestoreMode,
  CreateGameServerRequestSchema,
  GameServerSchema,
  IPSchema,
  type Game,
} from '@/proto/shared_pb'
import {
  AddGameRequestSchema,
  CreateGameServerBackupRequestSchema,
  CreateUserRequestSchema,
  DeleteGameServerBackupRequestSchema,
  GetGameRequestSchema,
  ListGameServerBackupsRequestSchema,
  RemoveGameRequestSchema,
  RestoreGameServerBackupRequestSchema,
  UpdateGameStartArgBlocklistRequestSchema,
  UpdateGameStartArgsTemplateRequestSchema,
  Xylona,
} from '@/proto/xylona_pb'
import { GameServerFileOrDirectoryCreateRequestSchema } from '@/proto/gameserver_files_operations_pb'

export const BACKEND_URL = process.env['BACKEND_URL'] ?? 'http://localhost:9091'

export interface ApiCookies {
  sessionId: string
  sessionToken: string
  raw: string
}

export interface TestUser {
  id: string
  username: string
  password: string
  superUser: boolean
}

export function createAPIClient(cookies: ApiCookies) {
  const transport = createConnectTransport({
    baseUrl: BACKEND_URL,
    fetch: (input, init) => {
      const headers = new Headers(init?.headers)
      headers.set('Cookie', cookies.raw)
      return fetch(input, { ...init, headers })
    },
  })
  return createClient(Xylona, transport)
}

function extractCookies(setCookieHeaders: string[]): ApiCookies {
  let sessionId = ''
  let sessionToken = ''

  for (const header of setCookieHeaders) {
    const parts = (header.split(';')[0] ?? '').split('=')
    const name = parts[0]?.trim() ?? ''
    const value = parts.slice(1).join('=').trim()
    if (name === 'xylona_session_id') sessionId = value
    if (name === 'xylona_session_token') sessionToken = value
  }

  return {
    sessionId,
    sessionToken,
    raw: `xylona_session_id=${sessionId}; xylona_session_token=${sessionToken}`,
  }
}

export async function apiLogin(username: string, password: string): Promise<ApiCookies> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/Login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify({ user_name: username, password }),
  })

  if (!resp.ok) {
    throw new Error(`Login failed for ${username}: ${resp.status} ${await resp.text()}`)
  }

  const setCookies = resp.headers.getSetCookie?.() ?? []
  return extractCookies(setCookies)
}

export async function apiCreateUser(
  cookies: ApiCookies,
  userDef: {
    username: string
    email: string
    password: string
    firstName: string
    lastName: string
    superUser?: boolean
  },
): Promise<TestUser> {
  const response = await createAPIClient(cookies).createUser(
    create(CreateUserRequestSchema, {
      userName: userDef.username,
      email: userDef.email,
      password: userDef.password,
      firstName: userDef.firstName,
      lastName: userDef.lastName,
      superUser: userDef.superUser ?? false,
    }),
  )

  if (!response.user?.id) {
    throw new Error(`CreateUser returned no user ID for ${userDef.username}`)
  }

  return {
    id: response.user.id,
    username: userDef.username,
    password: userDef.password,
    superUser: userDef.superUser ?? false,
  }
}

export async function apiAddGame(cookies: ApiCookies, game: Game): Promise<Game> {
  const response = await createAPIClient(cookies).addGame(create(AddGameRequestSchema, { game }))
  if (!response.game) {
    throw new Error('AddGame returned no game')
  }
  return response.game
}

export async function apiGetGame(cookies: ApiCookies, gameId: string): Promise<Game> {
  const response = await createAPIClient(cookies).getGame(
    create(GetGameRequestSchema, { id: gameId }),
  )
  if (!response.game) {
    throw new Error(`GetGame returned no game for ${gameId}`)
  }
  return response.game
}

export async function apiRemoveGame(cookies: ApiCookies, gameId: string): Promise<void> {
  await createAPIClient(cookies).removeGame(create(RemoveGameRequestSchema, { gameId }))
}

export async function apiCreateGameServer(
  cookies: ApiCookies,
  serverDef: {
    name: string
    gameId: string
    userId?: string
    directory: string
    nodeId?: string
    ip?: string
    port?: number
    queryPort?: number
    maxPlayers?: number
    setMaxPlayers?: number
  },
): Promise<string> {
  const request = create(CreateGameServerRequestSchema, {
    gameServer: create(GameServerSchema, {
      name: serverDef.name,
      gameId: serverDef.gameId,
      userId: serverDef.userId ?? '',
      directory: serverDef.directory,
      nodeId: serverDef.nodeId ?? '',
      ip: create(IPSchema, {
        address: serverDef.ip ?? '127.0.0.1',
        nodeId: serverDef.nodeId ?? '',
      }),
      port: BigInt(serverDef.port ?? 25565),
      queryPort: BigInt(serverDef.queryPort ?? (serverDef.port ?? 25565) + 1),
      maxPlayers: BigInt(serverDef.maxPlayers ?? serverDef.setMaxPlayers ?? 20),
      setMaxPlayers: BigInt(serverDef.setMaxPlayers ?? 20),
      maxMemoryMb: BigInt(1024),
    }),
  })

  const response = await createAPIClient(cookies).createGameServer(request)
  const id = response.gameServer?.id
  if (!id) throw new Error('CreateGameServer returned no server ID')
  return id
}

export async function apiStartGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  await createAPIClient(cookies).startGameServer({ serverId })
}

export async function apiStopGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  await createAPIClient(cookies).stopGameServer({ serverId })
}

export async function apiRemoveGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  await createAPIClient(cookies).removeGameServer({ serverId })
}

export async function apiCreateFile(
  cookies: ApiCookies,
  gameServerId: string,
  filePath: string,
  content: string,
): Promise<void> {
  await createAPIClient(cookies).gameServersFileOrDirectoryCreate(
    create(GameServerFileOrDirectoryCreateRequestSchema, {
      gameServerId,
      fullFilePath: filePath,
      content,
    }),
  )
}

export async function apiUpdateGameStartArgsTemplate(
  cookies: ApiCookies,
  input: {
    gameId: string
    platform: string
    startArgsTemplate: string
    baseCommand: string
    allowStartArgEditing: boolean
  },
): Promise<Game> {
  const response = await createAPIClient(cookies).updateGameStartArgsTemplate(
    create(UpdateGameStartArgsTemplateRequestSchema, input),
  )
  if (!response.game) {
    throw new Error(`UpdateGameStartArgsTemplate returned no game for ${input.gameId}`)
  }
  return response.game
}

export async function apiUpdateGameStartArgBlocklist(
  cookies: ApiCookies,
  gameId: string,
  startArgBlocklist: string,
): Promise<Game> {
  const response = await createAPIClient(cookies).updateGameStartArgBlocklist(
    create(UpdateGameStartArgBlocklistRequestSchema, {
      gameId,
      startArgBlocklist,
    }),
  )
  if (!response.game) {
    throw new Error(`UpdateGameStartArgBlocklist returned no game for ${gameId}`)
  }
  return response.game
}

export async function apiCreateBackup(
  cookies: ApiCookies,
  gameServerId: string,
  backupName: string,
) {
  const response = await createAPIClient(cookies).createGameServerBackup(
    create(CreateGameServerBackupRequestSchema, { gameServerId, backupName }),
  )
  if (!response.backup) {
    throw new Error('CreateGameServerBackup returned no backup')
  }
  return response.backup
}

export async function apiListBackups(cookies: ApiCookies, gameServerId: string) {
  const response = await createAPIClient(cookies).listGameServerBackups(
    create(ListGameServerBackupsRequestSchema, { gameServerId }),
  )
  return response.backups
}

export async function apiRestoreBackup(
  cookies: ApiCookies,
  gameServerId: string,
  backupId: string,
): Promise<void> {
  await createAPIClient(cookies).restoreGameServerBackup(
    create(RestoreGameServerBackupRequestSchema, {
      gameServerId,
      backupId,
      restoreMode: BackupRestoreMode.OVERWRITE,
    }),
  )
}

export async function apiDeleteBackup(
  cookies: ApiCookies,
  gameServerId: string,
  backupId: string,
): Promise<void> {
  await createAPIClient(cookies).deleteGameServerBackup(
    create(DeleteGameServerBackupRequestSchema, { gameServerId, backupId }),
  )
}
