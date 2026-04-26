import { expect, type Page } from '@playwright/test'

export async function expectAppShell(page: Page): Promise<void> {
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.locator('body')).not.toContainText('404')
}

export async function gotoAppPage(page: Page, url: string): Promise<void> {
  await page.goto(url)
  await expect(page).not.toHaveURL(/\/login/)
  await expectAppShell(page)
}

export function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      errors.push(msg.text())
    }
  })
  page.on('pageerror', (err) => {
    errors.push(err.message)
  })
  return errors
}
