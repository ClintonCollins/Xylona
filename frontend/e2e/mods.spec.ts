import { test, expect, type Page } from '@playwright/test'
import {
  loadTestState,
  apiInstallMod,
  apiListInstalledMods,
  apiLogin,
  apiSearchMods,
  apiSetServerVariant,
  apiUninstallMod,
} from './helpers'

const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

async function gotoConsolePage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/console`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByText('Variant', { exact: true })).toBeVisible({ timeout: 10_000 })
}

async function gotoModsPage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/mods`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByRole('tab', { name: /Installed/i })).toBeVisible({ timeout: 10_000 })
}

async function openVariantDialog(page: Page) {
  const changeButton = page.getByRole('button', { name: 'Change' }).first()
  await expect(changeButton).toBeVisible({ timeout: 5_000 })
  await changeButton.click()

  const dialog = page.locator('.q-dialog').filter({ hasText: 'Change Variant' }).first()
  await expect(dialog).toBeVisible({ timeout: 5_000 })

  return dialog
}

async function openVariantMenu(page: Page) {
  const dialog = await openVariantDialog(page)
  const variantSelect = dialog.getByLabel('Variant').first()
  await expect(variantSelect).toBeVisible({ timeout: 5_000 })
  await variantSelect.click()

  const menu = page.locator('.q-menu').last()
  await expect(menu).toBeVisible({ timeout: 5_000 })

  return { dialog, menu }
}

async function selectVariant(page: Page, variantName: string) {
  const { menu } = await openVariantMenu(page)
  await menu.getByText(variantName, { exact: true }).click()
}

async function applyVariantChange(page: Page) {
  const dialog = page.locator('.q-dialog').filter({ hasText: 'Change Variant' }).first()
  await expect(dialog).toBeVisible({ timeout: 5_000 })
  await dialog.getByRole('button', { name: 'Apply' }).click()
  await expect(dialog).not.toBeVisible({ timeout: 10_000 })
}

async function openBrowseTab(page: Page) {
  await page.getByRole('tab', { name: /Browse/i }).click()
  await expect(page.getByPlaceholder('Search mods...')).toBeVisible({ timeout: 10_000 })
}

/**
 * E2E tests for variant selection and mod management.
 *
 * Prerequisites (seeded by the E2E orchestrator):
 * - "E2E Test Game" with typed Minecraft variants (Vanilla, Paper, Purpur, Fabric)
 * - "E2E Test Server" with the Paper variant selected
 * - Backend running with mod providers registered
 *
 * Tests are sequential (workers: 1) and build on shared state.
 * Each describe block cleans up after itself.
 */

// ============================================================================
// Variant Selection
// ============================================================================
test.describe('Variant Selection', () => {
  test('console page shows Variant section', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    await expect(page.getByText('Variant', { exact: true })).toBeVisible({ timeout: 5_000 })
  })

  test('change dialog lists Paper, Purpur, Fabric, Vanilla', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    const { dialog, menu } = await openVariantMenu(page)

    await expect(menu.getByText('Paper')).toBeVisible()
    await expect(menu.getByText('Purpur')).toBeVisible()
    await expect(menu.getByText('Fabric')).toBeVisible()
    await expect(menu.getByText('Vanilla')).toBeVisible()

    await page.keyboard.press('Escape')
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).not.toBeVisible({ timeout: 5_000 })
  })

  test('change dialog shows the current variant and switch warning', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    const dialog = await openVariantDialog(page)

    await expect(dialog).toContainText('Currently active')
    await expect(dialog).toContainText('Paper')
    await expect(dialog).toContainText('Switching variants may change update behavior')

    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).not.toBeVisible({ timeout: 5_000 })
  })

  test('apply Purpur and keep it selected after reload', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    await selectVariant(page, 'Purpur')
    await applyVariantChange(page)

    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.software-card .software-name').first()).toContainText('Purpur', {
      timeout: 5_000,
    })
  })

  test('apply Fabric and keep it selected after reload', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    await selectVariant(page, 'Fabric')
    await applyVariantChange(page)

    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.software-card .software-name').first()).toContainText('Fabric', {
      timeout: 5_000,
    })
  })

  test('restore Paper for subsequent tests', async () => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerVariant(cookies, state.gameServerId, 'paper')
  })
})

// ============================================================================
// Mods Tab Visibility
// ============================================================================
test.describe('Mods Tab Visibility', () => {
  test('Mods tab visible when variant is Paper', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    await expect(page.getByRole('tab', { name: /Mods/i })).toBeVisible({ timeout: 5_000 })
  })

  test('Mods tab hidden when variant is Vanilla', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerVariant(cookies, state.gameServerId, 'vanilla')

    await gotoConsolePage(page, state.gameServerId)

    await expect(page.getByRole('tab', { name: /Mods/i })).not.toBeVisible({ timeout: 3_000 })

    await apiSetServerVariant(cookies, state.gameServerId, 'paper')
  })

  test('Mods tab returns when switching back to Paper', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    await expect(page.getByRole('tab', { name: /Mods/i })).toBeVisible({ timeout: 5_000 })
  })
})

// ============================================================================
// Mod Browsing - Paper
// ============================================================================
test.describe('Mod Browsing - Paper', () => {
  test('mods page loads with Installed and Browse tabs', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await expect(page.getByRole('tab', { name: /Installed/i })).toBeVisible()
    await expect(page.getByRole('tab', { name: /Browse/i })).toBeVisible()
  })

  test('Browse tab shows search input and empty prompt', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)

    await expect(page.getByPlaceholder('Search mods...')).toBeVisible()
    await expect(page.locator('body')).toContainText(/search for mods/i)
  })

  test('search "WorldEdit" returns results from Modrinth', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')

    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    const cards = page.locator('.mod-card')
    expect(await cards.count()).toBeGreaterThan(0)

    const firstName = await cards.first().locator('.mod-card-name').textContent()
    expect(firstName?.toLowerCase()).toContain('worldedit')
  })
})

// ============================================================================
// Mod Install Flow - Paper
// ============================================================================
test.describe('Mod Install Flow - Paper', () => {
  test.afterAll(async () => {
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const mods = await apiListInstalledMods(cookies, state.gameServerId)
    for (const mod of mods) {
      await apiUninstallMod(cookies, state.gameServerId, mod.id)
    }
  })

  test('install a mod from browse results', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')

    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    const installableCard = page
      .locator('.mod-card')
      .filter({ hasNot: page.locator('.installed-badge') })
      .first()
    const modName = (await installableCard.locator('.mod-card-name').textContent())?.trim() ?? ''
    const installBtn = installableCard.getByRole('button', { name: /Install/i })
    await expect(installBtn).toBeVisible({ timeout: 5_000 })
    await installBtn.click()

    const installDialog = page.locator('.q-dialog').filter({ hasText: 'Install Mod' }).first()
    if (await installDialog.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await installDialog.getByRole('button', { name: 'Install' }).click()
      await expect(installDialog).not.toBeVisible({ timeout: 10_000 })
    }

    if (modName !== '') {
      const installedCard = page.locator('.mod-card').filter({ hasText: modName }).first()
      await expect(installedCard.locator('.installed-badge')).toBeVisible({ timeout: 30_000 })
    }

    await page.getByRole('tab', { name: /Installed/i }).click()

    const installedRows = page.locator('.mod-row')
    await expect(installedRows.first()).toBeVisible({ timeout: 10_000 })
    if (modName !== '') {
      await expect(installedRows.filter({ hasText: modName }).first()).toBeVisible({
        timeout: 10_000,
      })
    }
  })

  test('installed mod shows in Browse as "Installed"', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    await expect(page.locator('.installed-badge').first()).toBeVisible({ timeout: 5_000 })
  })
})

// ============================================================================
// Installed Mod Management
// ============================================================================
test.describe('Installed Mod Management', () => {
  test.beforeAll(async () => {
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)

    const results = await apiSearchMods(cookies, state.gameServerId, 'WorldEdit')
    if (results.length === 0) {
      return
    }

    const mod = results[0]
    if (!mod) {
      throw new Error('No mod search results')
    }

    try {
      const installedMods = await apiListInstalledMods(cookies, state.gameServerId)
      const alreadyInstalled = installedMods.some(
        (installedMod) => installedMod.source === mod.source,
      )
      if (!alreadyInstalled) {
        await apiInstallMod(cookies, state.gameServerId, mod.source, mod.source_id, '')
      }
    } catch {
      // Best effort - tests will skip if no mod is installed.
    }
  })

  test.afterAll(async () => {
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const mods = await apiListInstalledMods(cookies, state.gameServerId)
    for (const mod of mods) {
      await apiUninstallMod(cookies, state.gameServerId, mod.id)
    }
  })

  test('installed tab shows mod with name, version, source', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    const firstRow = page.locator('.mod-row').first()
    if (!(await firstRow.isVisible().catch(() => false))) {
      test.skip(true, 'No mods installed to test')
      return
    }

    await expect(firstRow.locator('.mod-name')).toBeVisible()
    await expect(firstRow.locator('.version-text')).toBeVisible()
    await expect(firstRow.locator('.source-badge')).toBeVisible()
  })

  test('toggle auto-update persists across reload', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    const firstRow = page.locator('.mod-row').first()
    if (!(await firstRow.isVisible().catch(() => false))) {
      test.skip(true, 'No mods installed to test')
      return
    }

    const toggle = firstRow.locator('.q-toggle')
    await expect(toggle).toBeVisible()

    const checkbox = toggle.locator('input[type="checkbox"]')
    const wasChecked = await checkbox.isChecked()

    await toggle.click()
    if (wasChecked) {
      await expect(checkbox).not.toBeChecked({ timeout: 10_000 })
    } else {
      await expect(checkbox).toBeChecked({ timeout: 10_000 })
    }

    const nowChecked = await checkbox.isChecked()
    expect(nowChecked).not.toBe(wasChecked)

    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.mod-row').first()).toBeVisible({ timeout: 10_000 })

    const reloadedToggle = page
      .locator('.mod-row')
      .first()
      .locator('.q-toggle input[type="checkbox"]')
    const afterReload = await reloadedToggle.isChecked()
    expect(afterReload).toBe(nowChecked)
  })

  test('uninstall mod via actions menu', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    const firstRow = page.locator('.mod-row').first()
    if (!(await firstRow.isVisible().catch(() => false))) {
      test.skip(true, 'No mods installed to test')
      return
    }

    const modName = await firstRow.locator('.mod-name').textContent()
    const initialCount = await page.locator('.mod-row').count()

    await firstRow.locator('[aria-label*="Actions"]').click()

    const uninstallAction = page.getByText('Uninstall', { exact: true })
    await expect(uninstallAction).toBeVisible({ timeout: 5_000 })
    await uninstallAction.click()

    const dialog = page
      .locator('.q-dialog')
      .filter({ hasText: /uninstall/i })
      .first()
    if (await dialog.isVisible().catch(() => false)) {
      await dialog.getByRole('button', { name: /Uninstall|OK|Confirm/i }).click()
      await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    }

    if (modName) {
      await expect(page.locator('.mod-name').filter({ hasText: modName })).toHaveCount(0, {
        timeout: 10_000,
      })
    }

    await expect
      .poll(async () => page.locator('.mod-row').count(), { timeout: 10_000 })
      .toBeLessThan(initialCount)
    const finalCount = await page.locator('.mod-row').count()
    expect(finalCount).toBeLessThan(initialCount)
  })
})

// ============================================================================
// Fabric Mod Flow
// ============================================================================
test.describe('Fabric Mod Flow', () => {
  test.afterAll(async () => {
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const mods = await apiListInstalledMods(cookies, state.gameServerId)
    for (const mod of mods) {
      await apiUninstallMod(cookies, state.gameServerId, mod.id)
    }
    await apiSetServerVariant(cookies, state.gameServerId, 'paper')
  })

  test('switch to Fabric via API and verify Mods tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerVariant(cookies, state.gameServerId, 'fabric')

    await gotoModsPage(page, state.gameServerId)

    await expect(page.getByRole('tab', { name: /Installed/i })).toBeVisible()
    await expect(page.getByRole('tab', { name: /Browse/i })).toBeVisible()
  })

  test('search "Sodium" on Browse tab returns Fabric mods', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('Sodium')

    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    const cards = page.locator('.mod-card')
    expect(await cards.count()).toBeGreaterThan(0)

    const firstName = await cards.first().locator('.mod-card-name').textContent()
    expect(firstName?.toLowerCase()).toContain('sodium')
  })

  test('install Sodium and show it in Installed tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('Sodium')
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    const installableCard = page
      .locator('.mod-card')
      .filter({ hasNot: page.locator('.installed-badge') })
      .first()
    const modName = (await installableCard.locator('.mod-card-name').textContent())?.trim() ?? ''
    const installBtn = installableCard.getByRole('button', { name: /Install/i })
    await expect(installBtn).toBeVisible({ timeout: 5_000 })
    await installBtn.click()

    const installDialog = page.locator('.q-dialog').filter({ hasText: 'Install Mod' }).first()
    if (await installDialog.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await installDialog.getByRole('button', { name: 'Install' }).click()
      await expect(installDialog).not.toBeVisible({ timeout: 10_000 })
    }

    if (modName !== '') {
      const installedCard = page.locator('.mod-card').filter({ hasText: modName }).first()
      await expect(installedCard.locator('.installed-badge')).toBeVisible({ timeout: 30_000 })
    }

    await page.getByRole('tab', { name: /Installed/i }).click()
    await expect(page.locator('.mod-row').first()).toBeVisible({ timeout: 10_000 })

    if (modName !== '') {
      await expect(
        page.locator('.mod-row .mod-name').filter({ hasText: modName }).first(),
      ).toBeVisible({
        timeout: 10_000,
      })
    }
  })
})
