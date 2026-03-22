import { test, expect, type Page } from '@playwright/test'
import { loadTestState } from './helpers'

async function gotoFilesPage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/files`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.locator('#file-list')).toBeVisible({ timeout: 10_000 })
}

test.describe('File browser operations (superuser)', () => {
  test.beforeEach(async ({ page: _page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
  })

  test('file list loads and shows files', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await gotoFilesPage(page, state.gameServerId)
    // Should see the test files created during setup
    await expect(page.locator('body')).toContainText('e2e-test-config.cfg', { timeout: 10_000 })
  })

  test('can navigate into a subdirectory and back', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await gotoFilesPage(page, state.gameServerId)

    // Click on the subdirectory
    await page.getByText('e2e-test-subdir').click()
    await expect(page.getByLabel('File path')).toHaveValue(/e2e-test-subdir/, { timeout: 10_000 })

    // Should see the nested file
    await expect(page.locator('body')).toContainText('nested-file.txt', { timeout: 10_000 })

    // Navigate back (click parent directory or back button)
    const backButton = page.getByRole('button', { name: /^\.\.$/ })
    await expect(backButton).toBeVisible({ timeout: 10_000 })
    await backButton.click()
    await expect(page.getByLabel('File path')).not.toHaveValue(/e2e-test-subdir/, {
      timeout: 10_000,
    })

    // Should see the original files again
    await expect(page.locator('body')).toContainText('e2e-test-config.cfg', { timeout: 10_000 })
  })

  test('can create a new file', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await gotoFilesPage(page, state.gameServerId)
    const fileName = `e2e-created-file-${Date.now()}.txt`

    const createButton = page.getByRole('button', { name: 'Create' }).first()
    await expect(createButton).toBeVisible({ timeout: 5_000 })
    await createButton.click()

    const dialog = page
      .locator('.q-dialog')
      .filter({ hasText: 'Create new file or directory' })
      .first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    const nameInput = page.getByLabel('Name')
    await expect(nameInput).toBeVisible()
    await nameInput.fill(fileName)
    await dialog.getByRole('button', { name: 'Submit' }).click()

    await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    await expect(page.locator(`[data-file-name="${fileName}"]`)).toBeVisible({ timeout: 10_000 })
  })

  test('can create a new directory', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await gotoFilesPage(page, state.gameServerId)
    const dirName = `e2e-created-dir-${Date.now()}`

    const createButton = page.getByRole('button', { name: 'Create' }).first()
    await expect(createButton).toBeVisible({ timeout: 5_000 })
    await createButton.click()

    const dialog = page
      .locator('.q-dialog')
      .filter({ hasText: 'Create new file or directory' })
      .first()
    await expect(dialog).toBeVisible({ timeout: 5_000 })
    const typeSelect = dialog.getByLabel('File or Directory')
    await typeSelect.click()
    const optionsMenu = page.locator('.q-menu').last()
    await expect(optionsMenu).toBeVisible({ timeout: 5_000 })
    await optionsMenu.getByText('Directory', { exact: true }).click()

    const nameInput = dialog.getByLabel('Name')
    await nameInput.fill(dirName)
    await dialog.getByRole('button', { name: 'Submit' }).click()

    await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    await expect(page.getByRole('button', { name: dirName })).toBeVisible({ timeout: 10_000 })
  })

  test('can delete a file', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await gotoFilesPage(page, state.gameServerId)

    // Find and select the test file for deletion
    const fileRow = page.locator('[data-file-name="e2e-test-readme.txt"]').first()
    await expect(fileRow).toBeVisible({ timeout: 10_000 })
    const checkbox = fileRow.getByRole('checkbox')
    await expect(checkbox).toBeVisible()
    await checkbox.click()

    const deleteButton = page.getByRole('button', { name: /^Delete$/i }).first()
    await expect(deleteButton).toBeVisible({ timeout: 5_000 })
    await deleteButton.click()

    const deleteDialog = page.locator('.q-dialog').filter({ hasText: 'Delete Files' }).first()
    await expect(deleteDialog).toBeVisible({ timeout: 5_000 })
    const confirmButton = deleteDialog.getByRole('button', { name: 'Delete' })
    await expect(confirmButton).toBeVisible({ timeout: 5_000 })
    await confirmButton.click()

    await expect(page.locator('[data-file-name="e2e-test-readme.txt"]')).toHaveCount(0, {
      timeout: 10_000,
    })
  })

  test('viewer cannot access file browser (no files.view permission)', async ({ browser }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-viewer.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto(`/game-servers/${state.gameServerId}/files`)
    // Viewer has no files.view permission — should be redirected
    await expect(page).not.toHaveURL(/\/files/, { timeout: 10_000 })

    await ctx.close()
  })

  test('operator cannot access file browser (no files.view permission)', async ({ browser }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
    const ctx = await browser.newContext({
      storageState: './e2e/.auth/e2e-operator.json',
      ignoreHTTPSErrors: true,
    })
    const page = await ctx.newPage()

    await page.goto(`/game-servers/${state.gameServerId}/files`)
    // Operator has no files.view permission — should be redirected
    await expect(page).not.toHaveURL(/\/files/, { timeout: 10_000 })

    await ctx.close()
  })
})
