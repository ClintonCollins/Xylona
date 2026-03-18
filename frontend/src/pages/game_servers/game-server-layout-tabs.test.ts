import { describe, expect, it } from 'vitest'

import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'

describe('buildGameServerTabs', () => {
  it('includes only console and files when configuration and access are disabled', () => {
    const tabs = buildGameServerTabs('server-1', false, false)
    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Files', 'Metrics'])
  })

  it('includes configuration when enabled', () => {
    const tabs = buildGameServerTabs('server-1', true, false)
    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Files', 'Metrics', 'Configuration'])
  })

  it('includes access when enabled', () => {
    const tabs = buildGameServerTabs('server-1', true, true)
    expect(tabs.map((tab) => tab.name)).toEqual([
      'Console',
      'Files',
      'Metrics',
      'Configuration',
      'Access',
    ])
  })
})

describe('buildGameServerTabs with access only', () => {
  it('includes Access but not Configuration when access=true, config=false', () => {
    const tabs = buildGameServerTabs('server-1', false, true)
    const names = tabs.map((tab) => tab.name)
    expect(names).toContain('Console')
    expect(names).toContain('Files')
    expect(names).toContain('Access')
    expect(names).not.toContain('Configuration')
  })
})

describe('buildGameServerTabs paths include serverID', () => {
  it('each tab path contains the server ID', () => {
    const serverID = 'my-unique-server-42'
    const tabs = buildGameServerTabs(serverID, true, true)
    for (const tab of tabs) {
      expect(tab.to).toContain(serverID)
    }
  })
})

describe('getUnauthorizedRedirect', () => {
  it('redirects access route when access is not allowed', () => {
    const redirect = getUnauthorizedRedirect(
      '/game-servers/server-1/access',
      'server-1',
      true,
      false,
    )
    expect(redirect).toBe('/game-servers/server-1/console')
  })

  it('redirects configuration route when configuration is not allowed', () => {
    const redirect = getUnauthorizedRedirect(
      '/game-servers/server-1/configuration',
      'server-1',
      false,
      true,
    )
    expect(redirect).toBe('/game-servers/server-1/console')
  })

  it('does not redirect allowed routes', () => {
    const redirect = getUnauthorizedRedirect('/game-servers/server-1/files', 'server-1', true, true)
    expect(redirect).toBeNull()
  })

  it('does not redirect console route regardless of permissions', () => {
    const redirect = getUnauthorizedRedirect(
      '/game-servers/server-1/console',
      'server-1',
      false,
      false,
    )
    expect(redirect).toBeNull()
  })

  it('does not redirect files route regardless of permissions', () => {
    const redirect = getUnauthorizedRedirect(
      '/game-servers/server-1/files',
      'server-1',
      false,
      false,
    )
    expect(redirect).toBeNull()
  })
})
