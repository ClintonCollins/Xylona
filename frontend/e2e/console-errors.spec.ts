import { test, expect } from '@playwright/test'
import { loadTestState } from './helpers'

// Uses the default auth state (e2e-superuser saved by auth.setup.ts)

const ALL_PAGES = [
  { name: 'Dashboard', path: '/' },
  { name: 'Game servers list', path: '/game-servers' },
  { name: 'Games list', path: '/games' },
  { name: 'Nodes list', path: '/nodes' },
  { name: 'Admin users list', path: '/admin/users' },
  { name: 'Secret keys', path: '/secret-keys' },
]

test.describe('All pages load without console errors', () => {
  for (const { name, path: pagePath } of ALL_PAGES) {
    test(`${name} (${pagePath}) loads without console errors`, async ({ page }) => {
      const errors: string[] = []
      page.on('console', (msg) => {
        if (msg.type() === 'error') errors.push(msg.text())
      })
      page.on('pageerror', (err) => errors.push(err.message))

      await page.goto(pagePath)
      await page.waitForLoadState('networkidle')

      expect(errors).toEqual([])
    })
  }

  test('Game server console page loads without console errors', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const errors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto(`/game-servers/${state.gameServerId}/console`)
    await page.waitForLoadState('networkidle')

    expect(errors).toEqual([])
  })

  test('Game server files page loads without console errors', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const errors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto(`/game-servers/${state.gameServerId}/files`)
    await page.waitForLoadState('networkidle')

    expect(errors).toEqual([])
  })

  test('Game server configuration page loads without console errors', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const errors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto(`/game-servers/${state.gameServerId}/configuration`)
    await page.waitForLoadState('networkidle')

    expect(errors).toEqual([])
  })

  test('Game server access page loads without console errors', async ({ page }) => {
    const state = loadTestState()
    if (!state.gameServerId) {
      test.skip(true, 'No game server available')
      return
    }

    const errors: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text())
    })
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto(`/game-servers/${state.gameServerId}/access`)
    await page.waitForLoadState('networkidle')

    expect(errors).toEqual([])
  })
})
