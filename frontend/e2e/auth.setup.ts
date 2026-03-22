import { test as setup, expect } from '@playwright/test'
import * as path from 'path'
import { loginAsUser, AUTH_DIR } from './helpers'

// Save auth state for all test users so permission tests can use pre-saved
// storage states via browser.newContext({ storageState }) instead of logging
// in repeatedly during each test.
const USERS = [
  { username: 'e2e-superuser', file: 'user.json' }, // default context for most tests
  { username: 'e2e-operator', file: 'e2e-operator.json' },
  { username: 'e2e-viewer', file: 'e2e-viewer.json' },
  { username: 'e2e-noaccess', file: 'e2e-noaccess.json' },
]

for (const { username, file } of USERS) {
  setup(`authenticate as ${username}`, async ({ page }) => {
    await loginAsUser(page, username, 'TestPassword123!')
    await expect(page).not.toHaveURL(/\/login/)
    await page.context().storageState({ path: path.join(AUTH_DIR, file) })
  })
}
