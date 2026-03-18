import * as path from 'path'
import {
  apiLogin,
  apiCreateUser,
  apiListGameServers,
  apiGrantGameServerAccess,
  saveTestUsers,
  TestUser,
  BACKEND_URL,
} from './helpers'

// globalSetup runs in a separate context; load .env explicitly
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional — shell env vars or CI secrets take precedence
}

const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

const TEST_USER_DEFS = [
  {
    userName: 'e2e-superuser',
    email: 'e2e-superuser@test.local',
    password: 'TestPassword123!',
    firstName: 'E2E',
    lastName: 'Superuser',
    superUser: true,
  },
  {
    userName: 'e2e-operator',
    email: 'e2e-operator@test.local',
    password: 'TestPassword123!',
    firstName: 'E2E',
    lastName: 'Operator',
    superUser: false,
  },
  {
    userName: 'e2e-viewer',
    email: 'e2e-viewer@test.local',
    password: 'TestPassword123!',
    firstName: 'E2E',
    lastName: 'Viewer',
    superUser: false,
  },
  {
    userName: 'e2e-noaccess',
    email: 'e2e-noaccess@test.local',
    password: 'TestPassword123!',
    firstName: 'E2E',
    lastName: 'NoAccess',
    superUser: false,
  },
]

export default async function globalSetup(): Promise<void> {
  console.log(`[E2E Setup] Connecting to backend at ${BACKEND_URL}`)
  console.log(`[E2E Setup] Logging in as admin: ${ADMIN_USERNAME}`)

  const adminCookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)

  const createdUsers: TestUser[] = []

  for (const userDef of TEST_USER_DEFS) {
    try {
      const id = await apiCreateUser(adminCookies, userDef)
      createdUsers.push({
        id,
        username: userDef.userName,
        password: userDef.password,
        superUser: userDef.superUser,
      })
      console.log(`[E2E Setup] Created user: ${userDef.userName} (id: ${id})`)
    } catch (err) {
      console.warn(`[E2E Setup] Warning: could not create user ${userDef.userName}: ${err}`)
    }
  }

  saveTestUsers(createdUsers)

  // Optionally assign RBAC roles if any game servers exist
  const gameServers = await apiListGameServers(adminCookies)
  if (gameServers.length > 0) {
    const firstServer = gameServers[0]!
    console.log(`[E2E Setup] Assigning RBAC roles on game server: ${firstServer.name}`)

    for (const user of createdUsers) {
      if (user.superUser) continue

      let roleId: string
      if (user.username === 'e2e-operator') {
        roleId = 'operator'
      } else if (user.username === 'e2e-viewer') {
        roleId = 'viewer'
      } else {
        continue // e2e-noaccess gets no role
      }

      try {
        await apiGrantGameServerAccess(adminCookies, firstServer.id, user.id, roleId)
        console.log(`[E2E Setup] Granted ${roleId} on ${firstServer.name} to ${user.username}`)
      } catch (err) {
        console.warn(`[E2E Setup] Warning: could not grant access to ${user.username}: ${err}`)
      }
    }
  } else {
    console.log('[E2E Setup] No game servers found; skipping RBAC role assignments')
  }

  console.log('[E2E Setup] Setup complete')
}
