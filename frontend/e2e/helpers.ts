import { create } from '@bufbuild/protobuf'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { expect, Page } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import {
  CreateGameServerRequestSchema,
  GameServerSchema,
  IPSchema,
  type Game,
} from '@/proto/shared_pb'
import {
  AddGameRequestSchema,
  CreateUserRequestSchema,
  EditGameRequestSchema,
  GetGameRequestSchema,
  RemoveGameRequestSchema,
  UpdateGameStartArgBlocklistRequestSchema,
  UpdateGameStartArgsTemplateRequestSchema,
  Xylona,
} from '@/proto/xylona_pb'

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

function createAPIClient(cookies: ApiCookies) {
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

export async function apiListGameServers(
  cookies: ApiCookies,
): Promise<Array<{ id: string; name: string }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/ListGameServers`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({}),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as { game_servers?: Array<{ id: string; name: string }> }
  return data.game_servers ?? []
}

export async function apiListGames(
  cookies: ApiCookies,
): Promise<Array<{ id: string; name: string }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/ListGames`, {
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

export async function apiCreateGameServer(
  cookies: ApiCookies,
  serverDef: {
    name: string
    gameId: string
    userId?: string
    directory: string
    ip?: string
    port?: number
    queryPort?: number
    setMaxPlayers?: number
  },
): Promise<string> {
  const request = create(CreateGameServerRequestSchema, {
    gameServer: create(GameServerSchema, {
      name: serverDef.name,
      gameId: serverDef.gameId,
      userId: serverDef.userId ?? '',
      directory: serverDef.directory,
      ip: create(IPSchema, { address: serverDef.ip ?? '0.0.0.0' }),
      port: BigInt(serverDef.port ?? 25565),
      queryPort: BigInt(serverDef.queryPort ?? (serverDef.port ?? 25565) + 1),
      setMaxPlayers: BigInt(serverDef.setMaxPlayers ?? 20),
    }),
  })
  const response = await createAPIClient(cookies).createGameServer(request)
  const id = response.gameServer?.id
  if (!id) throw new Error('CreateGameServer returned no server ID')
  return id
}

export async function apiAddGame(cookies: ApiCookies, game: Game): Promise<Game> {
  const response = await createAPIClient(cookies).addGame(
    create(AddGameRequestSchema, {
      game,
    }),
  )

  if (!response.game) {
    throw new Error('AddGame returned no game')
  }

  return response.game
}

export async function apiGetGame(cookies: ApiCookies, gameId: string): Promise<Game> {
  const response = await createAPIClient(cookies).getGame(
    create(GetGameRequestSchema, {
      id: gameId,
    }),
  )

  if (!response.game) {
    throw new Error(`GetGame returned no game for ${gameId}`)
  }

  return response.game
}

export async function apiEditGame(cookies: ApiCookies, game: Game): Promise<Game> {
  const response = await createAPIClient(cookies).editGame(
    create(EditGameRequestSchema, {
      gameId: game.id,
      game,
    }),
  )

  if (!response.game) {
    throw new Error(`EditGame returned no game for ${game.id}`)
  }

  return response.game
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
    create(UpdateGameStartArgsTemplateRequestSchema, {
      gameId: input.gameId,
      platform: input.platform,
      startArgsTemplate: input.startArgsTemplate,
      baseCommand: input.baseCommand,
      allowStartArgEditing: input.allowStartArgEditing,
    }),
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

export async function apiRemoveGame(cookies: ApiCookies, gameId: string): Promise<void> {
  await createAPIClient(cookies).removeGame(
    create(RemoveGameRequestSchema, {
      gameId,
    }),
  )
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

export async function apiStartGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/StartGameServer`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ server_id: serverId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`StartGameServer failed: ${resp.status} ${body}`)
  }
}

export const AUTH_DIR = path.join(import.meta.dirname, '.auth')

export interface TestState {
  gameServerId?: string
  gameId?: string
  gameName?: string
  noTrackerServerId?: string
}

const TEST_USERS_FILE = path.join(import.meta.dirname, '.auth', 'test-users.json')
const TEST_STATE_FILE = path.join(import.meta.dirname, '.auth', 'test-state.json')

export function loadTestUsers(): TestUser[] {
  if (!fs.existsSync(TEST_USERS_FILE)) return []
  return JSON.parse(fs.readFileSync(TEST_USERS_FILE, 'utf-8')) as TestUser[]
}

export function loadTestState(): TestState {
  if (!fs.existsSync(TEST_STATE_FILE)) return {}
  return JSON.parse(fs.readFileSync(TEST_STATE_FILE, 'utf-8')) as TestState
}

export async function apiStopGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/StopGameServer`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ server_id: serverId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`StopGameServer failed: ${resp.status} ${body}`)
  }
}

export async function apiRemoveGameServer(cookies: ApiCookies, serverId: string): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/RemoveGameServer`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ server_id: serverId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`RemoveGameServer failed: ${resp.status} ${body}`)
  }
}

export async function loginAsUser(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/login')
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 })
}

// ---------------------------------------------------------------------------
// Mod management API helpers
// ---------------------------------------------------------------------------

export async function apiSetServerVariant(
  cookies: ApiCookies,
  gameServerId: string,
  variantId: string,
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/SetServerVariant`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_server_id: gameServerId,
      variant_id: variantId,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`SetServerVariant failed: ${resp.status} ${body}`)
  }
}

export async function apiSearchMods(
  cookies: ApiCookies,
  gameServerId: string,
  query: string,
  source?: string,
): Promise<Array<{ source: string; source_id: string; name: string; is_installed: boolean }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/SearchMods`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_server_id: gameServerId,
      query,
      source: source ?? '',
      page: 0,
      page_size: 20,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`SearchMods failed: ${resp.status} ${body}`)
  }
  const data = (await resp.json()) as {
    results?: Array<{ source: string; source_id: string; name: string; is_installed: boolean }>
  }
  return data.results ?? []
}

export async function apiInstallMod(
  cookies: ApiCookies,
  gameServerId: string,
  source: string,
  sourceId: string,
  versionId: string,
): Promise<{ id: string }> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/InstallMod`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_server_id: gameServerId,
      source,
      source_id: sourceId,
      version_id: versionId,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`InstallMod failed: ${resp.status} ${body}`)
  }
  const data = (await resp.json()) as { installed_mod?: { id: string } }
  return { id: data.installed_mod?.id ?? '' }
}

export async function apiUninstallMod(
  cookies: ApiCookies,
  gameServerId: string,
  installedModId: string,
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/UninstallMod`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_server_id: gameServerId,
      installed_mod_id: installedModId,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`UninstallMod failed: ${resp.status} ${body}`)
  }
}

export async function apiListInstalledMods(
  cookies: ApiCookies,
  gameServerId: string,
): Promise<Array<{ id: string; mod_name: string; source: string; installed_version: string }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/ListInstalledMods`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ game_server_id: gameServerId }),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as {
    installed_mods?: Array<{
      id: string
      mod_name: string
      source: string
      installed_version: string
    }>
  }
  return data.installed_mods ?? []
}

export async function apiSetDummyUpdateFailure(
  cookies: ApiCookies,
  simulateFailure: boolean,
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/SetDummyUpdateFailure`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ simulate_failure: simulateFailure }),
  })
  if (!resp.ok) {
    throw new Error(`SetDummyUpdateFailure failed: ${resp.status}`)
  }
}

// ---------------------------------------------------------------------------
// Notification channel API helpers
// ---------------------------------------------------------------------------

export async function apiCreateNotificationChannel(
  cookies: ApiCookies,
  opts: {
    name: string
    channelType: number
    config: string
    enabled?: boolean
  },
): Promise<{ id: string; name: string }> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/CreateNotificationChannel`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      name: opts.name,
      channel_type: opts.channelType,
      config: opts.config,
      enabled: opts.enabled ?? true,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`CreateNotificationChannel failed: ${resp.status} ${body}`)
  }
  const data = (await resp.json()) as { channel?: { id: string; name: string } }
  return { id: data.channel?.id ?? '', name: data.channel?.name ?? '' }
}

export async function apiListNotificationChannels(
  cookies: ApiCookies,
): Promise<Array<{ id: string; name: string }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/ListNotificationChannels`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({}),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as { channels?: Array<{ id: string; name: string }> }
  return data.channels ?? []
}

export async function apiDeleteNotificationChannel(
  cookies: ApiCookies,
  channelId: string,
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/DeleteNotificationChannel`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ id: channelId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`DeleteNotificationChannel failed: ${resp.status} ${body}`)
  }
}

// ---------------------------------------------------------------------------
// Alert rule API helpers
// ---------------------------------------------------------------------------

export async function apiCreateAlertRule(
  cookies: ApiCookies,
  opts: {
    serverId: string
    serverNodeId: string
    eventType: number
    notificationChannelId: string
    condition?: string
    enabled?: boolean
  },
): Promise<{ id: string }> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/CreateAlertRule`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      server_id: opts.serverId,
      server_node_id: opts.serverNodeId,
      event_type: opts.eventType,
      notification_channel_id: opts.notificationChannelId,
      condition: opts.condition ?? '',
      enabled: opts.enabled ?? true,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`CreateAlertRule failed: ${resp.status} ${body}`)
  }
  const data = (await resp.json()) as { rule?: { id: string } }
  return { id: data.rule?.id ?? '' }
}

export async function apiListAlertRules(
  cookies: ApiCookies,
  serverId?: string,
  serverNodeId?: string,
): Promise<Array<{ id: string; event_type: number; notification_channel_id: string }>> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/ListAlertRules`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      server_id: serverId ?? '',
      server_node_id: serverNodeId ?? '',
    }),
  })
  if (!resp.ok) return []
  const data = (await resp.json()) as {
    rules?: Array<{ id: string; event_type: number; notification_channel_id: string }>
  }
  return data.rules ?? []
}

export async function apiDeleteAlertRule(cookies: ApiCookies, ruleId: string): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/DeleteAlertRule`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ id: ruleId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`DeleteAlertRule failed: ${resp.status} ${body}`)
  }
}
