import { Page } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'

export const BACKEND_URL = process.env['BACKEND_URL'] ?? 'http://localhost:8080'

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
  userData: {
    userName: string
    email: string
    password: string
    firstName: string
    lastName: string
    superUser: boolean
  },
): Promise<string> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/CreateUser`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      user_name: userData.userName,
      email: userData.email,
      password: userData.password,
      first_name: userData.firstName,
      last_name: userData.lastName,
      super_user: userData.superUser,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`CreateUser failed for ${userData.userName}: ${resp.status} ${body}`)
  }
  const data = (await resp.json()) as { user?: { id?: string } }
  const id = data.user?.id
  if (!id) throw new Error(`CreateUser returned no user ID for ${userData.userName}`)
  return id
}

export async function apiDeleteUser(cookies: ApiCookies, userId: string): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/DeleteUser`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({ id: userId }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`DeleteUser failed for ${userId}: ${resp.status} ${body}`)
  }
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

export async function apiGrantGameServerAccess(
  cookies: ApiCookies,
  gameServerId: string,
  userId: string,
  roleId: string,
): Promise<void> {
  const resp = await fetch(`${BACKEND_URL}/xylona.Xylona/GrantGameServerAccess`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
      Cookie: cookies.raw,
    },
    body: JSON.stringify({
      game_server_id: gameServerId,
      user_id: userId,
      role_id: roleId,
    }),
  })
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`GrantGameServerAccess failed: ${resp.status} ${body}`)
  }
}

export const AUTH_DIR = path.join(import.meta.dirname, '.auth')
export const TEST_USERS_FILE = path.join(AUTH_DIR, 'test-users.json')

export function storageStatePath(username: string): string {
  return path.join(AUTH_DIR, `${username}.json`)
}

export function saveTestUsers(users: TestUser[]): void {
  if (!fs.existsSync(AUTH_DIR)) fs.mkdirSync(AUTH_DIR, { recursive: true })
  fs.writeFileSync(TEST_USERS_FILE, JSON.stringify(users, null, 2))
}

export function loadTestUsers(): TestUser[] {
  if (!fs.existsSync(TEST_USERS_FILE)) return []
  return JSON.parse(fs.readFileSync(TEST_USERS_FILE, 'utf-8')) as TestUser[]
}

export async function loginAsUser(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/login')
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10_000 })
}
