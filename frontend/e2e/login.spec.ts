import { test, expect } from '@playwright/test'

// These tests run without any stored auth state
test.use({ storageState: { cookies: [], origins: [] } })

test.describe('Login page', () => {
  test('renders the login form', async ({ page }) => {
    await page.goto('/login')

    await expect(page.locator('.login-brand-name')).toHaveText('Xylona')
    await expect(page.locator('.login-brand-tagline')).toContainText(/Game Server\s*Control Panel/i)
    await expect(page.getByLabel('Username')).toBeVisible()
    await expect(page.getByLabel('Password')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
  })

  test('redirects unauthenticated users to /login', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveURL(/\/login/)
  })

  test('redirects to dashboard on successful login', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('Username').fill('e2e-superuser')
    await page.getByLabel('Password').fill('TestPassword123!')
    await page.getByRole('button', { name: 'Sign in' }).click()

    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 })
  })

  test('shows error on invalid credentials', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('Username').fill('nonexistent-user')
    await page.getByLabel('Password').fill('wrongpassword')
    await page.getByRole('button', { name: 'Sign in' }).click()

    // Wait for the login RPC to complete and an error notification to appear
    await expect(page.locator('.q-notification')).toBeVisible({ timeout: 5_000 })
    // Should still be on the login page
    await expect(page).toHaveURL(/\/login/)
  })
})
