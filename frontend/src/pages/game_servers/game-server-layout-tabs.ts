export interface GameServerLayoutTab {
  name: string
  to: string
  icon: string
  exact: boolean
  requiredPermission?: string
}

export function buildGameServerTabs(
  serverID: string,
  permissions: string[],
  isOwnerOrSuper: boolean,
  hasModSupport = false,
  allowStartArgEditing = true,
  isSuperUser = false,
  hasLiveMap = false,
): GameServerLayoutTab[] {
  const basePath = `/game-servers/${serverID}`
  const has = (perm: string) => permissions.includes(perm)

  const tabs: GameServerLayoutTab[] = [
    { name: 'Console', to: `${basePath}/console`, icon: 'terminal', exact: true },
  ]

  if (hasLiveMap && has('game_server.view')) {
    tabs.push({
      name: 'Map',
      to: `${basePath}/map`,
      icon: 'public',
      exact: true,
      requiredPermission: 'game_server.view',
    })
  }

  if (has('game_server.players.manage')) {
    tabs.push({
      name: 'Players',
      to: `${basePath}/players`,
      icon: 'groups',
      exact: true,
      requiredPermission: 'game_server.players.manage',
    })
  }
  if (has('game_server.config')) {
    tabs.push({
      name: 'Configuration',
      to: `${basePath}/configuration`,
      icon: 'tune',
      exact: true,
      requiredPermission: 'game_server.config',
    })
  }
  if (has('game_server.files.view')) {
    tabs.push({
      name: 'Files',
      to: `${basePath}/files`,
      icon: 'folder',
      exact: true,
      requiredPermission: 'game_server.files.view',
    })
  }
  if (has('game_server.metrics')) {
    tabs.push({
      name: 'Metrics',
      to: `${basePath}/metrics`,
      icon: 'show_chart',
      exact: true,
      requiredPermission: 'game_server.metrics',
    })
  }
  if (has('game_server.settings')) {
    if (allowStartArgEditing || isSuperUser) {
      tabs.push({
        name: 'Start Command',
        to: `${basePath}/start-command`,
        icon: 'terminal',
        exact: true,
        requiredPermission: 'game_server.settings',
      })
    }
    tabs.push({
      name: 'Settings',
      to: `${basePath}/settings`,
      icon: 'settings',
      exact: true,
      requiredPermission: 'game_server.settings',
    })
  }
  if (has('game_server.mods') && hasModSupport) {
    tabs.push({
      name: 'Mods',
      to: `${basePath}/mods`,
      icon: 'extension',
      exact: true,
      requiredPermission: 'game_server.mods',
    })
  }
  if (has('game_server.scheduled_tasks')) {
    tabs.push({
      name: 'Schedules',
      to: `${basePath}/schedules`,
      icon: 'schedule',
      exact: true,
      requiredPermission: 'game_server.scheduled_tasks',
    })
  }
  if (has('game_server.backup')) {
    tabs.push({
      name: 'Backups',
      to: `${basePath}/backups`,
      icon: 'archive',
      exact: true,
      requiredPermission: 'game_server.backup',
    })
  }
  if (isOwnerOrSuper || has('alerts.manage') || has('alerts.view_history')) {
    tabs.push({
      name: 'Alerts',
      to: `${basePath}/alerts`,
      icon: 'notifications',
      exact: true,
    })
  }
  if (isOwnerOrSuper) {
    tabs.push({
      name: 'Access',
      to: `${basePath}/access`,
      icon: 'manage_accounts',
      exact: true,
    })
  }

  return tabs
}

export function getUnauthorizedRedirect(
  currentPath: string,
  serverID: string,
  permissions: string[],
  isOwnerOrSuper: boolean,
  hasModSupport = false,
  allowStartArgEditing = true,
  isSuperUser = false,
  hasLiveMap = false,
): string | null {
  const basePath = `/game-servers/${serverID}`
  const consolePath = `${basePath}/console`
  const has = (perm: string) => permissions.includes(perm)

  if (currentPath === `${basePath}/map` && (!hasLiveMap || !has('game_server.view'))) {
    return consolePath
  }

  if (currentPath === `${basePath}/files` && !has('game_server.files.view')) {
    return consolePath
  }
  if (currentPath === `${basePath}/players` && !has('game_server.players.manage')) {
    return consolePath
  }
  if (currentPath === `${basePath}/metrics` && !has('game_server.metrics')) {
    return consolePath
  }
  if (currentPath === `${basePath}/configuration` && !has('game_server.config')) {
    return consolePath
  }
  if (currentPath === `${basePath}/settings` && !has('game_server.settings')) {
    return consolePath
  }
  if (
    currentPath === `${basePath}/start-command` &&
    (!has('game_server.settings') || (!allowStartArgEditing && !isSuperUser))
  ) {
    return consolePath
  }
  if (currentPath === `${basePath}/mods` && (!has('game_server.mods') || !hasModSupport)) {
    return consolePath
  }
  if (currentPath === `${basePath}/schedules` && !has('game_server.scheduled_tasks')) {
    return consolePath
  }
  if (currentPath === `${basePath}/backups` && !has('game_server.backup')) {
    return consolePath
  }
  if (
    currentPath === `${basePath}/alerts` &&
    !isOwnerOrSuper &&
    !has('alerts.manage') &&
    !has('alerts.view_history')
  ) {
    return consolePath
  }
  if (currentPath === `${basePath}/access` && !isOwnerOrSuper) {
    return consolePath
  }

  return null
}
