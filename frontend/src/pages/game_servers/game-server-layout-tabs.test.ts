import { describe, expect, it } from 'vitest'

import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'

describe('buildGameServerTabs', () => {
  it('includes only console and files when configuration and access are disabled', () => {
    const tabs = buildGameServerTabs('server-1', false, false)
    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Files'])
  })

  it('includes configuration when enabled', () => {
    const tabs = buildGameServerTabs('server-1', true, false)
    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Files', 'Configuration'])
  })

  it('includes access when enabled', () => {
    const tabs = buildGameServerTabs('server-1', true, true)
    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Files', 'Configuration', 'Access'])
  })
})

describe('getUnauthorizedRedirect', () => {
  it('redirects access route when access is not allowed', () => {
    const redirect = getUnauthorizedRedirect('/game-servers/server-1/access', 'server-1', true, false)
    expect(redirect).toBe('/game-servers/server-1/console')
  })

  it('redirects configuration route when configuration is not allowed', () => {
    const redirect = getUnauthorizedRedirect('/game-servers/server-1/configuration', 'server-1', false, true)
    expect(redirect).toBe('/game-servers/server-1/console')
  })

  it('does not redirect allowed routes', () => {
    const redirect = getUnauthorizedRedirect('/game-servers/server-1/files', 'server-1', true, true)
    expect(redirect).toBeNull()
  })
})
