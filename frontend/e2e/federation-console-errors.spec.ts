import { test, expect } from '@playwright/test'
import {
  fedApiListAggregatedGameServers,
  fedApiLogin,
  NODE_A_BACKEND,
} from './federation-helpers'

let remoteServerId: string | undefined

test.beforeAll(async () => {
  const { cookies: adminCookies } = await fedApiLogin('e2e-superuser', 'TestPassword123!', NODE_A_BACKEND)
  const servers = await fedApiListAggregatedGameServers(adminCookies, NODE_A_BACKEND)
  const remoteServer = servers.find((s) => !s.isLocal)
  remoteServerId = remoteServer?.remoteServer?.remoteServerId
})

test.describe('All federation pages load without JS errors (superuser)', () => {
  const STATIC_PAGES = [
    { name: 'Nodes (shows paired node)', path: '/nodes' },
    { name: 'Game servers (shows remote)', path: '/game-servers' },
  ]

  for (const { name, path: pagePath } of STATIC_PAGES) {
    test(`${name} loads without errors`, async ({ page }) => {
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

  const REMOTE_TABS = [
    { name: 'Remote console', suffix: '/console' },
    { name: 'Remote files', suffix: '/files' },
    { name: 'Remote configuration', suffix: '/configuration' },
    { name: 'Remote access', suffix: '/access' },
  ]

  for (const { name, suffix } of REMOTE_TABS) {
    test(`${name} loads without errors`, async ({ page }) => {
      if (!remoteServerId) {
        test.skip(true, 'No remote server')
        return
      }

      const errors: string[] = []
      page.on('console', (msg) => {
        if (msg.type() === 'error') errors.push(msg.text())
      })
      page.on('pageerror', (err) => errors.push(err.message))

      await page.goto(`/game-servers/${remoteServerId}${suffix}`)
      await page.waitForLoadState('networkidle')

      expect(errors).toEqual([])
    })
  }
})
