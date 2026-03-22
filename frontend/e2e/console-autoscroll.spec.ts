import { test, expect, type Page } from '@playwright/test'
import { loadTestState } from './helpers'

interface ScrollInfo {
  scrollTop: number
  scrollHeight: number
  clientHeight: number
  isAtBottom: boolean
  isScrollable: boolean
}

async function getScrollInfo(page: Page): Promise<ScrollInfo> {
  const info = await page.evaluate(() => {
    const container = document.querySelector('#consoleContainer .q-scrollarea__container')
    if (!container) return null
    const gap = container.scrollHeight - container.scrollTop - container.clientHeight
    return {
      scrollTop: container.scrollTop,
      scrollHeight: container.scrollHeight,
      clientHeight: container.clientHeight,
      isAtBottom: gap < 5,
      isScrollable: container.scrollHeight > container.clientHeight + 5,
    }
  })
  if (info === null) {
    throw new Error('Console scroll container not found')
  }
  return info
}

async function scrollToTop(page: Page): Promise<void> {
  await page.evaluate(() => {
    const container = document.querySelector('#consoleContainer .q-scrollarea__container')
    if (container) container.scrollTop = 0
  })
}

async function appendConsoleContent(page: Page, lineCount: number, prefix: string): Promise<void> {
  await page.evaluate(
    ({ count, pfx }) => {
      const codeEl = document.querySelector('#consoleCodeEl')
      if (!codeEl) return
      const lines: string[] = []
      for (let i = 1; i <= count; i++) {
        lines.push(`${pfx} ${i}\n`)
      }
      codeEl.innerHTML += lines.join('')
    },
    { count: lineCount, pfx: prefix },
  )
  // Allow DOM to settle
  await page.waitForTimeout(100)
}

async function gotoConsolePage(page: Page, gameServerId: string): Promise<void> {
  await page.goto(`/game-servers/${gameServerId}/console`)
  await expect(page.locator('.q-layout')).toBeVisible({ timeout: 10_000 })
}

function getAutoScrollButton(page: Page) {
  return page.getByRole('button', { name: /auto-scroll/i })
}

async function ensureScrollable(page: Page): Promise<void> {
  // If console isn't scrollable yet, inject enough content to make it so
  const info = await getScrollInfo(page)
  if (!info.isScrollable) {
    await appendConsoleContent(page, 100, '[test] filler line')
  }
  const afterInfo = await getScrollInfo(page)
  expect(afterInfo.isScrollable).toBe(true)
}

test.describe('Console auto-scroll toggle', () => {
  test('slow output: toggle controls scroll behavior', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)
    await expect(page.locator('#consoleContainer')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('#consoleCodeEl')).not.toBeEmpty({ timeout: 15_000 })

    // Auto-scroll button should exist and show enabled state
    const toggleBtn = getAutoScrollButton(page)
    await expect(toggleBtn).toBeVisible()

    // Make console scrollable
    await ensureScrollable(page)

    // Disable auto-scroll
    await toggleBtn.click()

    // Scroll to top
    await scrollToTop(page)
    await page.waitForTimeout(300)

    const infoAfterScrollUp = await getScrollInfo(page)
    expect(infoAfterScrollUp.isAtBottom).toBe(false)

    // Inject new content — scroll should NOT snap to bottom (auto-scroll is off)
    await appendConsoleContent(page, 20, '[test] noscroll-line')

    const infoAfterNewOutput = await getScrollInfo(page)
    expect(infoAfterNewOutput.isAtBottom).toBe(false)

    // Re-enable auto-scroll — should immediately scroll to bottom
    await toggleBtn.click()
    await page.waitForTimeout(300)

    const infoAfterReEnable = await getScrollInfo(page)
    expect(infoAfterReEnable.isAtBottom).toBe(true)

    // Disable again, scroll up, add content, verify position held
    await toggleBtn.click()
    await scrollToTop(page)
    await page.waitForTimeout(300)
    await appendConsoleContent(page, 10, '[test] second-batch')
    const infoSecondCheck = await getScrollInfo(page)
    expect(infoSecondCheck.isAtBottom).toBe(false)

    // Re-enable once more — snaps to bottom again
    await toggleBtn.click()
    await page.waitForTimeout(300)
    const infoFinal = await getScrollInfo(page)
    expect(infoFinal.isAtBottom).toBe(true)
  })

  test('fast bulk output: auto-scroll off stays stable', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)
    await expect(page.locator('#consoleContainer')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('#consoleCodeEl')).not.toBeEmpty({ timeout: 15_000 })

    // Make console scrollable
    await ensureScrollable(page)

    // Disable auto-scroll
    const toggleBtn = getAutoScrollButton(page)
    await toggleBtn.click()

    // Scroll to top
    await scrollToTop(page)
    await page.waitForTimeout(300)

    const infoBefore = await getScrollInfo(page)
    expect(infoBefore.isAtBottom).toBe(false)

    // Inject 200 lines at once (simulates flood / fast output)
    await appendConsoleContent(page, 200, '[test] flood-line')
    await page.waitForTimeout(300)

    // Scroll position should still NOT be at bottom
    const infoAfterFlood = await getScrollInfo(page)
    expect(infoAfterFlood.isAtBottom).toBe(false)

    // Re-enable auto-scroll — should jump to bottom
    await toggleBtn.click()
    await page.waitForTimeout(300)

    const infoAfterReEnable = await getScrollInfo(page)
    expect(infoAfterReEnable.isAtBottom).toBe(true)
  })

  test('preference persists across page reload', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    await gotoConsolePage(page, state.gameServerId)
    await expect(page.locator('#consoleContainer')).toBeVisible({ timeout: 10_000 })

    const toggleBtn = getAutoScrollButton(page)
    await expect(toggleBtn).toBeVisible()

    // Verify default state: auto-scroll is ON
    const defaultValue = await page.evaluate(() =>
      localStorage.getItem('xylona_console_autoscroll'),
    )
    expect(defaultValue === null || defaultValue === 'true').toBe(true)

    // Disable auto-scroll
    await toggleBtn.click()
    await page.waitForTimeout(200)

    // Verify localStorage was updated
    const disabledValue = await page.evaluate(() =>
      localStorage.getItem('xylona_console_autoscroll'),
    )
    expect(disabledValue).toBe('false')

    // Reload the page
    await page.reload()
    await expect(page.locator('#consoleContainer')).toBeVisible({ timeout: 10_000 })

    // Verify preference was restored from localStorage
    const restoredValue = await page.evaluate(() =>
      localStorage.getItem('xylona_console_autoscroll'),
    )
    expect(restoredValue).toBe('false')

    // Re-enable auto-scroll (cleanup for other tests)
    const toggleBtnAfterReload = getAutoScrollButton(page)
    await toggleBtnAfterReload.click()
    await page.waitForTimeout(200)

    const cleanupValue = await page.evaluate(() =>
      localStorage.getItem('xylona_console_autoscroll'),
    )
    expect(cleanupValue).toBe('true')
  })
})
