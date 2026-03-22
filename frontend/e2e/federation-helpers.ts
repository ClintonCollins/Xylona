import * as fs from 'fs'
import * as path from 'path'
import { ApiCookies } from './helpers'

export const NODE_A_BACKEND = 'http://localhost:9081'
export const NODE_B_BACKEND = 'http://localhost:9082'

export interface FederationTestState {
  nodeAUrl: string
  nodeBUrl: string
  nodeAId?: string
  nodeBId?: string
  pairedNodeIdOnA?: string
  pairedNodeIdOnB?: string
  gameServerId?: string
  gameId?: string
  testUsers?: Array<{ id: string; username: string; superUser: boolean }>
}

const FEDERATION_STATE_DIR = path.join(import.meta.dirname, '.federation')
const FEDERATION_STATE_FILE = path.join(FEDERATION_STATE_DIR, 'state.json')

export function loadFederationState(): FederationTestState {
  if (!fs.existsSync(FEDERATION_STATE_FILE)) {
    return { nodeAUrl: NODE_A_BACKEND, nodeBUrl: NODE_B_BACKEND }
  }
  return JSON.parse(fs.readFileSync(FEDERATION_STATE_FILE, 'utf-8')) as FederationTestState
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

export interface FedLoginResult {
  cookies: ApiCookies
  userId: string
}

export async function fedApiLogin(
  username: string,
  password: string,
  backendUrl: string,
): Promise<FedLoginResult> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/Login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify({ userName: username, password }),
  })
  if (!resp.ok) {
    throw new Error(
      `Login failed for ${username} at ${backendUrl}: ${resp.status} ${await resp.text()}`,
    )
  }
  const data = (await resp.json()) as { user?: { id?: string } }
  const setCookies = resp.headers.getSetCookie?.() ?? []
  return {
    cookies: extractCookies(setCookies),
    userId: data.user?.id ?? '',
  }
}

export async function fedApiListGames(
  cookies: ApiCookies,
  backendUrl: string,
): Promise<Array<{ id: string; name: string }>> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/ListGames`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({}),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as { games?: Array<{ id: string; name: string }> }
  return data.games ?? []
}

export async function fedApiCreateGameServer(
  cookies: ApiCookies,
  serverDef: {
    name: string
    gameId: string
    userId: string
    startCommand: string
    directory: string
    ip?: string
    port?: number
  },
  backendUrl: string,
): Promise<string> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/CreateGameServer`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      gameServer: {
        name: serverDef.name,
        gameId: serverDef.gameId,
        userId: serverDef.userId,
        startCommand: serverDef.startCommand,
        directory: serverDef.directory,
        ip: { address: serverDef.ip ?? '' },
        port: serverDef.port ?? 25599,
        queryPort: (serverDef.port ?? 25599) + 1,
        setMaxPlayers: 20,
      },
    }),
  })
  if (!resp.ok) {
    throw new Error(`CreateGameServer failed: ${resp.status} ${await resp.text()}`)
  }
  const data = (await resp.json()) as { gameServer?: { id?: string } }
  return data.gameServer?.id ?? ''
}

export async function fedApiRemoveGameServer(
  cookies: ApiCookies,
  serverId: string,
  backendUrl: string,
): Promise<void> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/RemoveGameServer`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ serverId }),
  })
  if (!resp.ok) {
    throw new Error(`RemoveGameServer failed: ${resp.status} ${await resp.text()}`)
  }
}

export async function fedApiListGameServers(
  cookies: ApiCookies,
  backendUrl: string,
): Promise<Array<{ id: string; name: string }>> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/ListGameServers`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({}),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as { gameServers?: Array<{ id: string; name: string }> }
  return data.gameServers ?? []
}

export async function fedApiListAggregatedGameServers(
  cookies: ApiCookies,
  backendUrl: string,
): Promise<
  Array<{
    isLocal: boolean
    localServer?: { id: string; name: string }
    remoteServer?: { id: string; displayName: string; remoteServerId: string; nodeName: string }
  }>
> {
  const resp = await fetch(`${backendUrl}/xylona.Xylona/ListAggregatedGameServers`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({}),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as {
    servers?: Array<{
      isLocal: boolean
      localServer?: { id: string; name: string }
      remoteServer?: { id: string; displayName: string; remoteServerId: string; nodeName: string }
    }>
  }
  return data.servers ?? []
}

/**
 * Wait for a condition to become true, polling at the given interval.
 */
export async function waitForCondition(
  condition: () => Promise<boolean>,
  timeoutMs: number = 30_000,
  intervalMs: number = 1000,
): Promise<void> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    if (await condition()) return
    await new Promise((resolve) => setTimeout(resolve, intervalMs))
  }
  throw new Error(`Condition not met within ${timeoutMs}ms`)
}
