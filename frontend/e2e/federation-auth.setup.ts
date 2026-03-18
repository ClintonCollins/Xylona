import { test as setup, expect } from '@playwright/test'
import * as path from 'path'
import { loginAsUser, AUTH_DIR } from './helpers'

const USERS = [
  { username: 'e2e-superuser', file: 'federation-superuser.json' },
  { username: 'e2e-operator', file: 'federation-operator.json' },
  { username: 'e2e-viewer', file: 'federation-viewer.json' },
  { username: 'e2e-noaccess', file: 'federation-noaccess.json' },
]

for (const { username, file } of USERS) {
  setup(`authenticate as ${username} (federation)`, async ({ page }) => {
    await loginAsUser(page, username, 'TestPassword123!')
    await expect(page).not.toHaveURL(/\/login/)
    await page.context().storageState({ path: path.join(AUTH_DIR, file) })
  })
}
