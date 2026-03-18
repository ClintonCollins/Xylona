import { test, expect } from '@playwright/test'

test.describe('Admin page access control', () => {
  test('superuser can access /admin/users', async ({ page }) => {
    // Default auth state is e2e-superuser (set in playwright.config.ts)
    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page).toHaveURL(/\/admin\/users/)
  })

  test('non-superuser (operator) is redirected away from /admin/users', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-operator.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/admin/users')
    await expect(page).not.toHaveURL(/\/admin\/users/, { timeout: 5_000 })

    await ctx.close()
  })

  test('non-superuser (viewer) cannot access user create page', async ({ browser }) => {
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-viewer.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto('/admin/users/create')
    await expect(page).not.toHaveURL(/\/admin\/users\/create/, { timeout: 5_000 })

    await ctx.close()
  })
})
