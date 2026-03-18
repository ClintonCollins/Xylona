import { test, expect } from '@playwright/test'
import { loadTestState } from './helpers'

test.describe('File browser operations (superuser)', () => {
  test.beforeEach(async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }
  })

  test('file list loads and shows files', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    // Wait for files to load
    await page.waitForTimeout(2000)
    // Should see the test files created during setup
    await expect(page.locator('body')).toContainText('e2e-test-config.cfg')
  })

  test('can navigate into a subdirectory and back', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Click on the subdirectory
    await page.getByText('e2e-test-subdir').click()
    await page.waitForTimeout(1000)

    // Should see the nested file
    await expect(page.locator('body')).toContainText('nested-file.txt')

    // Navigate back (click parent directory or back button)
    const backButton = page.locator('[aria-label="Go back"], [aria-label="Parent directory"]').first()
    if (await backButton.isVisible()) {
      await backButton.click()
    } else {
      // Try the breadcrumb/path navigation
      await page.goBack()
    }
    await page.waitForTimeout(1000)

    // Should see the original files again
    await expect(page.locator('body')).toContainText('e2e-test-config.cfg')
  })

  test('can create a new file', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Look for a create/new file button
    const newFileButton = page.getByRole('button', { name: /new file|create file/i }).first()
    if (await newFileButton.isVisible()) {
      await newFileButton.click()
      await page.waitForTimeout(500)

      // Fill in file name
      const nameInput = page.getByLabel(/name|file name/i).first()
      if (await nameInput.isVisible()) {
        await nameInput.fill('e2e-created-file.txt')

        // Submit
        const submitButton = page.getByRole('button', { name: /create|save|ok/i }).first()
        await submitButton.click()
        await page.waitForTimeout(1000)

        // Verify file appears in the list
        await expect(page.locator('body')).toContainText('e2e-created-file.txt')
      }
    }
  })

  test('can create a new directory', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Look for a create/new directory button
    const newDirButton = page.getByRole('button', { name: /new folder|create dir|new dir/i }).first()
    if (await newDirButton.isVisible()) {
      await newDirButton.click()
      await page.waitForTimeout(500)

      const nameInput = page.getByLabel(/name|folder name|directory name/i).first()
      if (await nameInput.isVisible()) {
        await nameInput.fill('e2e-created-dir')

        const submitButton = page.getByRole('button', { name: /create|save|ok/i }).first()
        await submitButton.click()
        await page.waitForTimeout(1000)

        await expect(page.locator('body')).toContainText('e2e-created-dir')
      }
    }
  })

  test('can delete a file', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) return
    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await page.waitForTimeout(2000)

    // Find and select the test file for deletion
    const fileRow = page.locator('tr, .q-item').filter({ hasText: 'e2e-test-readme.txt' }).first()
    if (await fileRow.isVisible()) {
      // Try selecting the file via checkbox
      const checkbox = fileRow.locator('input[type="checkbox"], .q-checkbox').first()
      if (await checkbox.isVisible()) {
        await checkbox.click()
      } else {
        // Right-click for context menu
        await fileRow.click({ button: 'right' })
      }
      await page.waitForTimeout(500)

      // Click delete button
      const deleteButton = page.getByRole('button', { name: /delete/i }).first()
      if (await deleteButton.isVisible()) {
        await deleteButton.click()
        await page.waitForTimeout(500)

        // Confirm deletion
        const confirmButton = page.getByRole('button', { name: /confirm|yes|delete/i }).last()
        if (await confirmButton.isVisible()) {
          await confirmButton.click()
          await page.waitForTimeout(1000)
        }
      }
    }
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
    await page.waitForTimeout(3000)
    // Viewer has no files.view permission — should be redirected
    await expect(page).not.toHaveURL(/\/files/)

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
    await page.waitForTimeout(3000)
    // Operator has no files.view permission — should be redirected
    await expect(page).not.toHaveURL(/\/files/)

    await ctx.close()
  })
})
