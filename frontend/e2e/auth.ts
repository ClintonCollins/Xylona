import { expect, type Browser, type Page } from '@playwright/test'
import * as path from 'path'

import { AUTH_DIR, type TestUser } from './fixtures'

export async function loginAsUser(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/login')
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 })
}

export function storageStatePath(file: string): string {
  return path.join(AUTH_DIR, file)
}

export async function openUserPage(browser: Browser, user: TestUser) {
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()
  await loginAsUser(page, user.username, user.password)
  return { context, page }
}
