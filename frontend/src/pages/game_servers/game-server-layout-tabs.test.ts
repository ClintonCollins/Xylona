import { describe, expect, it } from 'vitest'
import {
  GAME_SERVER_TAB_GROUPS,
  buildGameServerTabs,
  getUnauthorizedRedirect,
} from './game-server-layout-tabs'

describe('buildGameServerTabs', () => {
  const serverID = 'test-server'

  it('shows only Console tab for viewer (game_server.view only)', () => {
    const tabs = buildGameServerTabs(serverID, ['game_server.view'], false)
    expect(tabs.map((t) => t.name)).toEqual(['Console'])
  })

  it('shows the exact live map to viewers of supported map games', () => {
    const tabs = buildGameServerTabs(
      serverID,
      ['game_server.view'],
      false,
      false,
      true,
      false,
      true,
    )

    expect(tabs.map((tab) => tab.name)).toEqual(['Console', 'Map'])
  })

  it('does not show the map tab for games without live map support', () => {
    const tabs = buildGameServerTabs(
      serverID,
      ['game_server.view'],
      false,
      false,
      true,
      false,
      false,
    )

    expect(tabs.map((tab) => tab.name)).toEqual(['Console'])
  })

  it('shows Players for an operator with the player-management permission', () => {
    const perms = [
      'game_server.view',
      'game_server.start',
      'game_server.stop',
      'game_server.restart',
      'game_server.console',
      'game_server.players.manage',
    ]
    const tabs = buildGameServerTabs(serverID, perms, false)
    expect(tabs.map((t) => t.name)).toEqual(['Console', 'Players'])
  })

  it('shows all tabs for admin role', () => {
    const perms = [
      'game_server.view',
      'game_server.start',
      'game_server.stop',
      'game_server.restart',
      'game_server.console',
      'game_server.players.manage',
      'game_server.files.view',
      'game_server.files.edit',
      'game_server.settings',
      'game_server.metrics',
      'game_server.backup',
      'game_server.delete',
    ]
    const tabs = buildGameServerTabs(serverID, perms, false)
    expect(tabs.map((t) => t.name)).toEqual([
      'Console',
      'Players',
      'Metrics',
      'Files',
      'Start Command',
      'Settings',
      'Backups',
    ])
  })

  it('shows Backups tab when user has backup permission', () => {
    const tabs = buildGameServerTabs(serverID, ['game_server.backup'], false)
    expect(tabs.map((t) => t.name)).toContain('Backups')
  })

  it('hides Start Command tab when editing is disabled for non-superusers', () => {
    const tabs = buildGameServerTabs(
      serverID,
      ['game_server.view', 'game_server.settings'],
      false,
      false,
      false,
      false,
    )

    expect(tabs.map((t) => t.name)).not.toContain('Start Command')
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

  it('shows Alerts tab when user has alerts.manage permission', () => {
    const tabs = buildGameServerTabs(serverID, ['alerts.manage'], false)
    expect(tabs.map((t) => t.name)).toContain('Alerts')
  })

  it('shows Alerts tab when user has alerts.view_history permission', () => {
    const tabs = buildGameServerTabs(serverID, ['alerts.view_history'], false)
    expect(tabs.map((t) => t.name)).toContain('Alerts')
  })

  it('does not show Alerts tab without alerts permissions', () => {
    const tabs = buildGameServerTabs(serverID, ['game_server.view'], false)
    expect(tabs.map((t) => t.name)).not.toContain('Alerts')
  })

  it('shows Alerts tab for superuser even without alert permissions', () => {
    const tabs = buildGameServerTabs(serverID, ['game_server.view'], true)
    expect(tabs.map((t) => t.name)).toContain('Alerts')
  })

  describe('tab groups', () => {
    const allPerms = [
      'game_server.view',
      'game_server.players.manage',
      'game_server.config',
      'game_server.files.view',
      'game_server.metrics',
      'game_server.settings',
      'game_server.mods',
      'game_server.scheduled_tasks',
      'game_server.backup',
    ]

    function buildAllTabs() {
      return buildGameServerTabs(serverID, allPerms, true, true, true, false, true)
    }

    it('assigns every tab to exactly one known group', () => {
      const tabs = buildAllTabs()
      expect(tabs).toHaveLength(13)
      for (const tab of tabs) {
        expect(GAME_SERVER_TAB_GROUPS).toContain(tab.group)
      }
    })

    it.each([
      { name: 'Console', group: 'Operate' },
      { name: 'Map', group: 'Operate' },
      { name: 'Players', group: 'Operate' },
      { name: 'Metrics', group: 'Operate' },
      { name: 'Configuration', group: 'Configure' },
      { name: 'Files', group: 'Configure' },
      { name: 'Start Command', group: 'Configure' },
      { name: 'Settings', group: 'Configure' },
      { name: 'Mods', group: 'Configure' },
      { name: 'Schedules', group: 'Automate' },
      { name: 'Backups', group: 'Automate' },
      { name: 'Alerts', group: 'Automate' },
      { name: 'Access', group: 'Access' },
    ])('places the $name tab in the $group group', ({ name, group }) => {
      const tabs = buildAllTabs()
      expect(tabs.find((tab) => tab.name === name)?.group).toBe(group)
    })

    it.each([
      {
        caseName: 'full owner tab set',
        build: buildAllTabs,
      },
      {
        caseName: 'admin without config or ownership',
        build: () =>
          buildGameServerTabs(
            serverID,
            [
              'game_server.view',
              'game_server.players.manage',
              'game_server.files.view',
              'game_server.settings',
              'game_server.metrics',
              'game_server.backup',
            ],
            false,
          ),
      },
      {
        caseName: 'viewer with live map',
        build: () =>
          buildGameServerTabs(serverID, ['game_server.view'], false, false, true, false, true),
      },
    ])('keeps groups contiguous and in stable order for $caseName', ({ build }) => {
      const tabs = build()
      const groupIndexes = tabs.map((tab) => GAME_SERVER_TAB_GROUPS.indexOf(tab.group))
      const sortedIndexes = [...groupIndexes].sort((a, b) => a - b)
      expect(groupIndexes).toEqual(sortedIndexes)
    })

    it('keeps group order as Operate, Configure, Automate, Access', () => {
      expect(GAME_SERVER_TAB_GROUPS).toEqual(['Operate', 'Configure', 'Automate', 'Access'])
    })

    it('orders the full owner tab set by group cluster', () => {
      const tabs = buildAllTabs()
      expect(tabs.map((tab) => tab.name)).toEqual([
        'Console',
        'Map',
        'Players',
        'Metrics',
        'Configuration',
        'Files',
        'Start Command',
        'Settings',
        'Mods',
        'Schedules',
        'Backups',
        'Alerts',
        'Access',
      ])
    })
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

  it('allows the map for a viewer of a supported map game', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/map`,
      serverID,
      ['game_server.view'],
      false,
      false,
      true,
      false,
      true,
    )

    expect(result).toBeNull()
  })

  it('redirects the map when the game has no live map support', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/map`,
      serverID,
      ['game_server.view'],
      false,
      false,
      true,
      false,
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

  it('redirects /players when missing player-management permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/players`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('returns null for /players with player-management permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/players`,
      serverID,
      ['game_server.players.manage'],
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

  it('redirects /backups when missing backup permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/backups`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('returns null for /backups when user has backup permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/backups`,
      serverID,
      ['game_server.backup'],
      false,
    )
    expect(result).toBeNull()
  })

  it('redirects /start-command when editing is disabled for non-superusers', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/start-command`,
      serverID,
      ['game_server.settings'],
      false,
      false,
      false,
      false,
    )
    expect(result).toBe(consolePath)
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

  it('redirects /alerts when missing alerts permissions', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/alerts`,
      serverID,
      ['game_server.view'],
      false,
    )
    expect(result).toBe(consolePath)
  })

  it('returns null for /alerts when user has alerts.manage permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/alerts`,
      serverID,
      ['alerts.manage'],
      false,
    )
    expect(result).toBeNull()
  })

  it('returns null for /alerts when user has alerts.view_history permission', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/alerts`,
      serverID,
      ['alerts.view_history'],
      false,
    )
    expect(result).toBeNull()
  })

  it('returns null for /alerts when user is owner/super even without alert permissions', () => {
    const result = getUnauthorizedRedirect(
      `/game-servers/${serverID}/alerts`,
      serverID,
      ['game_server.view'],
      true,
    )
    expect(result).toBeNull()
  })
})
