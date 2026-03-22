import { test, expect, type Page } from '@playwright/test'
import {
  loadTestState,
  apiLogin,
  apiSetServerSoftware,
  apiSearchMods,
  apiListInstalledMods,
  apiUninstallMod,
} from './helpers'

const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

async function gotoConsolePage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/console`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByLabel('Software').first()).toBeVisible({ timeout: 10_000 })
}

async function gotoModsPage(page: Page, gameServerId: string) {
  await page.goto(`/game-servers/${gameServerId}/mods`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByRole('tab', { name: /Installed/i })).toBeVisible({ timeout: 10_000 })
}

async function openSoftwareMenu(page: Page) {
  const softwareSelect = page.getByLabel('Software').first()
  await expect(softwareSelect).toBeVisible({ timeout: 5_000 })
  await softwareSelect.click()

  const menu = page.locator('.q-menu').last()
  await expect(menu).toBeVisible({ timeout: 5_000 })

  return { softwareSelect, menu }
}

async function selectSoftware(page: Page, softwareName: string) {
  const { menu } = await openSoftwareMenu(page)
  await menu.getByText(softwareName, { exact: true }).click()
}

async function openBrowseTab(page: Page) {
  await page.getByRole('tab', { name: /Browse/i }).click()
  await expect(page.getByPlaceholder('Search mods...')).toBeVisible({ timeout: 10_000 })
}

async function confirmSoftwareChange(page: Page) {
  const dialog = page.locator('.q-dialog').filter({ hasText: 'Change Server Software' }).first()
  await expect(dialog).toBeVisible({ timeout: 5_000 })
  await dialog.getByRole('button', { name: 'Confirm' }).click()
  await expect(dialog).not.toBeVisible({ timeout: 10_000 })
}

/**
 * E2E tests for server software selection and mod management.
 *
 * Prerequisites (seeded by the E2E orchestrator):
 * - "E2E Test Game" with server_software JSON (Minecraft preset: Vanilla, Paper, Purpur, Fabric)
 * - "E2E Test Server" with server_software set to "paper"
 * - Backend running on :8080 with mod providers registered
 *
 * Tests are sequential (workers: 1) and build on shared state.
 * Each describe block cleans up after itself.
 */

// ============================================================================
// Server Software Selection
// ============================================================================
test.describe('Server Software Selection', () => {
  test('console page shows Server Software section', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // The Server Software section heading should be visible.
    await expect(page.getByText('Server Software')).toBeVisible({ timeout: 5_000 })
  })

  test('software dropdown lists Paper, Purpur, Fabric, Vanilla', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Click the Software dropdown to open options.
    const { menu } = await openSoftwareMenu(page)

    // Quasar renders dropdown options in a q-menu portal.
    await expect(menu.getByText('Paper')).toBeVisible()
    await expect(menu.getByText('Purpur')).toBeVisible()
    await expect(menu.getByText('Fabric')).toBeVisible()
    await expect(menu.getByText('Vanilla')).toBeVisible()

    // Close the menu by pressing Escape.
    await page.keyboard.press('Escape')
  })

  test('selecting Paper loads version dropdown from PaperMC API', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Select Paper from the software dropdown.
    await selectSoftware(page, 'Paper')

    // The version dropdown should become enabled and populated (hits PaperMC API).
    const versionSelect = page.getByLabel('Version')
    await expect(versionSelect).toBeVisible({ timeout: 15_000 })

    // Click to verify it has options.
    await versionSelect.click()
    const versionMenu = page.locator('.q-menu').last()
    await expect(versionMenu).toBeVisible({ timeout: 5_000 })

    // Should have at least one version option (e.g., "1.21.4").
    const options = versionMenu.locator('.q-item')
    await expect(options.first()).toBeVisible({ timeout: 15_000 })
    expect(await options.count()).toBeGreaterThan(0)

    await page.keyboard.press('Escape')
  })

  test('apply Paper → confirm dialog → success', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Select Paper.
    await selectSoftware(page, 'Paper')

    // Click Apply.
    const applyBtn = page.getByRole('button', { name: /Apply/i })
    await expect(applyBtn).toBeVisible()
    await expect(applyBtn).toBeEnabled({ timeout: 15_000 })
    await applyBtn.click()

    await confirmSoftwareChange(page)
  })

  test('reload console → Paper is pre-selected', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // The software dropdown should show "Paper" as the current value.
    const softwareSelect = page.getByLabel('Software').first()
    await expect(softwareSelect).toContainText('Paper', { timeout: 5_000 })
  })

  test('apply Purpur → confirm → reload → Purpur pre-selected', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Select Purpur.
    await selectSoftware(page, 'Purpur')

    // Apply.
    const applyBtn = page.getByRole('button', { name: /Apply/i })
    await expect(applyBtn).toBeEnabled({ timeout: 15_000 })
    await applyBtn.click()
    await confirmSoftwareChange(page)

    // Reload and verify.
    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel('Software').first()).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel('Software').first()).toContainText('Purpur', { timeout: 5_000 })
  })

  test('apply Fabric → confirm → reload → Fabric pre-selected', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Select Fabric.
    await selectSoftware(page, 'Fabric')

    // Apply (Fabric has no jar_source, so no version dropdown needed).
    const applyBtn = page.getByRole('button', { name: /Apply/i })
    await expect(applyBtn).toBeEnabled({ timeout: 15_000 })
    await applyBtn.click()
    await confirmSoftwareChange(page)

    // Reload and verify.
    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel('Software').first()).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel('Software').first()).toContainText('Fabric', { timeout: 5_000 })
  })

  test('restore Paper for subsequent tests', async () => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Use API to restore Paper quickly.
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerSoftware(cookies, state.gameServerId, 'paper', '1.21.4')
  })
})

// ============================================================================
// Mods Tab Visibility
// ============================================================================
test.describe('Mods Tab Visibility', () => {
  test('Mods tab visible when software is Paper', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // The Mods tab should be visible in the navigation.
    await expect(page.getByRole('tab', { name: /Mods/i })).toBeVisible({ timeout: 5_000 })
  })

  test('Mods tab hidden when software is Vanilla', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Switch to Vanilla via API.
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerSoftware(cookies, state.gameServerId, 'vanilla', '')

    // Navigate to game server page.
    await gotoConsolePage(page, state.gameServerId)

    // Mods tab should NOT be visible.
    await expect(page.getByRole('tab', { name: /Mods/i })).not.toBeVisible({ timeout: 3_000 })

    // Restore Paper for subsequent tests.
    await apiSetServerSoftware(cookies, state.gameServerId, 'paper', '1.21.4')
  })

  test('Mods tab returns when switching back to Paper', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)

    // Mods tab should be visible again.
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

    // Switch to Browse tab.
    await openBrowseTab(page)

    // Search input should be visible.
    await expect(page.getByPlaceholder('Search mods...')).toBeVisible()

    // Before searching, should show the "Search for mods" empty state.
    await expect(page.locator('body')).toContainText(/search for mods/i)
  })

  test('search "WorldEdit" returns results from Modrinth', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    // Switch to Browse tab and search.
    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')

    // Wait for debounce (300ms) + API response (may take a while).
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    // Should have at least one result.
    const cards = page.locator('.mod-card')
    expect(await cards.count()).toBeGreaterThan(0)

    // First result should contain "WorldEdit" in the name.
    const firstName = await cards.first().locator('.mod-card-name').textContent()
    expect(firstName?.toLowerCase()).toContain('worldedit')
  })
})

// ============================================================================
// Mod Install Flow - Paper
// ============================================================================
test.describe('Mod Install Flow - Paper', () => {
  test.afterAll(async () => {
    // Cleanup: uninstall any mods we installed during these tests.
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

    // Switch to Browse tab and search for WorldEdit.
    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')

    // Wait for results.
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    // Click Install on the first non-installed card.
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

    // Switch to Installed tab.
    await page.getByRole('tab', { name: /Installed/i }).click()

    // Should have at least one installed mod.
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

    // Switch to Browse tab and search again.
    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('WorldEdit')
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    // At least one card should show "Installed" badge.
    await expect(page.locator('.installed-badge').first()).toBeVisible({ timeout: 5_000 })
  })
})

// ============================================================================
// Installed Mod Management
// ============================================================================
test.describe('Installed Mod Management', () => {
  // Pre-install a mod via API for these tests.
  test.beforeAll(async () => {
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)

    // Search for WorldEdit to get a source_id.
    const results = await apiSearchMods(cookies, state.gameServerId, 'WorldEdit')
    if (results.length > 0) {
      const mod = results[0]!
      // Install the first result (version_id empty = latest).
      try {
        await apiListInstalledMods(cookies, state.gameServerId).then(async (mods) => {
          // Only install if not already installed.
          const alreadyInstalled = mods.some((m) => m.source === mod.source)
          if (!alreadyInstalled) {
            await fetch(
              `${process.env['BACKEND_URL'] ?? 'http://localhost:8080'}/xylona.Xylona/InstallMod`,
              {
                method: 'POST',
                headers: {
                  'Content-Type': 'application/json',
                  'Connect-Protocol-Version': '1',
                  Cookie: cookies.raw,
                },
                body: JSON.stringify({
                  game_server_id: state.gameServerId,
                  source: mod.source,
                  source_id: mod.source_id,
                  version_id: '',
                }),
              },
            )
          }
        })
      } catch {
        // Best effort — tests will skip if no mod is installed.
      }
    }
  })

  test.afterAll(async () => {
    // Cleanup all installed mods.
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

    // Verify mod row has name, version, and source badge.
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

    // Find the auto-update toggle.
    const toggle = firstRow.locator('.q-toggle')
    await expect(toggle).toBeVisible()

    // Get initial state.
    const checkbox = toggle.locator('input[type="checkbox"]')
    const wasCh = await checkbox.isChecked()

    // Toggle it.
    await toggle.click()
    if (wasCh) {
      await expect(checkbox).not.toBeChecked({ timeout: 10_000 })
    } else {
      await expect(checkbox).toBeChecked({ timeout: 10_000 })
    }

    // Verify it changed.
    const nowCh = await checkbox.isChecked()
    expect(nowCh).not.toBe(wasCh)

    // Reload and verify persistence.
    await page.reload()
    await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.mod-row').first()).toBeVisible({ timeout: 10_000 })

    const reloadedToggle = page
      .locator('.mod-row')
      .first()
      .locator('.q-toggle input[type="checkbox"]')
    const afterReload = await reloadedToggle.isChecked()
    expect(afterReload).toBe(nowCh)
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

    // Click overflow menu.
    await firstRow.locator('[aria-label*="Actions"]').click()

    // Click Uninstall in the menu.
    const uninstallAction = page.getByText('Uninstall', { exact: true })
    await expect(uninstallAction).toBeVisible({ timeout: 5_000 })
    await uninstallAction.click()

    // Confirm in dialog.
    const dialog = page
      .locator('.q-dialog')
      .filter({ hasText: /uninstall/i })
      .first()
    if (await dialog.isVisible().catch(() => false)) {
      await dialog.getByRole('button', { name: /Uninstall|OK|Confirm/i }).click()
      await expect(dialog).not.toBeVisible({ timeout: 10_000 })
    }

    // The mod should be gone.
    if (modName) {
      await expect(page.locator('.mod-name').filter({ hasText: modName })).toHaveCount(0, {
        timeout: 10_000,
      })
    }

    // Count should decrease.
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
    // Cleanup: uninstall all mods and restore Paper.
    const state = loadTestState()
    if (!state.gameServerId) return
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    const mods = await apiListInstalledMods(cookies, state.gameServerId)
    for (const mod of mods) {
      await apiUninstallMod(cookies, state.gameServerId, mod.id)
    }
    await apiSetServerSoftware(cookies, state.gameServerId, 'paper', '1.21.4')
  })

  test('switch to Fabric via API and verify Mods tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    // Switch to Fabric via API.
    const cookies = await apiLogin(ADMIN_USERNAME, ADMIN_PASSWORD)
    await apiSetServerSoftware(cookies, state.gameServerId, 'fabric', '')

    // Navigate to game server page.
    await gotoModsPage(page, state.gameServerId)

    // Mods tab should still be visible (Fabric has mod_config).
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

    // Switch to Browse and search for Sodium (popular Fabric mod).
    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('Sodium')

    // Wait for results.
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    const cards = page.locator('.mod-card')
    expect(await cards.count()).toBeGreaterThan(0)

    // First result should contain "Sodium".
    const firstName = await cards.first().locator('.mod-card-name').textContent()
    expect(firstName?.toLowerCase()).toContain('sodium')
  })

  test('install Sodium → appears in Installed tab', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoModsPage(page, state.gameServerId)

    // Search for Sodium on Browse tab.
    await openBrowseTab(page)
    await page.getByPlaceholder('Search mods...').fill('Sodium')
    await expect(page.locator('.mod-card').first()).toBeVisible({ timeout: 30_000 })

    // Install the first result.
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

    // Switch to Installed tab and verify.
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
