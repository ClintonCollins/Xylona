import * as path from 'path'
import { apiLogin, apiDeleteUser, loadTestUsers, BACKEND_URL } from './helpers'

// globalTeardown runs in a separate context; load .env explicitly
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional — shell env vars or CI secrets take precedence
}

const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

export default async function globalTeardown(): Promise<void> {
  console.log(`[E2E Teardown] Connecting to backend at ${BACKEND_URL}`)

  const users = loadTestUsers()
  if (users.length === 0) {
    console.log('[E2E Teardown] No test users to clean up')
    return
  }

  let adminCookies
  try {
    adminCookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
  } catch (err) {
    console.warn(`[E2E Teardown] Warning: could not log in as admin, skipping cleanup: ${err}`)
    return
  }

  for (const user of users) {
    try {
      await apiDeleteUser(adminCookies, user.id)
      console.log(`[E2E Teardown] Deleted user: ${user.username} (id: ${user.id})`)
    } catch (err) {
      console.warn(`[E2E Teardown] Warning: could not delete user ${user.username}: ${err}`)
    }
  }

  console.log('[E2E Teardown] Teardown complete')
}
