import { describe, it, expect } from 'vitest'
import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'

describe('buildGameServerTabs', () => {
  const serverID = 'test-server'

  it('shows only Console tab for viewer (game_server.view only)', () => {
    const tabs = buildGameServerTabs(serverID, ['game_server.view'], false)
    expect(tabs.map((t) => t.name)).toEqual(['Console'])
  })

  it('shows Console tab for operator (no files/metrics/config)', () => {
    const perms = [
      'game_server.view',
      'game_server.start',
      'game_server.stop',
      'game_server.restart',
      'game_server.console',
    ]
    const tabs = buildGameServerTabs(serverID, perms, false)
    expect(tabs.map((t) => t.name)).toEqual(['Console'])
  })

  it('shows all tabs for admin role', () => {
    const perms = [
      'game_server.view',
      'game_server.start',
      'game_server.stop',
      'game_server.restart',
      'game_server.console',
      'game_server.files.view',
      'game_server.files.edit',
      'game_server.settings',
      'game_server.metrics',
      'game_server.backup',
      'game_server.delete',
    ]
    const tabs = buildGameServerTabs(serverID, perms, false)
    expect(tabs.map((t) => t.name)).toEqual(['Console', 'Files', 'Metrics', 'Settings'])
  })

  it('shows Access tab only for owner/super', () => {
    const perms = ['game_server.view']
    const tabs = buildGameServerTabs(serverID, perms, true)
    expect(tabs.map((t) => t.name)).toContain('Access')
  })

  it('does not show Access tab for non-owner/non-super even with all permissions', () => {
    const perms = [
      'game_server.view',
      'game_server.files.view',
      'game_server.settings',
      'game_server.metrics',
    ]
    const tabs = buildGameServerTabs(serverID, perms, false)
    expect(tabs.map((t) => t.name)).not.toContain('Access')
  })
})

describe('getUnauthorizedRedirect', () => {
  const serverID = 'test-server'
  const consolePath = `/game-servers/${serverID}/console`

  it('redirects /files when missing files.view permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/files`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('returns null for /files when user has files.view permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/files`,
      serverID,
      ['game_server.files.view'],
      false,
    )
    expect(result).toBeNull()
  })

  it('redirects /metrics when missing metrics permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/metrics`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('redirects /configuration when missing settings permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/configuration`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('redirects /access when not owner/super', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/access`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('allows /access for owner/super', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/access`,
      serverID,
      ['game_server.view'],
      true,
    )
    expect(result).toBeNull()
  })

  it('returns null for console path regardless of permissions', () => {
    const result = getUnauthorizedRedirect(consolePath, serverID, [], false)
    expect(result).toBeNull()
  })

  it('redirects /mods when missing mods permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/mods`,
      serverID,
      ['game_server.view'],
      false,
      true,
    )
    expect(result).toBe(consolePath)
  })

  it('redirects /mods when mod support is removed (hasModSupport false)', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/mods`,
      serverID,
      ['game_server.mods'],
      false,
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('returns null for /mods when user has mods permission and mod support is active', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/mods`,
      serverID,
      ['game_server.mods'],
      false,
      true,
    )
    expect(result).toBeNull()
  })
})
